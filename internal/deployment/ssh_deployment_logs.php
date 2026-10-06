<?php
// Passed to PHP -r; stdout contains only the original helper output bytes.
$stream = null;
$exitCode = 0;
try {
    $input = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);
    $root = $input['root'] ?? null;
    $release = $input['release'] ?? null;
    if (!is_string($root) || $root === '' || $root[0] !== '/' || str_contains($root, "\0")) {
        throw new RuntimeException('Invalid deployment root');
    }
    if (!is_string($release) || preg_match('/\A[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}\z/', $release) !== 1) {
        throw new RuntimeException('Invalid release name');
    }
    $root = rtrim($root, '/') ?: '/';
    foreach ([$root, "$root/.shopware-cli", "$root/.shopware-cli/logs"] as $directory) {
        if (is_link($directory) || (file_exists($directory) && !is_dir($directory))) {
            throw new RuntimeException("Expected a real directory: $directory");
        }
        if (!is_dir($directory)) {
            throw new RuntimeException("No deployment helper log available for release $release (missing $directory)");
        }
    }
    $file = "$root/.shopware-cli/logs/$release.log";
    if (is_link($file) || (file_exists($file) && !is_file($file))) {
        throw new RuntimeException("Deployment log must be a regular file: $file");
    }
    if (!is_file($file)) {
        throw new RuntimeException("No deployment helper log available for release $release; preparation may not have reached the helper");
    }
    $stream = @fopen($file, 'rb');
    if ($stream === false) {
        throw new RuntimeException("Cannot open deployment log: $file");
    }
    $stat = fstat($stream);
    if ($stat === false || ($stat['mode'] & 0170000) !== 0100000) {
        throw new RuntimeException("Deployment log must be a regular file: $file");
    }
    while (!feof($stream)) {
        $bytes = @fread($stream, 8192);
        if ($bytes === false || ($bytes === '' && !feof($stream))) {
            throw new RuntimeException("Cannot read deployment log: $file");
        }
        $offset = 0;
        while ($offset < strlen($bytes)) {
            $written = @fwrite(STDOUT, substr($bytes, $offset));
            if ($written === false || $written === 0) {
                throw new RuntimeException('Cannot stream deployment log to stdout');
            }
            $offset += $written;
        }
        if (!@fflush(STDOUT)) {
            throw new RuntimeException('Cannot flush deployment log to stdout');
        }
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
