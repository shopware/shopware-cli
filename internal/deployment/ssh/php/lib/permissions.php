<?php

// Runtime data lives outside the sealed release. Never follow a symlink while
// inspecting or changing permissions: shared paths belong to the application.
function deploymentPermissionStat(string $path): array
{
    clearstatcache(true, $path);
    $stat = @lstat($path);

    if ($stat === false) {
        throw new RuntimeException("Cannot inspect release permissions: $path");
    }

    return $stat;
}

/** Name the file type encoded in a stat mode; anything unexpected is "special". */
function deploymentFileType(int $mode): string
{
    return match ($mode & 0170000) {
        0040000 => 'directory',
        0100000 => 'file',
        0120000 => 'link',
        default => 'special',
    };
}

function deploymentPermissionDirectory(string $path): void
{
    if (deploymentFileType(deploymentPermissionStat($path)['mode']) !== 'directory') {
        throw new RuntimeException("Expected a real runtime directory: $path");
    }
}

function deploymentRuntimeDirectoryAccess(string $path, bool $makeWritable = false): void
{
    deploymentPermissionDirectory($path);
    $mode = deploymentPermissionStat($path)['mode'] & 07777;

    if ($makeWritable && ($mode & 0700) !== 0700) {
        if (!chmod($path, $mode | 0700)) {
            throw new RuntimeException("Cannot prepare writable runtime directory: $path");
        }

        clearstatcache(true, $path);
        $mode |= 0700;
    }

    if (($mode & 0700) !== 0700 || !is_writable($path)) {
        throw new RuntimeException("Runtime directory must be writable: $path");
    }
}

function deploymentValidateCacheTree(string $path, bool $makeWritable = false): void
{
    switch (deploymentFileType(deploymentPermissionStat($path)['mode'])) {
        case 'directory':
            deploymentRuntimeDirectoryAccess($path, $makeWritable);

            foreach (new FilesystemIterator($path) as $entry) {
                deploymentValidateCacheTree($entry->getPathname(), $makeWritable);
            }

            break;
        case 'file':
        case 'link':
            break;
        default:
            throw new RuntimeException("Runtime cache contains a special file: $path");
    }
}

function deploymentVerifyRuntimeCache(string $root, string $release, string $name): void
{
    $directories = [
        $root,
        path_join($root, 'runtime'),
        path_join($root, 'runtime', $name),
        path_join($root, 'runtime', $name, 'cache'),
        $release,
        path_join($release, 'var'),
    ];

    foreach ($directories as $directory) {
        deploymentPermissionDirectory($directory);
    }

    deploymentRuntimeDirectoryAccess(path_join($root, 'runtime'));
    deploymentRuntimeDirectoryAccess(path_join($root, 'runtime', $name));
    $cache = path_join($root, 'runtime', $name, 'cache');

    if (!is_link(path_join($release, 'var', 'cache')) || readlink(path_join($release, 'var', 'cache')) !== $cache) {
        throw new RuntimeException("Release cache must link to $cache");
    }

    deploymentValidateCacheTree($cache);
}

