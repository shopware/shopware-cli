<?php

// Functions only: the bootstrap loads this library before any command runs.
function deploymentCachetoolTemporary(string $directory, array &$state): string
{
    $path = $directory . '/.cachetool-' . bin2hex(random_bytes(16));
    $stream = deploymentOpenPrivate($path);

    if ($stream === false) {
        throw new RuntimeException('Cannot create protected CacheTool temporary file');
    }

    $state['temporary'][] = $path;
    fclose($stream);

    if (!chmod($path, 0600)) {
        throw new RuntimeException('Cannot protect CacheTool temporary file');
    }

    return $path;
}

function deploymentCachetoolCleanup(?array $state): void
{
    foreach ($state['temporary'] ?? [] as $path) {
        if (is_file($path) || is_link($path)) {
            @unlink($path);
        }
    }
}

// Resolve existing ancestors even when the final directory does not exist yet.
// Reject parent traversal rather than normalizing it across symlink boundaries.
function deploymentCachetoolPath(string $path, bool $resolve, array $protected = [], int $links = 0): string
{
    if (!str_starts_with($path, '/') || str_contains($path, '\\') || preg_match('/[\x00-\x1f\x7f]/', $path)) {
        throw new RuntimeException('CacheTool requires an absolute remote POSIX path');
    }

    if ($links > 40) {
        throw new RuntimeException('Too many symlinks in CacheTool path');
    }

    $result = '';

    foreach (explode('/', $path) as $part) {
        if ($part === '' || $part === '.') {
            continue;
        }

        if ($part === '..') {
            if ($links === 0) {
                throw new RuntimeException('CacheTool paths must not contain parent traversal');
            }

            // Relative symlink targets may legitimately contain parent components.
            $result = dirname($result);

            continue;
        }

        $result = rtrim($result, '/') . '/' . $part;

        foreach ($protected as $boundary) {
            if ($result === $boundary || str_starts_with($result, $boundary . '/')) {
                throw new RuntimeException("cannot use CacheTool writable path inside releases or the active symlink: $path");
            }
        }

        if ($resolve && is_link($result)) {
            $target = readlink($result);

            if ($target === false || realpath($result) === false) {
                throw new RuntimeException("Cannot resolve CacheTool path: $result");
            }

            // Resolve every hop, not just realpath's final destination: an alias
            // through the active link remains forbidden even if public links out.
            $target = str_starts_with($target, '/') ? $target : dirname($result) . '/' . $target;
            $result = deploymentCachetoolPath($target, true, $protected, $links + 1);
        }
    }

    return $result === '' ? '/' : $result;
}

function deploymentCachetoolExternalPath(string $path, string $root, string $field, string $current = 'current'): string
{
    $lexical = deploymentCachetoolPath($path, false);
    $boundaries = [];

    foreach (array_unique([deploymentCachetoolPath($root, false), deploymentCachetoolPath($root, true)]) as $base) {
        foreach (['releases', $current] as $reserved) {
            $protected = rtrim($base, '/') . '/' . $reserved;
            $boundaries[] = $protected;

            if ($lexical === $protected || str_starts_with($lexical, $protected . '/')) {
                throw new RuntimeException("cannot use CacheTool $field inside releases or the active symlink: $path");
            }
        }
    }

    return deploymentCachetoolPath($path, true, $boundaries);
}

function deploymentCachetoolValidateOptions(array $options): void
{
    foreach (['adapter', 'fcgi', 'fcgi_chroot', 'tmp_dir', 'web_url', 'web_path', 'config', 'version', 'url', 'sha256'] as $key) {
        if (isset($options[$key]) && (!is_string($options[$key]) || str_contains($options[$key], "\0"))) {
            throw new RuntimeException("Invalid CacheTool $key setting");
        }
    }

    $version = $options['version'] ?? '';
    $hash = $options['sha256'] ?? '';
    $url = $options['url'] ?? '';
    $timeout = $options['timeout_seconds'] ?? 30;

    if (!preg_match('/\A[0-9]+\.[0-9]+\.[0-9]+\z/', $version)
        || !preg_match('/\A[a-f0-9]{64}\z/', $hash)
        || !filter_var($url, FILTER_VALIDATE_URL) || parse_url($url, PHP_URL_SCHEME) !== 'https'
        || !is_int($timeout) || $timeout < 1 || $timeout > 3600) {
        throw new RuntimeException('Invalid CacheTool pin or timeout');
    }
}

