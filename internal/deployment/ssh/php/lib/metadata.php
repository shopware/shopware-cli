<?php

function deploymentWriteMetadata(string $file, array $metadata): void
{
    deploymentWriteAtomic($file, json_encode($metadata, JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), 'metadata');
}

function deploymentReadMetadata(string $file): array
{
    deploymentRegularFile($file, 'metadata', true);

    $metadata = json_decode((string) file_get_contents($file), true, 512, JSON_THROW_ON_ERROR);

    if (!is_array($metadata)) {
        throw new RuntimeException("Invalid metadata: $file");
    }

    return $metadata;
}

function deploymentValidName(mixed $name): bool
{
    return is_string($name) && strlen($name) <= 128
        && preg_match('/\A[a-zA-Z0-9][a-zA-Z0-9._-]*\z/', $name) === 1;
}

function deploymentValidChecksum(mixed $checksum): bool
{
    return is_string($checksum) && preg_match('/\A[a-f0-9]{64}\z/', $checksum) === 1;
}

/** Format a time, by default the current one, the way metadata records it. */
function deploymentTimestamp(?DateTimeImmutable $time = null): string
{
    $time ??= new DateTimeImmutable('now');

    return $time->setTimezone(new DateTimeZone('UTC'))->format('Y-m-d\TH:i:s.u\Z');
}

/** Parse an absolute ISO 8601 timestamp; relative date expressions are rejected. */
function deploymentParseTimestamp(mixed $value): ?DateTimeImmutable
{
    if (!is_string($value) || !preg_match('/\A\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,6})?(?:Z|[+-]\d\d:\d\d)\z/', $value)) {
        return null;
    }

    try {
        $time = new DateTimeImmutable($value);
        $errors = DateTimeImmutable::getLastErrors();

        return $errors === false || ($errors['warning_count'] === 0 && $errors['error_count'] === 0) ? $time : null;
    } catch (Throwable) {
        return null;
    }
}

function deploymentReleaseCreatedAt(string $root, string $name, array $state): ?string
{
    $created = deploymentParseTimestamp($state['created_at'] ?? null);

    if ($created === null) {
        // Older release state has no creation date; recover it from retained history.
        foreach (new FilesystemIterator(deploymentStatePath($root, 'rollouts')) as $file) {
            if ($file->isLink() || !$file->isFile() || $file->getExtension() !== 'json') {
                continue;
            }

            try {
                $record = deploymentReadMetadata($file->getPathname());
            } catch (Throwable) {
                continue;
            }

            if (($record['release'] ?? null) !== $name || ($record['status'] ?? null) !== 'successful'
                || ($record['sha256'] ?? null) !== ($state['sha256'] ?? null)) {
                continue;
            }

            $time = deploymentParseTimestamp($record['created_at'] ?? $record['deployed_at'] ?? null);

            if ($time !== null && ($created === null || $time < $created)) {
                $created = $time;
            }
        }
    }

    return $created === null ? null : deploymentTimestamp($created);
}

// Only fully recorded activations are eligible for reuse.
function deploymentRetained(string $root, string $name): array
{
    $release = path_join($root, 'releases', $name);

    if (!deploymentValidName($name) || is_link($release) || !is_dir($release)) {
        throw new RuntimeException("Retained release must be a real directory: $release");
    }

    $state = deploymentReadMetadata(deploymentStatePath($root, 'releases', "$name.json"));
    $reference = $state['reference'] ?? null;

    if (($state['release'] ?? null) !== $name || ($state['ready'] ?? null) !== true
        || array_key_exists('activation_pending', $state) || array_key_exists('pruning', $state)
        || !deploymentValidName($reference)
        || !deploymentValidChecksum($state['sha256'] ?? null)
        || deploymentParseTimestamp($state['deployed_at'] ?? null) === null
        || !str_ends_with($state['deployed_at'], 'Z')) {
        throw new RuntimeException("Cannot establish successful retained release $name");
    }

    $event = deploymentReadMetadata(deploymentStatePath($root, 'rollouts', "$reference.json"));

    if (($event['status'] ?? null) !== 'successful' || ($event['release'] ?? null) !== $name
        || ($event['reference'] ?? null) !== $reference
        || ($event['sha256'] ?? null) !== $state['sha256']
        || ($event['deployed_at'] ?? null) !== $state['deployed_at']
        || !is_string($event['deployment'] ?? null) || $event['deployment'] === ''
        || !is_string($event['archive'] ?? null) || $event['archive'] === ''
        || array_key_exists('activation_pending', $event) || array_key_exists('pruning', $event)) {
        throw new RuntimeException("Inconsistent successful retained release $name");
    }

    $event['release_created_at'] = deploymentReleaseCreatedAt($root, $name, $state);

    return $event;
}

/** List retained releases that can be activated again, newest first. */
function deploymentCandidates(string $root, ?string $active): array
{
    $candidates = [];
    $releases = path_join($root, 'releases');

    foreach (is_dir($releases) ? new FilesystemIterator($releases) : [] as $entry) {
        $name = $entry->getFilename();

        try {
            $event = deploymentRetained($root, $name);
        } catch (Throwable) {
            continue;
        }

        $candidates[] = [
            'deployment' => ['reference' => $name, 'name' => $name],
            'created_at' => $event['release_created_at'],
            'deployed_at' => $event['deployed_at'],
            'active' => $active === $entry->getRealPath(),
            'sha256' => $event['sha256'],
        ];
    }

    usort($candidates, static fn (array $a, array $b): int =>
        (deploymentParseTimestamp($b['created_at']) <=> deploymentParseTimestamp($a['created_at']))
        ?: strcmp($a['deployment']['reference'], $b['deployment']['reference']));

    return $candidates;
}
