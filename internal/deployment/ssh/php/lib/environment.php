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
    // This editor replaces physical lines. Reject quoted continuations before
    // matching keys, including apparent assignments inside a multiline value.
    foreach (explode("\n", $content) as $physicalLine) {
        $quote = null;
        $length = strlen($physicalLine);

        for ($i = 0; $i < $length; ++$i) {
            $char = $physicalLine[$i];

            if ($char === '#' && $quote === null && ($i === 0 || ctype_space($physicalLine[$i - 1]))) {
                break;
            }

            if ($char === "\\" && $quote !== "'") {
                ++$i;
                continue;
            }

            if ($quote === null && ($char === '"' || $char === "'")) {
                $quote = $char;
            } elseif ($char === $quote) {
                $quote = null;
            }
        }

        if ($quote !== null) {
            throw new RuntimeException('Cannot update deployment configuration with multiline or unterminated quoted values');
        }
    }

    ksort($values);

    foreach ($values as $key => $value) {
        if (!preg_match('/^[A-Z_][A-Z0-9_]*$/', (string) $key) || !is_string($value)) {
            throw new RuntimeException("Invalid environment value");
        }

        $line = $key . '=' . initEncode($value);
        $pattern = '/^[ \t]*(?:export[ \t]+)?' . preg_quote($key, '/') . '[ \t]*=.*$/m';

        if (preg_match($pattern, $content)) {
            $content = (string) preg_replace_callback($pattern, static fn (): string => $line, $content);
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