// Called only for inactive releases, under the deployment lock and after
// persisting ready=false. Only runtime directories gain owner access; never
// chmod cached files, which might be hard-linked to application code.
function deploymentRuntimeCache(string $root, string $release, string $name, bool $reuse = false): void
{
    foreach ([$root, $release] as $directory) {
        deploymentPermissionDirectory($directory);
    }

    deploymentDirectory(path_join($root, 'runtime'));
    deploymentRuntimeDirectoryAccess(path_join($root, 'runtime'), true);
    deploymentDirectory(path_join($root, 'runtime', $name));
    deploymentRuntimeDirectoryAccess(path_join($root, 'runtime', $name), true);
    $cache = path_join($root, 'runtime', $name, 'cache');
    $source = path_join($release, 'var', 'cache');

    if (is_link($cache) || (file_exists($cache) && !is_dir($cache))) {
        throw new RuntimeException("Expected a real runtime cache directory: $cache");
    }

    if (is_link(path_join($release, 'var')) || (file_exists(path_join($release, 'var')) && !is_dir(path_join($release, 'var')))) {
        throw new RuntimeException("Expected a real release var directory: $release/var");
    }

    if (is_link($source)) {
        // Only our own, exact runtime link is accepted on a reused release.
        if (!$reuse) {
            throw new RuntimeException("Archive cache must not be a symlink: $source");
        }

        if (readlink($source) !== $cache) {
            throw new RuntimeException("Release cache must link to $cache");
        }

        deploymentPermissionDirectory($cache);
        deploymentValidateCacheTree($cache, true);
        deploymentVerifyRuntimeCache($root, $release, $name);

        return;
    }

    if (file_exists($source)) {
        deploymentPermissionDirectory($source);

        if (file_exists($cache)) {
            throw new RuntimeException("Both release and runtime caches exist; refusing to overwrite: $source");
        }

        deploymentValidateCacheTree($source, true);
    } elseif (file_exists($cache)) {
        // Safe to resume a rename interrupted before the symlink was created.
        deploymentValidateCacheTree($cache, true);
    }

    // Readonly archive directories and partially sealed inactive releases may
    // need owner write permission briefly to install the cache link. Never add
    // read access, and restore these modes even when migration fails.
    $releaseMode = deploymentPermissionStat($release)['mode'] & 07777;
    $varMode = null;

    try {
        if (!is_dir(path_join($release, 'var'))) {
            if (!chmod($release, $releaseMode | 0200)) {
                throw new RuntimeException("Cannot prepare release runtime directory: $release");
            }

            deploymentDirectory(path_join($release, 'var'));
        }

        $varMode = deploymentPermissionStat(path_join($release, 'var'))['mode'] & 07777;

        if (!chmod(path_join($release, 'var'), $varMode | 0200)) {
            throw new RuntimeException("Cannot prepare release cache link: $source");
        }

        if (is_dir($source)) {
            if (!rename($source, $cache)) {
                throw new RuntimeException("Cannot relocate release cache: $source");
            }
        } else {
            deploymentDirectory($cache);
        }

        if (!symlink($cache, $source)) {
            throw new RuntimeException("Cannot link release cache: $source");
        }
    } finally {
        if ($varMode !== null && !chmod(path_join($release, 'var'), $varMode)) {
            throw new RuntimeException("Cannot restore release directory permissions: $release/var");
        }

        if (!chmod($release, $releaseMode)) {
            throw new RuntimeException("Cannot restore release directory permissions: $release");
        }
    }

    deploymentVerifyRuntimeCache($root, $release, $name);
}

function deploymentReleasePermissions(string $path, bool $seal): void
{
    $stat = deploymentPermissionStat($path);

    switch (deploymentFileType($stat['mode'])) {
        case 'link':
            return;
        case 'directory':
            foreach (new FilesystemIterator($path) as $entry) {
                deploymentReleasePermissions($entry->getPathname(), $seal);
            }

            break;
        case 'file':
            break;
        default:
            throw new RuntimeException("Release contains a special file: $path");
    }

    if (($stat['mode'] & 0222) !== 0) {
        if (!$seal) {
            throw new RuntimeException("Release entry is writable: $path");
        }

        if (!chmod($path, ($stat['mode'] & 07777) & ~0222)) {
            throw new RuntimeException("Cannot seal release entry: $path");
        }
    }
}

function deploymentVerifySealedRelease(string $root, string $release, string $name): void
{
    try {
        deploymentVerifyRuntimeCache($root, $release, $name);
        deploymentReleasePermissions($release, false);
    } catch (Throwable $error) {
        throw new RuntimeException("Active release $name is not protected by the readonly runtime-cache policy; deploy a new release name. {$error->getMessage()}", 0, $error);
    }
}
