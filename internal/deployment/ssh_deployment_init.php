<?php
error_reporting(E_ALL);
ini_set('display_errors', 'stderr');
ini_set('html_errors', '0');
ini_set('log_errors', '0');

/** Create a real directory with the requested permissions, rejecting symlinks. */
function initDirectory(string $directory, int $mode = 0755): void
{
    if (is_link($directory) || (file_exists($directory) && !is_dir($directory))) {
        throw new RuntimeException("Expected a real directory: $directory");
    }
    if (!is_dir($directory)) {
        if (!mkdir($directory, $mode, true) || !chmod($directory, $mode)) {
            throw new RuntimeException("Cannot create directory: $directory");
        }
    }
}

/** Read environment key names without exposing their values. */
function initKeys(string $file): array
{
    if (!file_exists($file) && !is_link($file)) {
        return [];
    }
    if (is_link($file) || !is_file($file)) {
        throw new RuntimeException("Deployment configuration must be a regular file: $file");
    }
    $content = file_get_contents($file);
    if ($content === false) {
        throw new RuntimeException("Cannot read deployment configuration: $file");
    }
    preg_match_all('/^\s*(?:export\s+)?([A-Z_][A-Z0-9_]*)\s*=/m', $content, $matches);
    $keys = array_values(array_unique($matches[1]));
    sort($keys);
    return $keys;
}

/** Quote a value for dotenv while preserving literal special characters. */
function initEncode(string $value): string
{
    return '"' . str_replace(
        ["\\", '"', '$', "\r", "\n"],
        ["\\\\", '\\"', '\$', '\r', '\n'],
        $value
    ) . '"';
}

/** Replace existing keys literally and append missing keys, preserving unrelated content. */
function initMerge(string $content, array $values): string
{
    ksort($values);
    foreach ($values as $key => $value) {
        if (!preg_match('/^[A-Z_][A-Z0-9_]*$/', (string) $key) || !is_string($value)) {
            throw new RuntimeException("Invalid environment value");
        }
        $line = $key . '=' . initEncode($value);
        $pattern = '/^\s*(?:export\s+)?' . preg_quote($key, '/') . '\s*=.*$/m';
        if (preg_match($pattern, $content)) {
            $content = (string) preg_replace_callback($pattern, static fn (): string => $line, $content, 1);
        } else {
            if ($content !== '' && !str_ends_with($content, "\n")) {
                $content .= "\n";
            }
            $content .= $line . "\n";
        }
    }
    return $content;
}

/** Publish a complete configuration file atomically with owner-only permissions. */
function initWrite(string $file, array $values): void
{
    if (is_link($file) || (file_exists($file) && !is_file($file))) {
        throw new RuntimeException("Deployment configuration must be a regular file: $file");
    }
    $content = file_exists($file) ? file_get_contents($file) : '';
    if ($content === false) {
        throw new RuntimeException("Cannot read deployment configuration: $file");
    }
    $temporary = $file . '.tmp-' . bin2hex(random_bytes(8));
    $handle = fopen($temporary, 'x');
    if ($handle === false) {
        throw new RuntimeException("Cannot create temporary deployment configuration");
    }
    $published = false;
    try {
        if (!chmod($temporary, 0600)) {
            throw new RuntimeException("Cannot protect deployment configuration: $file");
        }
        $merged = initMerge($content, $values);
        $written = fwrite($handle, $merged);
        if ($written === false || $written !== strlen($merged) || !fflush($handle)) {
            throw new RuntimeException("Cannot write deployment configuration: $file");
        }
        if (!fclose($handle)) {
            throw new RuntimeException("Cannot close deployment configuration: $file");
        }
        $handle = null;
        if (!rename($temporary, $file)) {
            throw new RuntimeException("Cannot publish deployment configuration: $file");
        }
        $published = true;
    } finally {
        if (is_resource($handle)) {
            fclose($handle);
        }
        if (!$published && file_exists($temporary)) {
            unlink($temporary);
        }
    }
}

$lock = null;
try {
    $input = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
    $root = $input['root'] ?? '';
    if (!is_string($root) || $root === '' || $root[0] !== '/') {
        throw new RuntimeException('Deployment root must be absolute');
    }
    $current = deploymentCurrentPath($input, $root);
    initDirectory($root);
    initDirectory("$root/.shopware-cli", 0700);
    $lockPath = "$root/.shopware-cli/deployment.lock";
    if (is_link($lockPath) || (file_exists($lockPath) && !is_file($lockPath))) {
        throw new RuntimeException('Deployment lock must be a regular file');
    }
    $lock = fopen($lockPath, 'c');
    if ($lock === false || !flock($lock, LOCK_EX | LOCK_NB)) {
        throw new RuntimeException("Another deployment holds the lock for $root");
    }
    $runtime = "$root/shared/.env.local";
    $install = "$root/.shopware-cli/install.env";
    $initialized = "$root/.shopware-cli/initialized";
    if (is_link($initialized) || (file_exists($initialized) && !is_file($initialized))) {
        throw new RuntimeException("cannot initialize deployment: completion marker must be a regular file: $initialized");
    }
    if (is_link("$root/shared") || (file_exists("$root/shared") && !is_dir("$root/shared"))) {
        throw new RuntimeException("Expected a real directory: $root/shared");
    }
    if (($input['action'] ?? '') === 'inspect') {
        $runtimeKeys = initKeys($runtime);
        $installKeys = initKeys($install);
        $runtimeHash = file_exists($runtime) ? hash_file('sha256', $runtime) : '';
        if ($runtimeHash === false) {
            throw new RuntimeException("Cannot hash deployment configuration: $runtime");
        }
        echo json_encode([
            'initialized' => file_exists($initialized),
            'has_shared_directory' => is_dir("$root/shared"),
            'runtime_sha256' => $runtimeHash,
            'has_current' => is_link($current),
            'has_runtime_config' => file_exists($runtime),
            'has_install_config' => file_exists($install),
            'runtime_keys' => $runtimeKeys,
            'install_keys' => $installKeys,
        ], JSON_THROW_ON_ERROR);
    } elseif (($input['action'] ?? '') === 'apply') {
        // Recheck under the lock: another init may have completed while prompting.
        if (!file_exists($initialized)) {
            initDirectory("$root/shared");
            if (!empty($input['runtime_values'])) {
                initWrite($runtime, $input['runtime_values']);
            }
            if (!empty($input['install_values'])) {
                initWrite($install, $input['install_values']);
            }
            initWrite($initialized, ['INITIALIZED' => '1']);
        }
        echo "{\"status\":\"initialized\"}\n";
    } else {
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
