<?php

/** Read environment key names without exposing their values. */
function initKeys(string $file): array
{
    if (!deploymentRegularFile($file, 'deployment configuration')) {
        return [];
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

/** Quote a value for dotenv while preserving literal special characters. */
function initEncode(string $value): string
{
    return '"' . str_replace(
        ["\\", '"', '$', "\r", "\n"],
        ["\\\\", '\\"', '\$', '\r', '\n'],
        $value
    ) . '"';
}

/** Replace existing keys literally and append missing keys, preserving unrelated content. */
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
            $content = (string) preg_replace_callback($pattern, static fn (): string => $line, $content, 1);
        } else {
            if ($content !== '' && !str_ends_with($content, "\n")) {
                $content .= "\n";
            }

            $content .= $line . "\n";
        }
    }

    return $content;
}

/** Merge values into a configuration file and publish it with owner-only permissions. */
function initWrite(string $file, array $values): void
{
    $content = deploymentRegularFile($file, 'deployment configuration') ? file_get_contents($file) : '';

    if ($content === false) {
        throw new RuntimeException("Cannot read deployment configuration: $file");
    }

    deploymentWriteAtomic($file, initMerge($content, $values), 'deployment configuration', 0600);
}
