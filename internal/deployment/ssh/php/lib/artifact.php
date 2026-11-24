<?php

/**
 * Return the cached deployment archive, receiving it from stdin when the cache
 * has no verified copy. $upload names the partial file while one exists so the
 * caller can remove it after a failure.
 */
function deploymentReceiveArtifact(array $input, string $root, string $id, ?string &$upload): string
{
    $archive = deploymentStatePath($root, 'artifacts', "{$input['sha256']}.tar.gz");

    if (deploymentRegularFile($archive, 'deployment artifact')) {
        if (filesize($archive) === $input['size'] && hash_equals($input['sha256'], (string) hash_file('sha256', $archive))) {
            deploymentSend("CACHED $id");

            return $archive;
        }

        if (!unlink($archive)) {
            throw new RuntimeException("Cannot replace invalid deployment artifact: $archive");
        }
    }

    $path = $archive . '.tmp-' . bin2hex(random_bytes(8));
    $output = deploymentOpenPrivate($path, 'x+b');

    if ($output === false) {
        throw new RuntimeException('Cannot create archive upload');
    }

    $upload = $path;
    deploymentSend("UPLOAD $id");

    try {
        if (stream_copy_to_stream(STDIN, $output, $input['size']) !== $input['size']) {
            throw new RuntimeException('Incomplete archive upload');
        }
    } finally {
        fclose($output);
    }

    if (!hash_equals($input['sha256'], (string) hash_file('sha256', $upload))) {
        throw new RuntimeException('Archive checksum mismatch');
    }

    if (!rename($upload, $archive)) {
        throw new RuntimeException('Cannot publish deployment artifact');
    }

    $upload = null;

    return $archive;
}