/** Native CacheTool settings for the FastCGI adapter. */
function deploymentCachetoolFastCGI(array $options): array
{
    if (($options['web_url'] ?? '') !== '' || ($options['web_path'] ?? '') !== '') {
        throw new RuntimeException('CacheTool fcgi adapter cannot use web options');
    }

    $native = ['adapter' => 'fastcgi'];

    if (($options['fcgi'] ?? '') !== '') {
        $native['fastcgi'] = $options['fcgi'];
    }

    if (($options['fcgi_chroot'] ?? '') !== '') {
        $native['fastcgiChroot'] = $options['fcgi_chroot'];
    }

    return $native;
}

/** Native CacheTool settings for the web adapter and its isolated endpoint directory. */
function deploymentCachetoolWeb(array $options, string $root, string $current): array
{
    if (($options['fcgi'] ?? '') !== '' || ($options['fcgi_chroot'] ?? '') !== '') {
        throw new RuntimeException('CacheTool web adapter cannot use fcgi options');
    }

    $webUrl = $options['web_url'] ?? '';

    if (!filter_var($webUrl, FILTER_VALIDATE_URL)
        || !in_array(parse_url($webUrl, PHP_URL_SCHEME), ['http', 'https'], true)
        || parse_url($webUrl, PHP_URL_QUERY) !== null
        || parse_url($webUrl, PHP_URL_FRAGMENT) !== null) {
        throw new RuntimeException('CacheTool web adapter requires an HTTP(S) web_url without query or fragment');
    }

    $webPath = $options['web_path'] ?? '';

    if ($webPath === '') {
        throw new RuntimeException('cannot use CacheTool web adapter without an explicit web_path outside releases and the active symlink');
    }

    $webPath = deploymentCachetoolExternalPath($webPath, $root, 'web_path', $current);

    if (!is_dir($webPath) || !is_writable($webPath)) {
        throw new RuntimeException('CacheTool web_path must be an existing writable external directory');
    }

    // Give CacheTool an isolated namespace so we can also clean up its
    // endpoint when the process is terminated before its own cleanup.
    $webName = 'shopware-cli-cachetool-' . bin2hex(random_bytes(16));
    $webDirectory = rtrim($webPath, '/') . '/' . $webName;

    return [[
        'adapter' => 'web',
        'webClient' => 'SymfonyHttpClient',
        'webUrl' => rtrim($webUrl, '/') . '/' . $webName,
        'webPath' => $webDirectory,
    ], $webDirectory];
}

function deploymentCachetoolConfigure(array $options, string $root, string $current): array
{
    $config = $options['config'] ?? '';
    $native = [];
    $webDirectory = null;

    if ($config !== '') {
        foreach (['adapter', 'fcgi', 'fcgi_chroot', 'tmp_dir', 'web_url', 'web_path'] as $key) {
            if (($options[$key] ?? '') !== '') {
                throw new RuntimeException('CacheTool config cannot be combined with inline options');
            }
        }

        deploymentRegularFile($config, 'CacheTool file');

        if (!str_starts_with($config, '/') || !is_file($config) || !is_readable($config)) {
            throw new RuntimeException('CacheTool config must be an existing readable absolute remote file');
        }

        // Native YAML must select fastcgi or web, not cli. CacheTool parses it;
        // do not implement a partial YAML parser here or expose its credentials.
        // Its owner must configure writable external web/temp paths. We cannot
        // preflight values hidden in native YAML and never reopen sealed code.
        // Its owner also manages web endpoint cleanup after HTTP failures.
    } else {
        [$native, $webDirectory] = match ($options['adapter'] ?? 'fcgi') {
            'fcgi' => [deploymentCachetoolFastCGI($options), null],
            'web' => deploymentCachetoolWeb($options, $root, $current),
            default => throw new RuntimeException('CacheTool adapter must be fcgi or web'),
        };

        if (($options['tmp_dir'] ?? '') !== '') {
            deploymentCachetoolExternalPath($options['tmp_dir'], $root, 'tmp_dir', $current);
            $native['temp_dir'] = $options['tmp_dir'];
        }
    }

    return [$config, $native, $webDirectory];
}

