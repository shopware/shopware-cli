<?php
// This script is passed to PHP -r and emits only JSON on stdout.
$exitCode = 0;
try {
    $input = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);
    $root = $input['root'] ?? null;
    if (!is_string($root) || $root === '' || $root[0] !== '/') {
        throw new RuntimeException('Invalid deployment root');
    }
    if (is_link($root)) {
        throw new RuntimeException("Deployment root must not be a symlink: $root");
    }
    if (!file_exists($root)) {
        echo "[]\n";
        exit(0);
    }
    if (!is_dir($root)) {
        throw new RuntimeException("Deployment root must be a real directory: $root");
    }

    $releases = "$root/releases";
    $rolloutsDirectory = "$root/.shopware-cli/rollouts";
    $releaseMetadataDirectory = "$root/.shopware-cli/releases";
    foreach ([$releases, "$root/.shopware-cli", $releaseMetadataDirectory] as $directory) {
        if (is_link($directory) || (file_exists($directory) && !is_dir($directory))) {
            throw new RuntimeException("Expected a real directory: $directory");
        }
    }
    $active = null;
    $current = "$root/current";
    if (is_link($current)) {
        $activePath = realpath($current);
        $releasesPath = realpath($releases);
        if ($activePath === false || !is_dir($activePath) || $releasesPath === false || dirname($activePath) !== $releasesPath) {
            throw new RuntimeException("current must point to an existing directory directly under $releases");
        }
        $active = basename($activePath);
    } elseif (file_exists($current)) {
        throw new RuntimeException('current must be a symlink');
    }

    $activeRecord = null;
    if ($active !== null) {
        $file = "$releaseMetadataDirectory/$active.json";
        if (is_link($file) || (file_exists($file) && !is_file($file))) {
            throw new RuntimeException("Release metadata must be a regular file: $file");
        }
        if (is_file($file)) {
            $activeRecord = json_decode((string) file_get_contents($file), true, 512, JSON_THROW_ON_ERROR);
            if (!is_array($activeRecord) || isset($activeRecord['activation_pending'])
                || ($activeRecord['ready'] ?? false) !== true
                || ($activeRecord['release'] ?? null) !== $active
                || !is_string($activeRecord['reference'] ?? null) || $activeRecord['reference'] === ''
                || !is_string($activeRecord['deployed_at'] ?? null) || $activeRecord['deployed_at'] === ''
                || !is_string($activeRecord['sha256'] ?? null)) {
                throw new RuntimeException("Cannot establish the last successful activation of release $active");
            }
        }
    }

    $rollouts = [];
    if (is_link($rolloutsDirectory)) {
        throw new RuntimeException("Rollout metadata path must not be a symlink: $rolloutsDirectory");
    }
    if (is_dir($rolloutsDirectory)) {
        foreach (new FilesystemIterator($rolloutsDirectory) as $file) {
            if ($file->isLink() || !$file->isFile() || $file->getExtension() !== 'json') {
                continue;
            }
            $metadata = json_decode((string) file_get_contents($file->getPathname()), true, 512, JSON_THROW_ON_ERROR);
            if (($metadata['status'] ?? null) !== 'successful') {
                continue;
            }
            $reference = $metadata['reference'] ?? null;
            $deployment = $metadata['deployment'] ?? null;
            $createdAt = $metadata['created_at'] ?? null;
            if (!is_string($reference) || $reference === '' || basename($reference) !== $reference
                || !is_string($deployment) || $deployment === ''
                || !is_string($createdAt) || $createdAt === '') {
                throw new RuntimeException("Invalid rollout metadata: {$file->getPathname()}");
            }
            // Older releases only recorded when preparation started, not activation.
            $deployedAt = $metadata['deployed_at'] ?? null;
            if ($deployedAt !== null && (!is_string($deployedAt) || $deployedAt === '')) {
                throw new RuntimeException("Invalid deployment timestamp: {$file->getPathname()}");
            }
            $release = $metadata['release'] ?? $reference;
            if (!is_string($release) || $release === '' || basename($release) !== $release) {
                throw new RuntimeException("Invalid release metadata: {$file->getPathname()}");
            }
            $isActive = false;
            if ($release === $active) {
                if ($activeRecord !== null) {
                    $isActive = $reference === $activeRecord['reference'];
                    if ($isActive && ($deployedAt !== $activeRecord['deployed_at']
                        || ($metadata['sha256'] ?? null) !== $activeRecord['sha256'])) {
                        throw new RuntimeException("Inconsistent activation metadata for release $active");
                    }
                } elseif (isset($metadata['release'])) {
                    // Named releases must never fall back to an earlier activation.
                    throw new RuntimeException("Missing activation metadata for release $active");
                } else {
                    // Legacy releases used the unique rollout ID as the directory.
                    $isActive = $reference === $active;
                }
            }
            $rollouts[] = [
                'reference' => $reference,
                'deployment' => ['reference' => $deployment],
                'active' => $isActive,
                'deployed_at' => $deployedAt,
                '_sort_at' => new DateTimeImmutable($deployedAt ?? $createdAt),
            ];
        }
    } elseif (file_exists($rolloutsDirectory)) {
        throw new RuntimeException("Rollout metadata path must be a real directory: $rolloutsDirectory");
    }

    usort($rollouts, static fn (array $left, array $right): int => $right['_sort_at'] <=> $left['_sort_at']);
    $activeFound = false;
    foreach ($rollouts as &$rollout) {
        if ($rollout['active']) {
            $activeFound = true;
        }
        unset($rollout['_sort_at']);
    }
    unset($rollout);
    if ($activeRecord !== null && !$activeFound) {
        throw new RuntimeException("Missing successful rollout metadata for active release $active");
    }
    echo json_encode($rollouts, JSON_THROW_ON_ERROR) . "\n";
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");
    $exitCode = 1;
}
exit($exitCode);
