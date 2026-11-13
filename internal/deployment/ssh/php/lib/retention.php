<?php

function pruneRecords(string $directory, string $identity, array &$reasons, array &$temporary): array
{
    $records = [];

    if (!is_dir($directory)) {
        return $records;
    }

    foreach (new FilesystemIterator($directory) as $entry) {
        $filename = $entry->getFilename();

        if (!str_ends_with($filename, '.json')) {
            // Only prune-owned files are disposable. Deployment writes may hold
            // an otherwise unknown checksum and must remain protected.
            if ($identity === 'release'
                && preg_match('/\A([a-zA-Z0-9][a-zA-Z0-9._-]*)\.json\.(prune-tmp|tmp)-[a-f0-9]{16}\z/', $filename, $match) === 1
                && deploymentValidName($match[1]) && !$entry->isLink() && $entry->isFile()) {
                $owned = $match[2] === 'prune-tmp';

                if (!$owned) {
                    $marker = json_decode((string) file_get_contents($entry->getPathname()), true);
                    $published = "$directory/{$match[1]}.json";
                    $record = !is_link($published) && is_file($published)
                        ? json_decode((string) file_get_contents($published), true) : null;
                    $owned = is_array($marker) && count($marker) === 4
                        && ($marker['pruning'] ?? null) === true && ($marker['ready'] ?? null) === false
                        && ($marker['release'] ?? null) === $match[1]
                        && is_string($marker['sha256'] ?? null)
                        && ($record['sha256'] ?? null) === $marker['sha256'];
                }

                if ($owned) {
                    $temporary[] = $entry->getPathname();

                    continue;
                }
            }
            $reasons[] = "Unrecognized metadata file: $directory/$filename";

            continue;
        }

        $name = substr($filename, 0, -5);

        if (!deploymentValidName($name)) {
            throw new RuntimeException("Invalid metadata filename: $filename");
        }

        deploymentRegularFile($entry->getPathname(), 'metadata', true);
        $object = json_decode((string) file_get_contents($entry->getPathname()), false, 512, JSON_THROW_ON_ERROR);

        if (!$object instanceof stdClass) {
            throw new RuntimeException("Invalid metadata: $filename");
        }

        $record = (array) $object;

        foreach (['release', 'reference', 'activation_pending'] as $field) {
            if (array_key_exists($field, $record) && !deploymentValidName($record[$field])) {
                throw new RuntimeException("Invalid $field in $filename");
            }
        }

        // Older callers used a local archive path as the deployment identity.
        foreach (['deployment', 'archive'] as $field) {
            if (array_key_exists($field, $record) && !is_string($record[$field])) {
                throw new RuntimeException("Invalid $field in $filename");
            }
        }

        if (isset($record[$identity]) && $record[$identity] !== $name) {
            throw new RuntimeException("Metadata identity does not match filename: $filename");
        }

        if (array_key_exists('sha256', $record)) {
            if (!deploymentValidChecksum($record['sha256'])) {
                throw new RuntimeException("Invalid checksum in $filename");
            }
        } else {
            $reasons[] = "Missing checksum in metadata: $directory/$filename";
        }

        foreach (['created_at', 'deployed_at'] as $field) {
            if (array_key_exists($field, $record)) {
                deploymentParseTimestamp($record[$field]) ?? throw new RuntimeException("Invalid $field in $filename");
            }
        }

        if (array_key_exists('ready', $record) && !is_bool($record['ready'])) {
            throw new RuntimeException("Invalid ready flag in $filename");
        }

        if (array_key_exists('pruning', $record)
            && (!is_bool($record['pruning']) || ($record['pruning']
                && (($record['ready'] ?? null) !== false || !isset($record['release'], $record['sha256']))))) {
            throw new RuntimeException("Invalid pruning marker in $filename");
        }

        if (array_key_exists('status', $record) && !is_string($record['status'])) {
            throw new RuntimeException("Invalid status in $filename");
        }

        $records[$name] = $record;
    }

    ksort($records, SORT_STRING);

    return $records;
}

function pruneRemoveTree(string $path): void
{
    // Links include shared data and per-release cache; unlink, never traverse.
    if (is_link($path) || is_file($path)) {
        if (!unlink($path)) {
            throw new RuntimeException("Cannot remove release entry: $path");
        }

        return;
    }

    if (!is_dir($path)) {
        throw new RuntimeException("Unexpected release entry: $path");
    }

    // Only directories need write access to unlink children. Preserve all other
    // mode bits, never chmod files or symlink targets, and only do this after
    // publishing the resumable pruning marker (never during plan validation).
    $mode = fileperms($path) & 07777;

    if (($mode & 0700) !== 0700 && !chmod($path, $mode | 0700)) {
        throw new RuntimeException("Cannot restore directory access: $path");
    }

    foreach (new FilesystemIterator($path) as $entry) {
        pruneRemoveTree($entry->getPathname());
    }

    if (!rmdir($path)) {
        throw new RuntimeException("Cannot remove release directory: $path");
    }
}