function deploymentCachetoolInstall(array $options, string $root, array &$state): string
{
    $version = $options['version'];
    $hash = $options['sha256'];
    $url = $options['url'];
    $timeout = $state['timeout'];

    foreach ([$root, deploymentStatePath($root)] as $directory) {
        if (is_link($directory) || !is_dir($directory)) {
            throw new RuntimeException("CacheTool requires a real management directory: $directory");
        }
    }

    $tools = $state['directory'];
    deploymentDirectory($tools, 0700);

    if (!chmod($tools, 0700)) {
        throw new RuntimeException('Cannot protect CacheTool tools directory');
    }

    $phar = path_join($tools, "cachetool-$version.phar");
    deploymentRegularFile($phar, 'CacheTool file');

    if (!is_file($phar) || !hash_equals($hash, (string) hash_file('sha256', $phar))) {
        $download = deploymentCachetoolTemporary($tools, $state);
        deploymentRunBoundedCommand([
            'curl', '--disable', '--fail', '--silent', '--show-error', '--location',
            '--proto', '=https', '--proto-redir', '=https',
            '--connect-timeout', (string) min(10, $timeout), '--max-time', (string) $timeout,
            '--output', $download, '--url', $url,
        ], $tools, $timeout);
        deploymentRegularFile($download, 'CacheTool file');

        if (!hash_equals($hash, (string) hash_file('sha256', $download))) {
            throw new RuntimeException('CacheTool download checksum mismatch');
        }

        deploymentRegularFile($phar, 'CacheTool file');

        if (!rename($download, $phar)) {
            throw new RuntimeException('Cannot publish verified CacheTool PHAR');
        }
    }

    if (!chmod($phar, 0600)) {
        throw new RuntimeException('Cannot protect CacheTool PHAR');
    }

    return $phar;
}

function deploymentCachetoolPrepare(array $input, string $root, string $release): ?array
{
    $current = basename(deploymentCurrentPath($input, $root));
    $options = $input['cachetool'] ?? null;

    if ($options === null || (is_array($options) && ($options['enabled'] ?? false) === false)) {
        return null;
    }

    if (!is_array($options) || ($options['enabled'] ?? null) !== true) {
        throw new RuntimeException('Invalid CacheTool enabled setting');
    }

    deploymentCachetoolValidateOptions($options);
    [$config, $native, $webDirectory] = deploymentCachetoolConfigure($options, $root, $current);
    $state = ['temporary' => [], 'directory' => deploymentStatePath($root, 'tools'), 'timeout' => $options['timeout_seconds'] ?? 30,
        'web_directory' => $webDirectory];

    try {
        $phar = deploymentCachetoolInstall($options, $root, $state);
        $tools = $state['directory'];

        if ($config === '') {
            $config = deploymentCachetoolTemporary($tools, $state);
            $json = json_encode($native, JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT);

            if (file_put_contents($config, $json) !== strlen($json)) {
                throw new RuntimeException('Cannot write explicit CacheTool config');
            }
        }

        $state['command'] = [
            $input['php'] ?? PHP_BINARY, ...($input['php_args'] ?? []), $phar, 'opcache:reset',
            '--no-interaction', '--no-ansi', '--config', $config,
        ];
        $state['phar'] = $phar;
        $state['sha256'] = $options['sha256'];

        return $state;
    } catch (Throwable $error) {
        deploymentCachetoolCleanup($state);

        throw $error;
    }
}

function deploymentCachetoolReset(?array $state): void
{
    if ($state === null) {
        return;
    }

    deploymentLog('Resetting OPcache with CacheTool');
    deploymentRegularFile($state['phar'], 'CacheTool file');

    if (!is_file($state['phar']) || !hash_equals($state['sha256'], (string) hash_file('sha256', $state['phar']))) {
        throw new RuntimeException('CacheTool PHAR checksum mismatch before execution');
    }

    $webDirectory = $state['web_directory'];
    $webCreated = false;

    try {
        if ($webDirectory !== null) {
            if (!str_starts_with($webDirectory, '/') || is_link(dirname($webDirectory))
                || !is_dir(dirname($webDirectory)) || !@mkdir($webDirectory, 0755)) {
                throw new RuntimeException('Cannot create isolated CacheTool web endpoint directory');
            }

            $webCreated = true;
        }

        deploymentRunBoundedCommand($state['command'], $state['directory'], $state['timeout']);
    } finally {
        if ($webCreated && !is_link($webDirectory) && is_dir($webDirectory)) {
            // Never glob the public tree or remove endpoints from other runs.
            foreach (new FilesystemIterator($webDirectory) as $entry) {
                if ($entry->isFile() || $entry->isLink()) {
                    if (!@unlink($entry->getPathname())) {
                        throw new RuntimeException("Cannot remove CacheTool web endpoint from $webDirectory");
                    }
                }
            }

            if (!@rmdir($webDirectory)) {
                throw new RuntimeException("Cannot remove CacheTool web endpoint directory: $webDirectory");
            }
        }
    }

    deploymentLog('OPcache reset with CacheTool completed');
}
