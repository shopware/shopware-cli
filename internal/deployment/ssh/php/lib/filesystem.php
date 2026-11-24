<?php

/**
 * Join POSIX path components, ignoring empty parts and trimming boundary slashes.
 * Later components are appended even when they start with a slash. Dot segments
 * and symlinks are not resolved; callers must validate untrusted paths separately.
 */
function path_join(string $base, string ...$parts): string
{
    $path = $base;

    foreach ($parts as $part) {
        if ($part === '') {
            continue;
        }

        $path = $path === '' ? $part : rtrim($path, '/') . '/' . ltrim($part, '/');
    }

    $trimmed = rtrim($path, '/');

    return $trimmed === '' && $path !== '' ? '/' : $trimmed;
}

/** Reject symlinks and non-directories; report whether the directory exists. */
function deploymentRealDirectory(string $path): bool
{
    if (is_link($path) || (file_exists($path) && !is_dir($path))) {
        throw new RuntimeException("Expected a real directory: $path");
    }

    return is_dir($path);
}

/**
 * Reject symlinks and non-files; report whether the file exists. A required
 * file must exist.
 */
function deploymentRegularFile(string $path, string $label, bool $required = false): bool
{
    clearstatcache(true, $path);

    if (is_link($path) || (file_exists($path) ? !is_file($path) : $required)) {
        throw new RuntimeException(ucfirst($label) . " must be a regular file: $path");
    }

    return is_file($path);
}

function deploymentDirectory(string $directory, int $mode = 0755): void
{
    if (!deploymentRealDirectory($directory) && !mkdir($directory, $mode, true)) {
        throw new RuntimeException("Cannot create directory: $directory");
    }
}

function deploymentParents(string $base, string $relative): void
{
    $directory = $base;

    foreach (explode('/', dirname($relative)) as $part) {
        if ($part !== '.') {
            $directory = path_join($directory, $part);
            deploymentDirectory($directory);
        }
    }
}

function deploymentRemove(string $path): void
{
    if (is_link($path) || is_file($path)) {
        if (!unlink($path)) {
            throw new RuntimeException("Cannot remove release entry: $path");
        }
    } elseif (is_dir($path)) {
        foreach (new FilesystemIterator($path) as $entry) {
            deploymentRemove($entry->getPathname());
        }

        if (!rmdir($path)) {
            throw new RuntimeException("Cannot remove release directory: $path");
        }
    }
}

/**
 * Create a new owner-only file. Returns false when it cannot be created, for
 * example because it already exists.
 *
 * @return resource|false
 */
function deploymentOpenPrivate(string $path, string $mode = 'xb')
{
    $mask = umask(0077);

    try {
        return @fopen($path, $mode);
    } finally {
        umask($mask);
    }
}

/** Write every byte to a stream that may accept only part of a buffer. */
function deploymentWriteAll($stream, string $bytes, string $label): void
{
    $offset = 0;

    while ($offset < strlen($bytes)) {
        $written = @fwrite($stream, substr($bytes, $offset));

        if ($written === false || $written === 0) {
            throw new RuntimeException("Cannot write $label");
        }

        $offset += $written;
    }

    if (!@fflush($stream)) {
        throw new RuntimeException("Cannot flush $label");
    }
}

/**
 * Publish complete contents with a rename, so readers never observe a partial
 * file. A mode makes the file private from creation onwards. The marker names
 * the temporary file; retention relies on it to recognize abandoned writes.
 */
function deploymentWriteAtomic(string $file, string $content, string $label, ?int $mode = null, string $marker = 'tmp'): void
{
    deploymentRegularFile($file, $label);
    $temporary = "$file.$marker-" . bin2hex(random_bytes(8));
    $stream = $mode === null ? fopen($temporary, 'xb') : deploymentOpenPrivate($temporary);

    if ($stream === false) {
        throw new RuntimeException("Cannot create $label: $file");
    }

    try {
        if (($mode !== null && !chmod($temporary, $mode))
            || fwrite($stream, $content) !== strlen($content) || !fflush($stream) || !fclose($stream)) {
            throw new RuntimeException("Cannot write $label: $file");
        }

        if (!rename($temporary, $file)) {
            throw new RuntimeException("Cannot publish $label: $file");
        }
    } finally {
        if (is_resource($stream)) {
            fclose($stream);
        }

        if (is_file($temporary)) {
            unlink($temporary);
        }
    }
}
