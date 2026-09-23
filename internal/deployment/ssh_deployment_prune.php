<?php
// Passed to PHP -r. Never interpret metadata's archive field as a target path.
function pruneName(mixed $name): bool
{
    return is_string($name) && strlen($name) <= 128
        && preg_match('/\A[a-zA-Z0-9][a-zA-Z0-9._-]*\z/', $name) === 1;
}

function pruneDirectory(string $path): bool
{
    if (is_link($path) || (file_exists($path) && !is_dir($path))) {
        throw new RuntimeException("Expected a real directory: $path");
    }
    return is_dir($path);
}

function pruneFile(string $path): void
{
    if (is_link($path) || !is_file($path)) {
        throw new RuntimeException("Expected a regular file: $path");
    }
}

function pruneTime(mixed $value): DateTimeImmutable
{
    // Require an absolute timestamp, not PHP's relative-date expressions.
    if (!is_string($value)
        || preg_match('/\A\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?(?:Z|[+-]\d{2}:\d{2})\z/', $value) !== 1) {
        throw new RuntimeException('Invalid deployment timestamp');
    }
    $time = new DateTimeImmutable($value);
    $errors = DateTimeImmutable::getLastErrors();
    if ($errors !== false && ($errors['warning_count'] || $errors['error_count'])) {
        throw new RuntimeException('Invalid deployment timestamp');
    }
    return $time;
}

