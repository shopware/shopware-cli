<?php

function deploymentInstallEnvironment(string $file): array
{
    if (!deploymentRegularFile($file, 'installation environment')) {
        return [];
    }

    // The initializer writes double-quoted dotenv values. The normal scanner
    // decodes their escaped quotes, dollars, and backslashes while preserving
    // quoted values as strings.
    $values = parse_ini_file($file, false, INI_SCANNER_NORMAL);

    if ($values === false) {
        throw new RuntimeException("Cannot parse installation environment: $file");
    }

    foreach ($values as $key => $value) {
        if ((!str_starts_with($key, 'INSTALL_') && $key !== 'SALES_CHANNEL_URL') || !is_string($value)) {
            throw new RuntimeException("Unsupported installation environment variable: $key");
        }

        putenv("$key=$value");
    }

    return array_keys($values);
}
