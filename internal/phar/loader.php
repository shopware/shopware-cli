<?php

(static function () {
    global $argv, $argc;

    if (!extension_loaded('phar')) {
        fwrite(STDERR, "Cannot run the embedded PHP payload: the phar extension is not loaded\n");

        exit(1);
    }

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
        // PHPArgs substitutes the archive size; leave subsequent input for the entrypoint.
        if (stream_copy_to_stream(STDIN, $output, __SHOPWARE_CLI_PHAR_SIZE__) !== __SHOPWARE_CLI_PHAR_SIZE__) {
            throw new RuntimeException('Cannot write temporary phar');
        }
    } finally {
        fclose($output);
    }

    $argv[0] = $archive;
    $_SERVER['argv'] = $argv;
    require $archive;
})();
