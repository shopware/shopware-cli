<?php

function deploymentShare(string $root, string $release, string $relative, bool $directory, ?string $previous, bool $coordinated = false): void
{
    $shared = path_join($root, 'shared', $relative);
    $source = path_join($release, $relative);
    deploymentParents(path_join($root, 'shared'), $relative);
    deploymentParents($release, $relative);

    if (is_link($shared)) {
        throw new RuntimeException("Shared paths must not be symlinks: $shared");
    }

    if (!file_exists($shared)) {
        if ($coordinated && ($directory || file_exists($source) || is_link($source))) {
            throw new RuntimeException("Provision shared path before multi-host deployment: $shared");
        }

        // Never move or copy data out of an application that is still serving
        // traffic. Adoption of an existing layout requires explicit migration.
        if ($previous !== null && (file_exists(path_join($previous, $relative)) || is_link(path_join($previous, $relative)))) {
            throw new RuntimeException("Migrate $previous/$relative into $shared before deploying");
        }

        if (file_exists($source) && !is_link($source)) {
            if (is_dir($source) !== $directory || !rename($source, $shared)) {
                throw new RuntimeException("Cannot initialize shared path: $shared");
            }
        } elseif ($directory) {
            deploymentDirectory($shared);
        } else {
            return; // Optional files are not created with empty contents.
        }
    }

    if (($directory && !is_dir($shared)) || (!$directory && !is_file($shared))) {
        throw new RuntimeException("Unexpected shared path type: $shared");
    }

    deploymentRemove($source);

    if (!symlink($shared, $source)) {
        throw new RuntimeException("Cannot link shared path: $source");
    }
}
