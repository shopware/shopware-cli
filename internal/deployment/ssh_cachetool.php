<?php
// Functions only: this file is prepended to the remote deployment script.
function deploymentCachetoolRegular(string $path): void
{
    clearstatcache(true, $path);
    if (is_link($path) || (file_exists($path) && !is_file($path))) {
        throw new RuntimeException("CacheTool requires a regular file: $path");
    }
}

function deploymentCachetoolTemporary(string $directory, array &$state): string
{
    $path = $directory . '/.cachetool-' . bin2hex(random_bytes(16));
    $mask = umask(0077);
    try {
        $stream = @fopen($path, 'xb');
    } finally {
        umask($mask);
    }
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

function deploymentWriteResetOutput(string $bytes, int $deadline): bool
{
    $blocking = stream_get_meta_data(STDERR)['blocked'] ?? true;
    if (!@stream_set_blocking(STDERR, false)) {
        return false;
    }
    try {
        $offset = 0;
        while ($offset < strlen($bytes)) {
            if (hrtime(true) >= $deadline) {
                return false;
            }
            $written = @fwrite(STDERR, substr($bytes, $offset));
            if ($written === false) {
                return false;
            }
            if ($written === 0) {
                usleep(10000);
                continue;
            }
            $offset += $written;
        }
        return true;
    } finally {
        @stream_set_blocking(STDERR, $blocking);
    }
}

function deploymentReportReset(string $message): void
{
    deploymentWriteResetOutput($message . "\n", hrtime(true) + 250000000);
}

// Bound process execution and output forwarding independently of the SSH reader.
function deploymentRunBoundedCommand(array $command, string $directory, int $timeout, bool $capture = false, array $successCodes = [0]): string
{
    $process = null;
    $pipes = [];
    $output = '';
    try {
        $process = proc_open($command, [
            0 => ['file', '/dev/null', 'r'],
            1 => ['pipe', 'w'],
            2 => ['redirect', 1],
        ], $pipes, $directory);
        if (!is_resource($process)) {
            throw new RuntimeException('Cannot start deployment command');
        }
        stream_set_blocking($pipes[1], false);
        $deadline = hrtime(true) + $timeout * 1000000000;
        $exit = null;
        while (true) {
            $bytes = fread($pipes[1], 8192);
            if ($bytes === false) {
                throw new RuntimeException('Cannot read deployment command output');
            }
            if ($capture) {
                if (strlen($output) + strlen($bytes) > 1048576) {
                    throw new RuntimeException('Deployment command output exceeds 1 MiB');
                }
                $output .= $bytes;
            } elseif ($bytes !== '' && !deploymentWriteResetOutput($bytes, $deadline)) {
                throw new RuntimeException('Deployment command output blocked or unavailable');
            }
            $status = proc_get_status($process);
            if (!$status['running'] && $exit === null) {
                $exit = $status['exitcode'];
            }
            if (!$status['running'] && feof($pipes[1])) {
                break;
            }
            if (hrtime(true) >= $deadline) {
                throw new RuntimeException("Deployment command timed out after $timeout seconds");
            }
            if ($bytes === '') {
                usleep(10000);
            }
        }
        fclose($pipes[1]);
        $pipes = [];
        $closed = proc_close($process);
        $process = null;
        $exit = $exit === null || $exit < 0 ? $closed : $exit;
        if (!in_array($exit, $successCodes, true)) {
            throw new RuntimeException(basename($command[0]) . " failed (exit $exit)");
        }
        return $output;
    } finally {
        if (is_resource($process)) {
            // Do not wait indefinitely for a helper that ignores SIGTERM.
            proc_terminate($process);
            $grace = hrtime(true) + 200000000;
            do {
                if (isset($pipes[1])) {
                    @fread($pipes[1], 8192);
                }
                $running = proc_get_status($process)['running'];
                if (!$running) {
                    break;
                }
                usleep(10000);
            } while (hrtime(true) < $grace);
            if ($running) {
                proc_terminate($process, 9);
            }
        }
        foreach ($pipes as $pipe) {
            fclose($pipe);
        }
        if (is_resource($process)) {
            proc_close($process);
        }
    }
}

function deploymentCachetoolPrepare(array $input, string $root, string $release): ?array
{
    $options = $input['cachetool'] ?? null;
    if ($options === null || (is_array($options) && ($options['enabled'] ?? false) === false)) {
        return null;
    }
    if (!is_array($options) || ($options['enabled'] ?? null) !== true) {
        throw new RuntimeException('Invalid CacheTool enabled setting');
    }
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
    $config = $options['config'] ?? '';
    $native = [];
    $webDirectory = null;
    if ($config !== '') {
        foreach (['adapter', 'fcgi', 'fcgi_chroot', 'tmp_dir', 'web_url', 'web_path'] as $key) {
            if (($options[$key] ?? '') !== '') {
                throw new RuntimeException('CacheTool config cannot be combined with inline options');
            }
        }
        deploymentCachetoolRegular($config);
        if (!str_starts_with($config, '/') || !is_file($config) || !is_readable($config)) {
            throw new RuntimeException('CacheTool config must be an existing readable absolute remote file');
        }
        // Native YAML must select fastcgi or web, not cli. CacheTool parses it;
        // do not implement a partial YAML parser here or expose its credentials.
        // Its owner also manages web endpoint cleanup after HTTP failures.
    } else {
        $adapter = $options['adapter'] ?? 'fcgi';
        if (!in_array($adapter, ['fcgi', 'web'], true)) {
            throw new RuntimeException('CacheTool adapter must be fcgi or web');
        }
        $native['adapter'] = $adapter === 'fcgi' ? 'fastcgi' : 'web';
        if ($adapter === 'web') {
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
            // CacheTool 10 leaves its endpoint behind when HTTP requests fail.
            // Give it an isolated namespace that we can remove safely afterward.
            $webName = 'shopware-cli-cachetool-' . bin2hex(random_bytes(16));
            $webDirectory = rtrim(($options['web_path'] ?? '') ?: "$release/public", '/') . '/' . $webName;
            $native['webClient'] = 'SymfonyHttpClient';
            $native['webUrl'] = rtrim($webUrl, '/') . '/' . $webName;
            $native['webPath'] = $webDirectory;
        } else {
            if (($options['web_url'] ?? '') !== '' || ($options['web_path'] ?? '') !== '') {
                throw new RuntimeException('CacheTool fcgi adapter cannot use web options');
            }
            if (($options['fcgi'] ?? '') !== '') {
                $native['fastcgi'] = $options['fcgi'];
            }
            if (($options['fcgi_chroot'] ?? '') !== '') {
                $native['fastcgiChroot'] = $options['fcgi_chroot'];
            }
        }
        if (($options['tmp_dir'] ?? '') !== '') {
            $native['temp_dir'] = $options['tmp_dir'];
        }
    }
    $state = ['temporary' => [], 'directory' => "$root/.shopware-cli/tools", 'timeout' => $timeout,
        'web_directory' => $webDirectory];
    try {
        foreach ([$root, "$root/.shopware-cli"] as $directory) {
            if (is_link($directory) || !is_dir($directory)) {
                throw new RuntimeException("CacheTool requires a real management directory: $directory");
            }
        }
        $tools = $state['directory'];
        deploymentDirectory($tools, 0700);
        if (!chmod($tools, 0700)) {
            throw new RuntimeException('Cannot protect CacheTool tools directory');
        }
        $phar = "$tools/cachetool-$version.phar";
        deploymentCachetoolRegular($phar);
        if (!is_file($phar) || !hash_equals($hash, (string) hash_file('sha256', $phar))) {
            $download = deploymentCachetoolTemporary($tools, $state);
            deploymentRunBoundedCommand([
                'curl', '--disable', '--fail', '--silent', '--show-error', '--location',
                '--proto', '=https', '--proto-redir', '=https',
                '--connect-timeout', (string) min(10, $timeout), '--max-time', (string) $timeout,
                '--output', $download, '--url', $url,
            ], $tools, $timeout);
            deploymentCachetoolRegular($download);
            if (!hash_equals($hash, (string) hash_file('sha256', $download))) {
                throw new RuntimeException('CacheTool download checksum mismatch');
            }
            deploymentCachetoolRegular($phar);
            if (!rename($download, $phar)) {
                throw new RuntimeException('Cannot publish verified CacheTool PHAR');
            }
        }
        if (!chmod($phar, 0600)) {
            throw new RuntimeException('Cannot protect CacheTool PHAR');
        }
        if ($config === '') {
            $config = deploymentCachetoolTemporary($tools, $state);
            $json = json_encode($native, JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT);
            if (file_put_contents($config, $json) !== strlen($json)) {
                throw new RuntimeException('Cannot write explicit CacheTool config');
            }
        }
        $state['command'] = [
            $input['php'] ?? PHP_BINARY, $phar, 'opcache:reset',
            '--no-interaction', '--no-ansi', '--config', $config,
        ];
        $state['phar'] = $phar;
        $state['sha256'] = $hash;
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
    deploymentReportReset('Resetting OPcache with CacheTool');
    deploymentCachetoolRegular($state['phar']);
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
    deploymentReportReset('OPcache reset with CacheTool completed');
}
