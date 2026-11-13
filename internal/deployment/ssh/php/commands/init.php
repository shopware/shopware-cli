<?php

require_once __DIR__ . '/../bootstrap.php';

$lock = null;

try {
    $input = deploymentInput(stream_get_contents(STDIN));
    $root = deploymentRoot($input);
    $current = deploymentCurrentPath($input, $root);
    umask(0022);
    deploymentDirectory($root);
    deploymentDirectory(deploymentStatePath($root), 0700);
    $lock = deploymentLock($root, 'c');
    $runtime = path_join($root, 'shared', '.env.local');
    $install = deploymentStatePath($root, 'install.env');
    $initialized = deploymentStatePath($root, 'initialized');
    deploymentRegularFile($initialized, 'deployment completion marker');
    $hasShared = deploymentRealDirectory(path_join($root, 'shared'));

    switch ($input['action'] ?? '') {
        case 'inspect':
            $runtimeHash = file_exists($runtime) ? hash_file('sha256', $runtime) : '';

            if ($runtimeHash === false) {
                throw new RuntimeException("Cannot hash deployment configuration: $runtime");
            }

            echo json_encode([
                'initialized' => file_exists($initialized),
                'has_shared_directory' => $hasShared,
                'runtime_sha256' => $runtimeHash,
                'has_current' => is_link($current),
                'has_runtime_config' => file_exists($runtime),
                'has_install_config' => file_exists($install),
                'runtime_keys' => initKeys($runtime),
                'install_keys' => initKeys($install),
            ], JSON_THROW_ON_ERROR);

            break;
        case 'apply':
            // Recheck under the lock: another init may have completed while prompting.
            if (!file_exists($initialized)) {
                deploymentDirectory(path_join($root, 'shared'));

                if (!empty($input['runtime_values'])) {
                    initWrite($runtime, $input['runtime_values']);
                }

                if (!empty($input['install_values'])) {
                    initWrite($install, $input['install_values']);
                }

                initWrite($initialized, ['INITIALIZED' => '1']);
            }

            echo "{\"status\":\"initialized\"}\n";

            break;
        default:
            throw new RuntimeException('Unknown deployment initialization action');
    }
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");

    exit(1);
} finally {
    if (is_resource($lock)) {
        fclose($lock);
    }
}
