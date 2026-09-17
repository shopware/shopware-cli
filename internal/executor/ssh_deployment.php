<?php
// This script is passed to PHP -r; archive bytes arrive on stdin. All human
// output goes to stderr, leaving stdout for the activation handshake.
function deploymentDirectory(string $directory, int $mode = 0755): void
{
    if (is_link($directory) || (file_exists($directory) && !is_dir($directory))) {
        throw new RuntimeException("Expected a real directory: $directory");
    }
    if (!is_dir($directory) && !mkdir($directory, $mode, true)) {
        throw new RuntimeException("Cannot create directory: $directory");
    }
}

function deploymentParents(string $base, string $relative): void
{
    $directory = $base;
    foreach (explode('/', dirname($relative)) as $part) {
        if ($part !== '.') {
            $directory .= '/' . $part;
            deploymentDirectory($directory);
        }
    }
}

function deploymentRemove(string $path): void
{
    if (is_link($path) || is_file($path)) {
        if (!unlink($path)) {
            throw new RuntimeException("Cannot remove release entry: $path");
        }
    } elseif (is_dir($path)) {
        foreach (new FilesystemIterator($path) as $entry) {
            deploymentRemove($entry->getPathname());
        }
        if (!rmdir($path)) {
            throw new RuntimeException("Cannot remove release directory: $path");
        }
    }
}

function deploymentRun(array $command, string $directory): void
{
    $process = proc_open($command, [0 => ['file', '/dev/null', 'r'], 1 => STDERR, 2 => STDERR], $pipes, $directory);
    if (!is_resource($process) || proc_close($process) !== 0) {
        throw new RuntimeException("Deployment command failed: " . implode(' ', $command));
    }
}

function deploymentShare(string $root, string $release, string $relative, bool $directory, ?string $previous): void
{
    $shared = "$root/shared/$relative";
    $source = "$release/$relative";
    deploymentParents("$root/shared", $relative);
    deploymentParents($release, $relative);
    if (is_link($shared)) {
        throw new RuntimeException("Shared paths must not be symlinks: $shared");
    }
    if (!file_exists($shared)) {
        // Never move or copy data out of an application that is still serving
        // traffic. Adoption of an existing layout requires explicit migration.
        if ($previous !== null && (file_exists("$previous/$relative") || is_link("$previous/$relative"))) {
            throw new RuntimeException("Migrate $previous/$relative into $shared before deploying");
        }
        if (file_exists($source) && !is_link($source)) {
            if (is_dir($source) !== $directory || !rename($source, $shared)) {
                throw new RuntimeException("Cannot initialize shared path: $shared");
            }
        } elseif ($directory) {
            deploymentDirectory($shared);
        } else {
            return; // Optional files are not created with empty contents.
        }
    }
    if (($directory && !is_dir($shared)) || (!$directory && !is_file($shared))) {
        throw new RuntimeException("Unexpected shared path type: $shared");
    }
    deploymentRemove($source);
    if (!symlink($shared, $source)) {
        throw new RuntimeException("Cannot link shared path: $source");
    }
}

function deploymentMetadata(string $file, array $metadata): void
{
    $temporary = "$file.tmp";
    if (file_put_contents($temporary, json_encode($metadata, JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT)) === false
        || !rename($temporary, $file)) {
        throw new RuntimeException("Cannot write rollout metadata: $file");
    }
}

