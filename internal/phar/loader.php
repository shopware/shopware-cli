<?php
(static function () {
    global $argv, $argc;

    $directory = sys_get_temp_dir() . '/shopware-cli-' . bin2hex(random_bytes(16));
    if (!mkdir($directory, 0700)) {
        throw new RuntimeException('Cannot create temporary phar directory');
    }

    $archive = $directory . '/payload.phar';
    register_shutdown_function(static function () use ($archive, $directory) {
        if (file_exists($archive)) {
            unlink($archive);
        }
        rmdir($directory);
    });

    $output = fopen($archive, 'wb');
    if ($output === false) {
        throw new RuntimeException('Cannot create temporary phar');
    }
    try {
        if (stream_copy_to_stream(STDIN, $output) === false) {
            throw new RuntimeException('Cannot write temporary phar');
        }
    } finally {
        fclose($output);
    }

    $argv[0] = $archive;
    $_SERVER['argv'] = $argv;
    require $archive;
})();