function pruneValidateTree(string $path): void
{
    foreach (new FilesystemIterator($path) as $entry) {
        if ($entry->isLink() || $entry->isFile()) {
            continue;
        }

        if (!$entry->isDir()) {
            throw new RuntimeException("Unexpected release entry: {$entry->getPathname()}");
        }

        pruneValidateTree($entry->getPathname());
    }
}

function pruneMarkRelease(string $file, string $name, string $checksum): void
{
    deploymentDirectory(dirname($file), 0700);
    $marker = ['release' => $name, 'sha256' => $checksum, 'ready' => false, 'pruning' => true];
    deploymentWriteAtomic($file, json_encode($marker, JSON_THROW_ON_ERROR), "pending cleanup for release $name", null, 'prune-tmp');
}

function prunePlan(array $input, array &$progress): array
{
    $root = deploymentRoot($input);

    if (!is_int($input['keep'] ?? null) || $input['keep'] < 0 || !is_bool($input['dry_run'] ?? null)) {
        throw new RuntimeException('Invalid prune input');
    }

    $current = deploymentCurrentPath($input, $root);
    $result = ['deployments' => [], 'artifacts' => []];
    $state = deploymentStatePath($root);

    if (!deploymentRealDirectory($root) || !deploymentRealDirectory($state)) {
        return $result;
    }

    deploymentRegularFile(path_join($state, 'deployment.lock'), 'deployment lock', true);
    $lock = deploymentLock($root, 'r+');

    try {
        foreach ([path_join($root, 'releases'), path_join($root, 'runtime'), path_join($state, 'releases'), path_join($state, 'rollouts'), path_join($state, 'logs'), path_join($state, 'artifacts')] as $directory) {
            deploymentRealDirectory($directory);
        }

        $active = deploymentActiveRelease($root, $current);
        $active = $active === null ? null : basename($active);

        $reasons = &$progress['artifact_skip_reasons'];
        $temporary = [];
        $records = pruneRecords("$state/releases", 'release', $reasons, $temporary);
        $rollouts = pruneRecords("$state/rollouts", 'reference', $reasons, $temporary);
        $history = [];
        $historyHashes = [];

        foreach ($rollouts as $reference => $record) {
            // Missing identity is incomplete, not proof of a managed legacy release.
            $name = $record['release'] ?? $record['reference'] ?? null;

            if ($name === null) {
                $reasons[] = "Missing release identity in rollout: $reference";

                continue;
            }

            $history[$name][$reference] = $record;

            if (isset($record['sha256'])) {
                if (isset($historyHashes[$name]) && $historyHashes[$name] !== $record['sha256']) {
                    throw new RuntimeException("Inconsistent rollout checksums for release $name");
                }

                $historyHashes[$name] = $record['sha256'];
            }

            if (isset($records[$name]['sha256'], $record['sha256'])
                && $records[$name]['sha256'] !== $record['sha256']) {
                throw new RuntimeException("Inconsistent checksum for release $name");
            }
        }

        // Validate recorded activations even for absent or protected directories.
        $pending = [];

        foreach ($records as $name => $record) {
            if (($record['pruning'] ?? false) === true) {
                if ((string) $name === $active) {
                    throw new RuntimeException("Refusing to resume cleanup of active release $name");
                }

                $pending[] = $name;

                continue;
            }

            if (!isset($record['reference'], $record['deployed_at'], $record['sha256'])) {
                continue;
            }

            $event = $rollouts[$record['reference']] ?? null;

            if ($event === null || ($event['status'] ?? null) !== 'successful'
                || ($event['release'] ?? null) !== (string) $name
                || ($event['sha256'] ?? null) !== $record['sha256']
                || ($event['deployed_at'] ?? null) !== $record['deployed_at']) {
                throw new RuntimeException("Inconsistent activation metadata for release $name");
            }
        }

        $candidates = [];

        if (is_dir(path_join($root, 'releases'))) {
            foreach (new FilesystemIterator(path_join($root, 'releases')) as $entry) {
                $name = $entry->getFilename();

                if (!deploymentValidName($name) || $entry->isLink() || !$entry->isDir()) {
                    $reasons[] = "Unmanaged release entry: $name";

                    continue;
                }

                $record = $records[$name] ?? null;
                $events = $history[$name] ?? [];
                $eligible = false;

                if ($record !== null) {
                    $eligible = ($record['release'] ?? null) === $name && ($record['ready'] ?? false) === true
                        && !array_key_exists('activation_pending', $record)
                        && isset($record['reference'], $record['deployed_at'], $record['sha256']);
                } elseif (isset($events[$name]) && !array_key_exists('release', $events[$name])) {
                    $eligible = true; // Legacy unique rollout directory.
                } else {
                    $reasons[] = "Missing metadata for release: $name";
                }

                $latest = null;

                foreach ($events as $reference => $event) {
                    if (($event['status'] ?? null) !== 'successful'
                        || ($event['reference'] ?? null) !== (string) $reference || !isset($event['sha256'])
                        || (isset($event['release']) && !isset($event['deployed_at']))
                        || (!isset($event['deployed_at']) && !isset($event['created_at']))) {
                        $eligible = false;

                        continue;
                    }

                    $time = deploymentParseTimestamp($event['deployed_at'] ?? $event['created_at']);

                    if ($latest === null || $time > $latest) {
                        $latest = $time;
                    }
                }

                if ($eligible && $latest !== null) {
                    $candidates[$name] = $latest;
                }
            }
        }

        uksort($candidates, static fn ($a, $b) => ($candidates[$b] <=> $candidates[$a]) ?: strcmp((string) $a, (string) $b));
        $remove = array_slice(array_keys($candidates), $input['keep']);
        $remove = array_values(array_filter($remove, static fn ($name) => (string) $name !== $active));
        $remove = array_values(array_unique(array_merge($remove, $pending)));
        sort($remove, SORT_STRING);
        $files = [];
        $removeHashes = [];

        foreach ($remove as $name) {
            // Runtime is owned by the same managed release, not an independent
            // retention candidate. Unknown runtime entries are never inspected.
            foreach ([path_join($root, 'releases', $name), path_join($root, 'runtime', $name)] as $path) {
                if (deploymentRealDirectory($path)) {
                    pruneValidateTree($path);
                }
            }

            $removeHashes[$name] = $records[$name]['sha256'] ?? $historyHashes[$name];
            $files[$name] = [];

            if (isset($records[$name])) {
                unset($records[$name]);
            }

            $logNames = [$name];

            foreach ($history[$name] ?? [] as $reference => $event) {
                $files[$name][path_join($state, 'rollouts', "$reference.json")] = true;
                $logNames[] = $reference;
                unset($rollouts[$reference]);
            }

            foreach ($logNames as $logName) {
                // Do not remove a legacy rollout log that is another retained release's helper log.
                if ($logName != $name && file_exists(path_join($root, 'releases', $logName)) && !in_array($logName, $remove, true)) {
                    continue;
                }

                $path = path_join($state, 'logs', "$logName.log");

                if (deploymentRegularFile($path, 'deployment log')) {
                    $files[$name][$path] = true;
                }
            }

            $result['deployments'][] = ['reference' => (string) $name];
        }

        $retainedHashes = [];

        foreach (array_merge(array_values($records), array_values($rollouts)) as $record) {
            if (isset($record['sha256'])) {
                $retainedHashes[$record['sha256']] = true;
            }
        }
        $reasons = array_values(array_unique($reasons));
        sort($reasons, SORT_STRING);
        $result['artifact_skip_reasons'] = $reasons;

        if (is_dir("$state/artifacts")) {
            foreach (new FilesystemIterator("$state/artifacts") as $entry) {
                if (preg_match('/\A([a-f0-9]{64})\.tar\.gz\z/', $entry->getFilename(), $match) !== 1) {
                    continue;
                }

                deploymentRegularFile($entry->getPathname(), 'cached artifact', true);

                if ($reasons === [] && !isset($retainedHashes[$match[1]])) {
                    $result['artifacts'][] = $entry->getFilename();
                }
            }
        }

        sort($result['artifacts'], SORT_STRING);

        // The entire plan, including logs and cached archives, is validated first.
        if (!$input['dry_run']) {
            foreach ($temporary as $path) {
                if (!unlink($path)) {
                    throw new RuntimeException("Cannot remove abandoned pruning metadata: $path");
                }
            }

            foreach ($remove as $name) {
                // Persist intent first; ready=false also blocks rollout reuse after
                // an interruption. Keep the marker until every related file is gone.
                $marker = path_join($state, 'releases', "$name.json");
                pruneMarkRelease($marker, (string) $name, $removeHashes[$name]);
                $progress['incomplete_deployments'] = [(string) $name];

                foreach (["$root/releases/$name", "$root/runtime/$name"] as $path) {
                    if (deploymentRealDirectory($path)) {
                        pruneRemoveTree($path);
                    }
                }

                foreach (array_keys($files[$name]) as $path) {
                    if (!unlink($path)) {
                        throw new RuntimeException("Cannot remove deployment metadata or log: $path");
                    }
                }

                if (!unlink($marker)) {
                    throw new RuntimeException("Cannot remove completed cleanup marker: $marker");
                }
                $progress['deployments'][] = ['reference' => (string) $name];
                $progress['incomplete_deployments'] = [];
            }

            foreach ($result['artifacts'] as $filename) {
                if (!unlink(path_join($state, 'artifacts', $filename))) {
                    throw new RuntimeException("Cannot remove cached artifact: $filename");
                }
                $progress['artifacts'][] = $filename;
            }
        }

        return $result;
    } finally {
        fclose($lock);
    }
}
