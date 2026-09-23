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

function deploymentLog(string $message): void
{
    fwrite(STDERR, "$message\n");
    fflush(STDERR);
}

function deploymentHelperWrite($stream, string $bytes): void
{
    $offset = 0;
    while ($offset < strlen($bytes)) {
        $written = @fwrite($stream, substr($bytes, $offset));
        if ($written === false || $written === 0) {
            throw new RuntimeException('Cannot write deployment helper output');
        }
        $offset += $written;
    }
    if (!@fflush($stream)) {
        throw new RuntimeException('Cannot flush deployment helper output');
    }
}

function deploymentRunHelper(array $command, string $directory, string $root, string $name): void
{
    // This is deliberately only called for a new preparation, never for reuse.
    foreach ([$root, "$root/.shopware-cli"] as $parent) {
        if (is_link($parent) || !is_dir($parent)) {
            throw new RuntimeException("Expected a real directory: $parent");
        }
    }
    $logs = "$root/.shopware-cli/logs";
    deploymentDirectory($logs, 0700);
    if (!chmod($logs, 0700)) {
        throw new RuntimeException("Cannot protect deployment logs: $logs");
    }
    $file = "$logs/$name.log";
    if (is_link($file) || (file_exists($file) && !is_file($file))) {
        throw new RuntimeException("Deployment log must be a regular file: $file");
    }
    // Exclusive creation preserves logs even after an unsuccessful preparation.
    $mask = umask(0077);
    try {
        $log = @fopen($file, 'xb');
    } finally {
        umask($mask);
    }
    if ($log === false) {
        throw new RuntimeException("Cannot create deployment log (it may already exist): $file");
    }
    $process = null;
    $pipes = [];
    try {
        if (!chmod($file, 0600)) {
            throw new RuntimeException("Cannot protect deployment log: $file");
        }
        $process = proc_open($command, [
            0 => ['file', '/dev/null', 'r'],
            1 => ['pipe', 'w'],
            2 => ['redirect', 1],
        ], $pipes, $directory);
        if (!is_resource($process)) {
            throw new RuntimeException('Cannot start deployment helper');
        }
        $streamError = null;
        $targets = ['log' => $log, 'stderr' => STDERR];
        while (!feof($pipes[1])) {
            $bytes = @fread($pipes[1], 8192);
            if ($bytes === false || ($bytes === '' && !feof($pipes[1]))) {
                throw new RuntimeException('Cannot read deployment helper output');
            }
            foreach ($targets as $target => $stream) {
                try {
                    deploymentHelperWrite($stream, $bytes);
                } catch (Throwable $error) {
                    // Keep draining the helper and preserve its log even if the
                    // SSH client's stderr is no longer writable.
                    $streamError ??= new RuntimeException("Cannot stream deployment helper $target: {$error->getMessage()}");
                    unset($targets[$target]);
                }
            }
        }
        fclose($pipes[1]);
        unset($pipes[1]);
        $status = proc_close($process);
        $process = null;
        if ($status !== 0) {
            throw new RuntimeException("Deployment helper failed (exit $status); log: $file");
        }
        if ($streamError !== null) {
            throw $streamError;
        }
    } finally {
        if (is_resource($process)) {
            proc_terminate($process);
        }
        foreach ($pipes as $pipe) {
            if (is_resource($pipe)) {
                fclose($pipe);
            }
        }
        if (is_resource($process)) {
            proc_close($process);
        }
        fclose($log);
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
    if (is_link($file) || (file_exists($file) && !is_file($file))) {
        throw new RuntimeException("Metadata must be a regular file: $file");
    }
    $temporary = $file . '.tmp-' . bin2hex(random_bytes(8));
    $stream = fopen($temporary, 'x');
    if ($stream === false) {
        throw new RuntimeException("Cannot create metadata: $file");
    }
    try {
        $json = json_encode($metadata, JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT);
        if (fwrite($stream, $json) !== strlen($json) || !fflush($stream)) {
            throw new RuntimeException("Cannot write metadata: $file");
        }
        fclose($stream);
        $stream = null;
        if (!rename($temporary, $file)) {
            throw new RuntimeException("Cannot publish metadata: $file");
        }
    } finally {
        if (is_resource($stream)) {
            fclose($stream);
        }
        if (is_file($temporary)) {
            unlink($temporary);
        }
    }
}

function deploymentReadMetadata(string $file): array
{
    if (is_link($file) || !is_file($file)) {
        throw new RuntimeException("Metadata must be a regular file: $file");
    }
    $metadata = json_decode((string) file_get_contents($file), true, 512, JSON_THROW_ON_ERROR);
    if (!is_array($metadata)) {
        throw new RuntimeException("Invalid metadata: $file");
    }
    return $metadata;
}

function deploymentValidName(mixed $name): bool
{
    return is_string($name) && strlen($name) <= 128
        && preg_match('/\A[a-zA-Z0-9][a-zA-Z0-9._-]*\z/', $name) === 1;
}

function deploymentInstallEnvironment(string $file): array
{
    if (!file_exists($file)) {
        return [];
    }
    if (is_link($file) || !is_file($file)) {
        throw new RuntimeException("Installation environment must be a regular file: $file");
    }
    // The initializer writes double-quoted dotenv values. The normal scanner
    // decodes their escaped quotes, dollars, and backslashes while preserving
    // quoted values as strings.
    $values = parse_ini_file($file, false, INI_SCANNER_NORMAL);
    if ($values === false) {
        throw new RuntimeException("Cannot parse installation environment: $file");
    }
    foreach ($values as $key => $value) {
        if ((!str_starts_with($key, 'INSTALL_') && $key !== 'SALES_CHANNEL_URL') || !is_string($value)) {
            throw new RuntimeException("Unsupported installation environment variable: $key");
        }
        putenv("$key=$value");
    }
    return array_keys($values);
}

$upload = null;
$archive = null;
$temporaryLink = null;
$metadataFile = null;
$activated = false;
$lock = null;
$exitCode = 0;
try {
    $input = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);
    $root = $input['root'];
    $id = $input['reference'];
    $name = $input['release'] ?? null;
    if (!deploymentValidName($name) || !deploymentValidName($id)) {
        throw new RuntimeException('Invalid release name or rollout reference');
    }
    $current = "$root/current";
    $release = "$root/releases/$name";
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
    if (!is_string($input['sha256'] ?? null) || !preg_match('/^[a-f0-9]{64}$/', $input['sha256'])) {
        throw new RuntimeException('Invalid deployment archive checksum');
    }
    if (!is_int($input['size'] ?? null) || $input['size'] < 0) {
        throw new RuntimeException('Invalid deployment archive size');
    }
    deploymentDirectory("$root/releases");
    deploymentDirectory("$root/shared");
    deploymentDirectory("$root/.shopware-cli/rollouts", 0700);
    deploymentDirectory("$root/.shopware-cli/releases", 0700);
    deploymentDirectory("$root/.shopware-cli/artifacts", 0700);
    $previous = null;
    if (is_link($current)) {
        $previous = realpath($current);
        if ($previous === false || !is_dir($previous) || dirname($previous) !== realpath("$root/releases")) {
            throw new RuntimeException("current must point to an existing directory directly under $root/releases");
        }
    } elseif (file_exists($current)) {
        throw new RuntimeException("Refusing to replace a real current directory or file");
    }
    $releaseFile = "$root/.shopware-cli/releases/$name.json";
    $reuse = file_exists($release) || is_link($release) || file_exists($releaseFile) || is_link($releaseFile);
    if ($reuse) {
        if (is_link($release) || !is_dir($release)) {
            throw new RuntimeException("Release must be a real directory: $release");
        }
        $releaseMetadata = deploymentReadMetadata($releaseFile);
        if (($releaseMetadata['sha256'] ?? null) !== $input['sha256']) {
            throw new RuntimeException("Release $name already exists with a different checksum");
        }
        if (($releaseMetadata['ready'] ?? false) !== true) {
            throw new RuntimeException("Release $name has incomplete preparation and cannot be reused");
        }
        if ($previous === realpath($release)) {
            $reference = $releaseMetadata['reference'] ?? null;
            if (isset($releaseMetadata['activation_pending']) || !deploymentValidName($reference)) {
                throw new RuntimeException("Cannot establish the last successful activation of release $name");
            }
            $successful = deploymentReadMetadata("$root/.shopware-cli/rollouts/$reference.json");
            if (($successful['status'] ?? null) !== 'successful'
                || ($successful['reference'] ?? null) !== $reference
                || ($successful['release'] ?? null) !== $name
                || ($successful['sha256'] ?? null) !== $input['sha256']
                || !is_string($releaseMetadata['deployed_at'] ?? null)
                || ($successful['deployed_at'] ?? null) !== $releaseMetadata['deployed_at']) {
                throw new RuntimeException("Cannot establish the last successful activation of release $name");
            }
            echo "UNCHANGED $id\n$reference\n";
            fflush(STDOUT);
            exit(0);
        }
    }
    $rolloutFile = "$root/.shopware-cli/rollouts/$id.json";
    if (file_exists($rolloutFile) || is_link($rolloutFile)) {
        throw new RuntimeException("Rollout reference already exists: $id");
    }
    $metadata = [
        'reference' => $id,
        'release' => $name,
        'deployment' => $input['deployment'],
        'archive' => $input['archive'] ?? $input['deployment'],
        'sha256' => $input['sha256'],
        'created_at' => (new DateTimeImmutable('now', new DateTimeZone('UTC')))->format('Y-m-d\TH:i:s.u\Z'),
        'status' => 'preparing',
    ];
    deploymentMetadata($rolloutFile, $metadata);
    $metadataFile = $rolloutFile;
    if ($reuse) {
        echo "REUSE $id\n";
        fflush(STDOUT);
    } else {
        $archive = "$root/.shopware-cli/artifacts/{$input['sha256']}.tar.gz";
        if (is_link($archive) || (file_exists($archive) && !is_file($archive))) {
            throw new RuntimeException("Deployment artifact must be a regular file: $archive");
        }
        $cached = is_file($archive)
            && filesize($archive) === $input['size']
            && hash_equals($input['sha256'], (string) hash_file('sha256', $archive));
        if ($cached) {
            echo "CACHED $id\n";
            fflush(STDOUT);
        } else {
            if (is_file($archive) && !unlink($archive)) {
                throw new RuntimeException("Cannot replace invalid deployment artifact: $archive");
            }
            $upload = $archive . '.tmp-' . bin2hex(random_bytes(8));
            $output = fopen($upload, 'x+b');
            if ($output === false) {
                throw new RuntimeException("Cannot create archive upload");
            }
            if (!chmod($upload, 0600)) {
                fclose($output);
                throw new RuntimeException("Cannot protect archive upload");
            }
            echo "UPLOAD $id\n";
            fflush(STDOUT);
            try {
                if (stream_copy_to_stream(STDIN, $output, $input['size']) !== $input['size']) {
                    throw new RuntimeException("Incomplete archive upload");
                }
            } finally {
                fclose($output);
            }
            if (!hash_equals($input['sha256'], (string) hash_file('sha256', $upload))) {
                throw new RuntimeException("Archive checksum mismatch");
            }
            if (!rename($upload, $archive)) {
                throw new RuntimeException("Cannot publish deployment artifact");
            }
            $upload = null;
        }
        echo "PREPARE $id\n";
        fflush(STDOUT);
        if (rtrim((string) fgets(STDIN), "\r\n") !== "CONTINUE $id") {
            throw new RuntimeException("Client did not authorize release preparation");
        }
        $releaseMetadata = ['release' => $name, 'sha256' => $input['sha256'], 'ready' => false];
        deploymentMetadata($releaseFile, $releaseMetadata);
        deploymentDirectory($release);
        deploymentRun(['tar', '-xzf', $archive, '--no-same-owner', '--no-same-permissions', '-C', $release], $root);
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
        if (!is_file("$release/vendor/bin/shopware-deployment-helper")) {
            throw new RuntimeException("Archive must include vendor/bin/shopware-deployment-helper");
        }
        $installEnvironment = "$root/.shopware-cli/install.env";
        $installEnvironmentKeys = deploymentInstallEnvironment($installEnvironment);
        deploymentLog('Running Shopware Deployment Helper');
        deploymentRunHelper([$input['php'] ?? PHP_BINARY, 'vendor/bin/shopware-deployment-helper', 'run', '--no-interaction'], $release, $root, $name);
        if ($installEnvironmentKeys !== []) {
            if (!unlink($installEnvironment)) {
                throw new RuntimeException("Cannot remove completed installation environment");
            }
            foreach ($installEnvironmentKeys as $key) {
                putenv($key);
            }
        }
        $shareInstallLock = in_array('install.lock', $input['shared_files'] ?? [], true);
        $installLock = $shareInstallLock ? "$root/shared/install.lock" : "$release/install.lock";
        if (!touch($installLock)) {
            throw new RuntimeException("Cannot write install.lock");
        }
        if ($shareInstallLock) {
            deploymentShare($root, $release, 'install.lock', false, $previous);
        }
        $releaseMetadata['ready'] = true;
        deploymentMetadata($releaseFile, $releaseMetadata);
    }
    $metadata['status'] = 'prepared';
    deploymentMetadata($metadataFile, $metadata);
    echo "READY $id\n";
    fflush(STDOUT);
    // No late activation after an upload client disconnects or is cancelled.
    if (rtrim((string) fgets(STDIN), "\r\n") !== "ACTIVATE $id") {
        throw new RuntimeException("Client did not authorize activation");
    }
    // Persist intent before the switch so a finalization failure cannot make a
    // later no-op report an older activation as the current successful rollout.
    $releaseMetadata['activation_pending'] = $id;
    deploymentMetadata($releaseFile, $releaseMetadata);
    $candidate = "$root/.current-$id";
    if (!symlink("releases/$name", $candidate)) {
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
        $metadata['deployed_at'] = (new DateTimeImmutable('now', new DateTimeZone('UTC')))->format('Y-m-d\TH:i:s.u\Z');
        deploymentMetadata($metadataFile, $metadata);
        $releaseMetadata['reference'] = $id;
        $releaseMetadata['deployed_at'] = $metadata['deployed_at'];
        unset($releaseMetadata['activation_pending']);
        deploymentMetadata($releaseFile, $releaseMetadata);
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
