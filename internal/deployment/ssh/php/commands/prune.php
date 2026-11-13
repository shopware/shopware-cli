<?php

require_once __DIR__ . '/../bootstrap.php';

$progress = ['deployments' => [], 'artifacts' => [], 'artifact_skip_reasons' => [], 'incomplete_deployments' => []];

try {
    // Warnings must never leak into the machine-readable stdout response.
    set_error_handler(static function (int $severity, string $message): never {
        throw new RuntimeException($message);
    });
    $input = deploymentInput($argv[1] ?? null);
    echo json_encode(prunePlan($input, $progress), JSON_THROW_ON_ERROR) . "\n";
} catch (Throwable $error) {
    echo json_encode($progress, JSON_THROW_ON_ERROR) . "\n";
    fwrite(STDERR, $error->getMessage() . "\n");

    exit(1);
}
