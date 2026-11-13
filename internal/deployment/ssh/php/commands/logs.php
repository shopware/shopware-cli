<?php

require_once __DIR__ . '/../bootstrap.php';

$stream = null;
$exitCode = 0;

try {
    $input = deploymentInput($argv[1] ?? null);
    $root = deploymentRoot($input);
    $release = $input['release'] ?? null;

    if (!($input['list'] ?? false) && !deploymentValidName($release)) {
        throw new RuntimeException('Invalid release name');
    }

    $logs = deploymentStatePath($root, 'logs');

    foreach ([$root, deploymentStatePath($root), $logs] as $directory) {
        if (!deploymentRealDirectory($directory)) {
            if ($input['list'] ?? false) {
                echo "[]\n";

                exit(0);
            }

            throw new RuntimeException("No deployment helper log available for release $release (missing $directory)");
        }
    }

    if ($input['list'] ?? false) {
        $references = [];

        foreach (new FilesystemIterator($logs) as $entry) {
            if ($entry->isLink() || !$entry->isFile() || !str_ends_with($entry->getFilename(), '.log')) {
                continue;
            }

            $name = substr($entry->getFilename(), 0, -4);

            if (deploymentValidName($name)) {
                $references[] = $name;
            }
        }

        sort($references, SORT_STRING);
        echo json_encode($references, JSON_THROW_ON_ERROR) . "\n";

        exit(0);
    }

    $file = path_join($logs, "$release.log");

    if (!deploymentRegularFile($file, 'deployment log')) {
        throw new RuntimeException("No deployment helper log available for release $release; preparation may not have reached the helper");
    }

    $stream = @fopen($file, 'rb');

    if ($stream === false) {
        throw new RuntimeException("Cannot open deployment log: $file");
    }

    $stat = fstat($stream);

    if ($stat === false || deploymentFileType($stat['mode']) !== 'file') {
        throw new RuntimeException("Deployment log must be a regular file: $file");
    }

    while (!feof($stream)) {
        $bytes = @fread($stream, 8192);

        if ($bytes === false || ($bytes === '' && !feof($stream))) {
            throw new RuntimeException("Cannot read deployment log: $file");
        }

        deploymentWriteAll(STDOUT, $bytes, 'deployment log to stdout');
    }
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");
    $exitCode = 1;
} finally {
    if (is_resource($stream)) {
        fclose($stream);
    }
}

exit($exitCode);