function pruneRecords(string $directory, string $identity, bool &$archivesKnown): array
{
    $records = [];
    if (!is_dir($directory)) {
        return $records;
    }
    foreach (new FilesystemIterator($directory) as $entry) {
        $filename = $entry->getFilename();
        if (!str_ends_with($filename, '.json')) {
            // An interrupted metadata write may contain an otherwise unknown SHA.
            $archivesKnown = false;
            continue;
        }
        $name = substr($filename, 0, -5);
        if (!pruneName($name)) {
            throw new RuntimeException("Invalid metadata filename: $filename");
        }
        pruneFile($entry->getPathname());
        $object = json_decode((string) file_get_contents($entry->getPathname()), false, 512, JSON_THROW_ON_ERROR);
        if (!$object instanceof stdClass) {
            throw new RuntimeException("Invalid metadata: $filename");
        }
        $record = (array) $object;
        foreach (['release', 'reference', 'activation_pending'] as $field) {
            if (array_key_exists($field, $record) && !pruneName($record[$field])) {
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
            if (!is_string($record['sha256']) || preg_match('/\A[a-f0-9]{64}\z/', $record['sha256']) !== 1) {
                throw new RuntimeException("Invalid checksum in $filename");
            }
        } else {
            $archivesKnown = false;
        }
        foreach (['created_at', 'deployed_at'] as $field) {
            if (array_key_exists($field, $record)) {
                pruneTime($record[$field]);
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
    // Release-local links include shared runtime data; unlink, never traverse.
    if (is_link($path) || is_file($path)) {
        if (!unlink($path)) {
            throw new RuntimeException("Cannot remove release entry: $path");
        }
        return;
    }
    if (!is_dir($path)) {
        throw new RuntimeException("Unexpected release entry: $path");
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
    $directory = dirname($file);
    if (!pruneDirectory($directory) && !mkdir($directory, 0700)) {
        throw new RuntimeException("Cannot create release metadata directory: $directory");
    }
    $temporary = $file . '.tmp-' . bin2hex(random_bytes(8));
    $stream = fopen($temporary, 'x');
    try {
        $data = json_encode(['release' => $name, 'sha256' => $checksum, 'ready' => false, 'pruning' => true], JSON_THROW_ON_ERROR);
        if ($stream === false || fwrite($stream, $data) !== strlen($data) || !fflush($stream)) {
            throw new RuntimeException("Cannot record pending cleanup for release $name");
        }
        if (!rename($temporary, $file)) {
            throw new RuntimeException("Cannot publish pending cleanup for release $name");
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

function prunePlan(array $input): array
{
    $root = $input['root'] ?? null;
    if (!is_string($root) || $root === '' || $root[0] !== '/' || str_contains($root, "\0")
        || !is_int($input['keep'] ?? null) || $input['keep'] < 0 || !is_bool($input['dry_run'] ?? null)) {
        throw new RuntimeException('Invalid prune input');
    }
    $root = rtrim($root, '/') ?: '/';
    $result = ['deployments' => [], 'artifacts' => []];
    if (!pruneDirectory($root) || !pruneDirectory("$root/.shopware-cli")) {
        return $result;
    }
    $state = "$root/.shopware-cli";
    $lockPath = "$state/deployment.lock";
    pruneFile($lockPath);
    $lock = fopen($lockPath, 'r+');
    if ($lock === false) {
        throw new RuntimeException('Cannot open deployment lock');
    }
    try {
        if (!flock($lock, LOCK_EX | LOCK_NB)) {
            throw new RuntimeException("Another deployment holds the lock for $root");
        }
        foreach (["$root/releases", "$state/releases", "$state/rollouts", "$state/logs", "$state/artifacts"] as $directory) {
            pruneDirectory($directory);
        }
        $active = null;
        $current = "$root/current";
        if (is_link($current)) {
            $target = realpath($current);
            if ($target === false || !is_dir($target) || dirname($target) !== realpath("$root/releases")
                || is_link("$root/releases/" . basename($target))) {
                throw new RuntimeException('current must point to a real direct child of releases');
            }
            $active = basename($target);
        } elseif (file_exists($current)) {
            throw new RuntimeException('current must be a symlink');
        }

        $archivesKnown = true;
        $records = pruneRecords("$state/releases", 'release', $archivesKnown);
        $rollouts = pruneRecords("$state/rollouts", 'reference', $archivesKnown);
        $history = [];
        $historyHashes = [];
        foreach ($rollouts as $reference => $record) {
            // Missing identity is incomplete, not proof of a managed legacy release.
            $name = $record['release'] ?? $record['reference'] ?? null;
            if ($name === null) {
                $archivesKnown = false;
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
        if (is_dir("$root/releases")) {
            foreach (new FilesystemIterator("$root/releases") as $entry) {
                $name = $entry->getFilename();
                if (!pruneName($name) || $entry->isLink() || !$entry->isDir()) {
                    $archivesKnown = false;
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
                    $archivesKnown = false; // Unmanaged release: archive references unknown.
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
                    $time = pruneTime($event['deployed_at'] ?? $event['created_at']);
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
            if (pruneDirectory("$root/releases/$name")) {
                pruneValidateTree("$root/releases/$name");
            }
            $removeHashes[$name] = $records[$name]['sha256'] ?? $historyHashes[$name];
            $files[$name] = [];
            if (isset($records[$name])) {
                unset($records[$name]);
            }
            $logNames = [$name];
            foreach ($history[$name] ?? [] as $reference => $event) {
                $files[$name]["$state/rollouts/$reference.json"] = true;
                $logNames[] = $reference;
                unset($rollouts[$reference]);
            }
            foreach ($logNames as $logName) {
                // Do not remove a legacy rollout log that is another retained release's helper log.
                if ($logName != $name && file_exists("$root/releases/$logName") && !in_array($logName, $remove, true)) {
                    continue;
                }
                $path = "$state/logs/$logName.log";
                if (file_exists($path) || is_link($path)) {
                    pruneFile($path);
                    $files[$name][$path] = true;
                }
            }
            $result['deployments'][] = ['reference' => (string) $name];
        }
        $retainedHashes = [];
        foreach (array_merge(array_values($records), array_values($rollouts)) as $record) {
            if (isset($record['sha256'])) {
                $retainedHashes[$record['sha256']] = true;
            } else {
                $archivesKnown = false;
            }
        }
        if (is_dir("$state/artifacts")) {
            foreach (new FilesystemIterator("$state/artifacts") as $entry) {
                if (preg_match('/\A([a-f0-9]{64})\.tar\.gz\z/', $entry->getFilename(), $match) !== 1) {
                    continue;
                }
                pruneFile($entry->getPathname());
                if ($archivesKnown && !isset($retainedHashes[$match[1]])) {
                    $result['artifacts'][] = $entry->getFilename();
                }
            }
        }
        sort($result['artifacts'], SORT_STRING);
        // The entire plan, including logs and cached archives, is validated first.
        if (!$input['dry_run']) {
            foreach ($remove as $name) {
                // Persist intent first; ready=false also blocks rollout reuse after
                // an interruption. Keep the marker until every related file is gone.
                $marker = "$state/releases/$name.json";
                pruneMarkRelease($marker, (string) $name, $removeHashes[$name]);
                if (is_dir("$root/releases/$name")) {
                    pruneRemoveTree("$root/releases/$name");
                }
                foreach (array_keys($files[$name]) as $path) {
                    if (!unlink($path)) {
                        throw new RuntimeException("Cannot remove deployment metadata or log: $path");
                    }
                }
                if (!unlink($marker)) {
                    throw new RuntimeException("Cannot remove completed cleanup marker: $marker");
                }
            }
            foreach ($result['artifacts'] as $filename) {
                if (!unlink("$state/artifacts/$filename")) {
                    throw new RuntimeException("Cannot remove cached artifact: $filename");
                }
            }
        }
        return $result;
    } finally {
        fclose($lock);
    }
}

try {
    // Warnings must never leak into the machine-readable stdout response.
    set_error_handler(static function (int $severity, string $message): never {
        throw new RuntimeException($message);
    });
    $input = json_decode($argv[1] ?? '', true, 512, JSON_THROW_ON_ERROR);
    if (!is_array($input)) {
        throw new RuntimeException('Invalid prune input');
    }
    echo json_encode(prunePlan($input), JSON_THROW_ON_ERROR) . "\n";
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");
    exit(1);
}
