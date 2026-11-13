<?php

/** Decode the JSON options sent by the Go client. */
function deploymentInput(string|false|null $json): array
{
    $input = json_decode((string) $json, true, 512, JSON_THROW_ON_ERROR);

    if (!is_array($input)) {
        throw new RuntimeException('Invalid deployment input');
    }

    return $input;
}

/** The directory that holds releases, shared data and deployment state. */
function deploymentRoot(array $input): string
{
    $root = $input['root'] ?? null;

    if (!is_string($root) || !str_starts_with($root, '/') || str_contains($root, "\0") || rtrim($root, '/') === '') {
        throw new RuntimeException('Invalid deployment root: expected an absolute directory other than /');
    }

    return rtrim($root, '/');
}

/** A path inside the management directory that stores metadata, logs and artifacts. */
function deploymentStatePath(string $root, string ...$parts): string
{
    return path_join($root, '.shopware-cli', ...$parts);
}

// Keep the active link beside deployment storage, never inside it.
function deploymentCurrentPath(array $input, string $root): string
{
    $name = $input['current'] ?? 'current';

    if ($root === '' || $root === '/' || $root[0] !== '/'
        || !is_string($name) || $name === '' || $name === '.' || $name === '..'
        || strpbrk($name, "/\\") !== false || preg_match('/[\x00-\x1f\x7f]/', $name)
        || in_array($name, ['releases', 'shared', 'runtime', '.shopware-cli'], true)
        || str_starts_with($name, '.current-')) {
        throw new RuntimeException('cannot resolve deployment layout: invalid active symlink path');
    }

    return path_join($root, $name);
}

/**
 * Resolve the release the active symlink points to, or null before the first
 * activation. Only a link to a real direct child of releases is accepted.
 */
function deploymentActiveRelease(string $root, string $current): ?string
{
    if (!is_link($current)) {
        if (file_exists($current)) {
            throw new RuntimeException("cannot use active path $current: it must be a symlink, not a real directory or file");
        }

        return null;
    }

    $active = realpath($current);
    $target = (string) readlink($current);
    $target = str_starts_with($target, '/') ? $target : path_join(dirname($current), $target);

    if ($active === false || !is_dir($active) || is_link($target) || dirname($active) !== realpath(path_join($root, 'releases'))) {
        throw new RuntimeException("cannot use active symlink $current: it must point to an existing directory directly under $root/releases");
    }

    return $active;
}

/**
 * Take the exclusive, non-blocking deployment lock. Closing the returned
 * stream releases it. Mode "c" creates the lock file; "r" requires it.
 *
 * @return resource
 */
function deploymentLock(string $root, string $mode)
{
    $path = deploymentStatePath($root, 'deployment.lock');
    deploymentRegularFile($path, 'deployment lock');
    $lock = @fopen($path, $mode);

    if ($lock === false) {
        throw new RuntimeException("Cannot open deployment lock: $path");
    }

    if (!flock($lock, LOCK_EX | LOCK_NB)) {
        fclose($lock);

        throw new RuntimeException("Another deployment holds the lock for $root");
    }

    return $lock;
}
