<?php

require_once __DIR__ . '/../bootstrap.php';

// Archive bytes and activation messages arrive on stdin; stdout is the handshake.
$upload = null;
$archive = null;
$temporaryLink = null;
$metadataFile = null;
$activated = false;
$hooksStarted = false;
$postHooksAttempted = false;
$lock = null;
$exitCode = 0;
$cachetool = null;

try {
    $input = deploymentInput($argv[1] ?? null);
    $root = deploymentRoot($input);
    $hookTimeout = $input['hooks']['timeout_seconds'] ?? 0;

    if (!is_int($hookTimeout) || $hookTimeout < 0) {
        throw new RuntimeException('cannot configure hook timeout: must be zero or positive');
    }
    $hookTimeout = $hookTimeout ?: 300;
    $action = $input['action'] ?? '';

    if (!in_array($action, ['', 'activate', 'candidates'], true)) {
        throw new RuntimeException('Invalid deployment action');
    }

    $id = $input['reference'] ?? null;
    $name = $input['release'] ?? null;

    if ($action !== 'candidates' && (!deploymentValidName($name) || !deploymentValidName($id))) {
        throw new RuntimeException('Invalid release name or rollout reference');
    }

    $current = deploymentCurrentPath($input, $root);
    $release = $action === 'candidates' ? null : path_join($root, 'releases', $name);
    $sameActive = $action !== 'candidates' && is_link($current)
        && realpath($current) !== false && realpath($current) === realpath($release);
    $directories = [
        $root,
        path_join($root, 'releases'),
        path_join($root, 'shared'),
        deploymentStatePath($root),
        deploymentStatePath($root, 'releases'),
        deploymentStatePath($root, 'rollouts'),
        deploymentStatePath($root, 'artifacts'),
    ];

    foreach ($directories as $directory) {
        deploymentRealDirectory($directory);
    }

    umask(0022);

    if ($action === '' && !$sameActive) {
        deploymentDirectory($root);
        deploymentDirectory(deploymentStatePath($root), 0700);
    } else {
        if (!is_dir(deploymentStatePath($root))) {
            if ($action === 'candidates' && !file_exists($current) && !is_link($current)) {
                echo "[]\n";

                exit(0);
            }

            throw new RuntimeException('Missing deployment management storage');
        }
    }

    $lock = deploymentLock($root, $action === 'candidates' || $sameActive ? 'r' : 'c');

    if ($action === '' && !deploymentValidChecksum($input['sha256'] ?? null)) {
        throw new RuntimeException('Invalid deployment archive checksum');
    }

    if ($action === '' && (!is_int($input['size'] ?? null) || $input['size'] < 0)) {
        throw new RuntimeException('Invalid deployment archive size');
    }

    $previous = deploymentActiveRelease($root, $current);

    if ($action === 'candidates') {
        deploymentSend(json_encode(deploymentCandidates($root, $previous), JSON_THROW_ON_ERROR));

        exit(0);
    }

    if ($action === 'activate') {
        $retained = deploymentRetained($root, $name);

        if (deploymentCoordinated($input) && ($input['sha256'] ?? null) !== $retained['sha256']) {
            throw new RuntimeException('Retained deployment checksum differs across hosts');
        }

        // Identity and provenance belong to the retained deployment, not this client.
        foreach (['sha256', 'deployment', 'archive'] as $field) {
            $input[$field] = $retained[$field];
        }
    }

    $releaseFile = deploymentStatePath($root, 'releases', "$name.json");
    $reuse = file_exists($release) || is_link($release) || file_exists($releaseFile) || is_link($releaseFile);

    if ($reuse) {
        if (is_link($release) || !is_dir($release)) {
            throw new RuntimeException("Release must be a real directory: $release");
        }

        $releaseMetadata = deploymentReadMetadata($releaseFile);

        if (($releaseMetadata['sha256'] ?? null) !== $input['sha256']) {
            throw new RuntimeException("Release $name already exists with a different checksum");
        }

        if (($releaseMetadata['ready'] ?? false) !== true || array_key_exists('activation_pending', $releaseMetadata)
            || array_key_exists('pruning', $releaseMetadata)) {
            throw new RuntimeException("Release $name has incomplete preparation and cannot be reused");
        }

        if ($previous === realpath($release)) {
            $reference = $releaseMetadata['reference'] ?? null;

            if (isset($releaseMetadata['activation_pending']) || !deploymentValidName($reference)) {
                throw new RuntimeException("Cannot establish the last successful activation of release $name");
            }

            $successful = deploymentReadMetadata(deploymentStatePath($root, 'rollouts', "$reference.json"));

            if (($successful['status'] ?? null) !== 'successful'
                || ($successful['reference'] ?? null) !== $reference
                || ($successful['release'] ?? null) !== $name
                || ($successful['sha256'] ?? null) !== $input['sha256']
                || !is_string($releaseMetadata['deployed_at'] ?? null)
                || ($successful['deployed_at'] ?? null) !== $releaseMetadata['deployed_at']) {
                throw new RuntimeException("Cannot establish the last successful activation of release $name");
            }

            deploymentVerifySealedRelease($root, $release, $name);
            deploymentSend("UNCHANGED $id\n$reference");
            deploymentActivationResult($input, $successful, true);
            deploymentAwaitCohort($input);

            exit(0);
        }
    }

    $rolloutFile = deploymentStatePath($root, 'rollouts', "$id.json");

    if (file_exists($rolloutFile) || is_link($rolloutFile)) {
        throw new RuntimeException("Rollout reference already exists: $id");
    }

    if ($action === '') {
        deploymentDirectory(path_join($root, 'releases'));
        deploymentDirectory(path_join($root, 'shared'));
        deploymentDirectory(deploymentStatePath($root, 'rollouts'), 0700);
        deploymentDirectory(deploymentStatePath($root, 'releases'), 0700);
        deploymentDirectory(deploymentStatePath($root, 'artifacts'), 0700);
    }

    // Same-active and candidate requests have already returned. Prepare before
    // rollout metadata, release writes, or migrations can change application state.
    $cachetool = deploymentCachetoolPrepare($input, $root, $release);
    $metadata = [
        'reference' => $id,
        'release' => $name,
        'deployment' => $input['deployment'],
        'archive' => $input['archive'] ?? $input['deployment'],
        'sha256' => $input['sha256'],
        'created_at' => deploymentTimestamp(),
        'status' => 'preparing',
    ];
    deploymentWriteMetadata($rolloutFile, $metadata);
    $metadataFile = $rolloutFile;

    if ($reuse) {
        deploymentSend("REUSE $id");
        deploymentAwaitPreparation($input, $id);

        $releaseMetadata['ready'] = false;
        deploymentWriteMetadata($releaseFile, $releaseMetadata);
        deploymentRuntimeCache($root, $release, $name, true);
    } else {
        $archive = deploymentReceiveArtifact($input, $root, $id, $upload);
        deploymentSend("PREPARE $id");
        deploymentExpect("CONTINUE $id", 'Client did not authorize release preparation');
        $releaseMetadata = [
            'release' => $name, 'sha256' => $input['sha256'], 'ready' => false,
            'created_at' => $metadata['created_at'],
        ];
        deploymentWriteMetadata($releaseFile, $releaseMetadata);
        deploymentDirectory($release);
        deploymentRun(['tar', '-xzf', $archive, '--no-same-owner', '--no-same-permissions', '-C', $release], $root);
        deploymentRuntimeCache($root, $release, $name);

        // Persistent shared data is separate from host- and release-local cache.
        foreach ($input['shared_directories'] ?? [] as $relative) {
            deploymentShare($root, $release, $relative, true, $previous, deploymentCoordinated($input));
        }

        foreach ($input['shared_files'] ?? [] as $relative) {
            deploymentShare($root, $release, $relative, false, $previous, deploymentCoordinated($input));
        }

        putenv('APP_ENV=prod');
        putenv('APP_DEBUG=0');

        // Let the helper validate runtime configuration using Symfony's full dotenv
        // cascade, rather than requiring one particular shared configuration file.
        // Archives omit local .env files. Symfony still needs a base file before
        // loading .env.local, including for web requests after activation.
        if (!file_exists(path_join($release, '.env')) && !file_exists(path_join($release, '.env.dist'))
            && file_put_contents(path_join($release, '.env'), "APP_ENV=prod\nAPP_DEBUG=0\n") === false) {
            throw new RuntimeException("Cannot create production dotenv defaults");
        }

        if (!is_file(path_join($release, 'vendor', 'bin', 'shopware-deployment-helper'))) {
            throw new RuntimeException("Archive must include vendor/bin/shopware-deployment-helper");
        }

        deploymentAwaitPreparation($input, $id);

        if ($input['follower'] ?? false) {
            deploymentRun(deploymentPHPCommand($input, 'bin/console', 'cache:warmup', '--no-interaction'), $release);
        } else {
            $installEnvironment = deploymentStatePath($root, 'install.env');
            $installEnvironmentKeys = deploymentInstallEnvironment($installEnvironment);
            deploymentLog('Running Shopware Deployment Helper');
            deploymentRunHelper(deploymentPHPCommand($input, 'vendor/bin/shopware-deployment-helper', 'run', '--no-interaction'), $release, $root, $name);

            if ($installEnvironmentKeys !== []) {
                if (!unlink($installEnvironment)) {
                    throw new RuntimeException("Cannot remove completed installation environment");
                }

                foreach ($installEnvironmentKeys as $key) {
                    putenv($key);
                }
            }
        }

        $shareInstallLock = in_array('install.lock', $input['shared_files'] ?? [], true);
        $installLock = $shareInstallLock ? path_join($root, 'shared', 'install.lock') : path_join($release, 'install.lock');

        if (!touch($installLock)) {
            throw new RuntimeException("Cannot write install.lock");
        }

        if ($shareInstallLock) {
            deploymentShare($root, $release, 'install.lock', false, $previous);
        }
    }

    deploymentVerifyRuntimeCache($root, $release, $name);
    deploymentReleasePermissions($release, true);
    $releaseMetadata['ready'] = true;
    deploymentWriteMetadata($releaseFile, $releaseMetadata);
    $metadata['status'] = 'prepared';
    deploymentWriteMetadata($metadataFile, $metadata);
    deploymentSend("READY $id");
    // No late activation after an upload client disconnects or is cancelled.
    deploymentExpect("ACTIVATE $id", 'Client did not authorize activation');

    if (!empty($input['hooks']['pre']) || !empty($input['hooks']['post'])) {
        // Verify timeout support before a pre-hook can stop workers.
        deploymentRun(['timeout', '--kill-after=5s', '1s', 'sh', '-c', 'exit 0'], $root);
    }
    $hooksStarted = true;

    foreach ($input['hooks']['pre'] ?? [] as $script) {
        deploymentRunHook($script, $root, $hookTimeout);
    }

    // Persist intent before the switch so a finalization failure cannot make a
    // later no-op report an older activation as the current successful rollout.
    $releaseMetadata['created_at'] ??= deploymentReleaseCreatedAt($root, $name, $releaseMetadata) ?? $metadata['created_at'];
    $releaseMetadata['activation_pending'] = $id;
    deploymentWriteMetadata($releaseFile, $releaseMetadata);
    $candidate = path_join($root, ".current-$id");

    if (!symlink("releases/$name", $candidate)) {
        throw new RuntimeException("Cannot create activation symlink");
    }

    $temporaryLink = $candidate;

    if (!rename($temporaryLink, $current)) {
        throw new RuntimeException("cannot atomically replace active symlink $current");
    }

    $temporaryLink = null;
    $activated = true;
    $metadata['status'] = 'successful';

    try {
        $metadata['deployed_at'] = deploymentTimestamp();
        deploymentWriteMetadata($metadataFile, $metadata);
        $releaseMetadata['reference'] = $id;
        $releaseMetadata['deployed_at'] = $metadata['deployed_at'];
        unset($releaseMetadata['activation_pending']);
        deploymentWriteMetadata($releaseFile, $releaseMetadata);
    } catch (Throwable $error) {
        // Activation has committed. Do not misreport it as a failed rollout.
        deploymentLog("Activated $id, but could not finalize rollout metadata: {$error->getMessage()}");
    }

    try {
        if ($cachetool !== null) {
            deploymentCachetoolReset($cachetool);
        } elseif (($input['probe_php_host'] ?? false) === true) {
            deploymentRestartPHPProcesses();
        }
    } catch (Throwable $error) {
        // The switch has committed; a reset failure must not undo readiness.
        deploymentLog("Deployment activated, but OPcache reset failed: {$error->getMessage()}");
    }

    $postHooksAttempted = true;
    deploymentRunPostHooks($input['hooks']['post'] ?? [], $root, $hookTimeout);
    deploymentSend("ACTIVE $id");
    deploymentActivationResult($input, $metadata, false);
    // Activation is committed; disconnect cleanup must not relabel it failed.
    deploymentAwaitCohort($input);
} catch (Throwable $error) {
    deploymentLog($error->getMessage());

    if (!$activated && $metadataFile !== null) {
        $metadata['status'] = 'failed';

        try {
            deploymentWriteMetadata($metadataFile, $metadata);
        } catch (Throwable) {
            // Preserve the original failure and leave the release for inspection.
        }
    }

    $exitCode = 1;
} finally {
    if ($hooksStarted && !$postHooksAttempted) {
        try {
            deploymentRunPostHooks($input['hooks']['post'] ?? [], $root, $hookTimeout);
        } catch (Throwable $error) {
            deploymentLog($error->getMessage());
            $exitCode = 1;
        }
    }

    deploymentCachetoolCleanup($cachetool);

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