$upload = null;
$temporaryLink = null;
$metadataFile = null;
$activated = false;
$lock = null;
$exitCode = 0;
try {
    $input = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);
    $root = $input['root'];
    $id = $input['reference'];
    $current = "$root/current";
    $release = "$root/releases/$id";
    umask(0022);
    deploymentDirectory($root);
    deploymentDirectory("$root/.shopware-cli", 0700);
    $lockPath = "$root/.shopware-cli/deployment.lock";
    if (is_link($lockPath) || (file_exists($lockPath) && !is_file($lockPath))) {
        throw new RuntimeException("Deployment lock must be a regular file");
    }
    $lock = fopen($lockPath, 'c');
    if ($lock === false || !flock($lock, LOCK_EX | LOCK_NB)) {
        throw new RuntimeException("Another deployment holds the lock for $root");
    }
    deploymentDirectory("$root/releases");
    deploymentDirectory("$root/shared");
    deploymentDirectory("$root/.shopware-cli/rollouts", 0700);
    $previous = null;
    if (is_link($current)) {
        $previous = realpath($current);
        if ($previous === false || !is_dir($previous) || dirname($previous) !== realpath("$root/releases")) {
            throw new RuntimeException("current must point to an existing directory directly under $root/releases");
        }
    } elseif (file_exists($current)) {
        throw new RuntimeException("Refusing to replace a real current directory or file");
    }
    if (file_exists($release) || is_link($release)) {
        throw new RuntimeException("Release already exists: $release");
    }
    $upload = "$root/.shopware-cli/$id.tar.gz";
    $output = fopen($upload, 'x+b');
    if ($output === false) {
        $upload = null;
        throw new RuntimeException("Cannot create archive upload");
    }
    try {
        if (stream_copy_to_stream(STDIN, $output, $input['size']) !== $input['size']) {
            throw new RuntimeException("Incomplete archive upload");
        }
    } finally {
        fclose($output);
    }
    if (!hash_equals($input['sha256'], hash_file('sha256', $upload))) {
        throw new RuntimeException("Archive checksum mismatch");
    }
    deploymentDirectory($release);
    $metadataFile = "$root/.shopware-cli/rollouts/$id.json";
    $metadata = [
        'reference' => $id,
        'deployment' => $input['deployment'],
        'sha256' => $input['sha256'],
        'created_at' => gmdate('c'),
        'status' => 'preparing',
    ];
    deploymentMetadata($metadataFile, $metadata);
    deploymentRun(['tar', '-xzf', $upload, '--no-same-owner', '--no-same-permissions', '-C', $release], $root);
    // Keep cache/build output release-local. Share runtime data and credentials.
    foreach ($input['shared_directories'] ?? [] as $relative) {
        deploymentShare($root, $release, $relative, true, $previous);
    }
    foreach ($input['shared_files'] ?? [] as $relative) {
        deploymentShare($root, $release, $relative, false, $previous);
    }
    putenv('APP_ENV=prod');
    putenv('APP_DEBUG=0');
    // Let the helper validate runtime configuration using Symfony's full dotenv
    // cascade, rather than requiring one particular shared configuration file.
    // Archives omit local .env files. Symfony still needs a base file before
    // loading .env.local, including for web requests after activation.
    if (!file_exists("$release/.env") && !file_exists("$release/.env.dist")
        && file_put_contents("$release/.env", "APP_ENV=prod\nAPP_DEBUG=0\n") === false) {
        throw new RuntimeException("Cannot create production dotenv defaults");
    }
    if (!is_file("$release/vendor/bin/shopware-deployment-helper") || !is_file("$release/bin/console")) {
        throw new RuntimeException("Archive must include vendor/bin/shopware-deployment-helper and bin/console");
    }
    deploymentRun([$input['php'], 'vendor/bin/shopware-deployment-helper', 'run', '--no-interaction'], $release);
    $shareInstallLock = in_array('install.lock', $input['shared_files'] ?? [], true);
    $installLock = $shareInstallLock ? "$root/shared/install.lock" : "$release/install.lock";
    if (!touch($installLock)) {
        throw new RuntimeException("Cannot write install.lock");
    }
    if ($shareInstallLock) {
        deploymentShare($root, $release, 'install.lock', false, $previous);
    }
    deploymentRun([$input['php'], 'bin/console', 'system:check', '--context=pre_rollout', '--no-interaction'], $release);
    $metadata['status'] = 'prepared';
    deploymentMetadata($metadataFile, $metadata);
    echo "READY $id\n";
    fflush(STDOUT);
    // No late activation after an upload client disconnects or is cancelled.
    if (rtrim((string) fgets(STDIN), "\r\n") !== "ACTIVATE $id") {
        throw new RuntimeException("Client did not authorize activation");
    }
    $candidate = "$root/.current-$id";
    if (!symlink("releases/$id", $candidate)) {
        throw new RuntimeException("Cannot create activation symlink");
    }
    $temporaryLink = $candidate;
    if (!rename($temporaryLink, $current)) {
        throw new RuntimeException("Cannot atomically replace current");
    }
    $temporaryLink = null;
    $activated = true;
    $metadata['status'] = 'successful';
    try {
        deploymentMetadata($metadataFile, $metadata);
    } catch (Throwable $error) {
        // Activation has committed. Do not misreport it as a failed rollout.
        fwrite(STDERR, "Activated $id, but could not finalize rollout metadata: {$error->getMessage()}\n");
    }
    echo "ACTIVE $id\n";
    fflush(STDOUT);
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");
    if (!$activated && $metadataFile !== null) {
        $metadata['status'] = 'failed';
        try {
            deploymentMetadata($metadataFile, $metadata);
        } catch (Throwable) {
            // Preserve the original failure and leave the release for inspection.
        }
    }
    $exitCode = 1;
} finally {
    if ($upload !== null && file_exists($upload)) {
        unlink($upload);
    }
    if ($temporaryLink !== null && is_link($temporaryLink)) {
        unlink($temporaryLink);
    }
    if (is_resource($lock)) {
        fclose($lock); // Releases the lock, including on failed preparations.
    }
}
exit($exitCode);
