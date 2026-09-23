<?php
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

function initEncode(string $value): string
{
    return '"' . str_replace(
        ["\\", '"', '$', "\r", "\n"],
        ["\\\\", '\\"', '\$', '\r', '\n'],
        $value
    ) . '"';
}

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
            $content = (string) preg_replace($pattern, $line, $content, 1);
        } else {
            if ($content !== '' && !str_ends_with($content, "\n")) {
                $content .= "\n";
            }
            $content .= $line . "\n";
        }
    }
    return $content;
}

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
    if (($input['action'] ?? '') === 'inspect') {
        $runtimeKeys = initKeys($runtime);
        $installKeys = initKeys($install);
        echo json_encode([
            'has_current' => is_link("$root/current"),
            'has_runtime_config' => file_exists($runtime),
            'has_install_config' => file_exists($install),
            'runtime_keys' => $runtimeKeys,
            'install_keys' => $installKeys,
        ], JSON_THROW_ON_ERROR);
    } elseif (($input['action'] ?? '') === 'apply') {
        initDirectory("$root/shared");
        if (!empty($input['runtime_values'])) {
            initWrite($runtime, $input['runtime_values']);
        }
        if (!empty($input['install_values'])) {
            initWrite($install, $input['install_values']);
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
