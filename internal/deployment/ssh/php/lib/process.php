<?php

function deploymentRun(array $command, string $directory): void
{
    $process = proc_open($command, [0 => ['file', '/dev/null', 'r'], 1 => STDERR, 2 => STDERR], $pipes, $directory);

    if (!is_resource($process) || proc_close($process) !== 0) {
        throw new RuntimeException("Deployment command failed: " . implode(' ', $command));
    }
}

/** Bound the whole hook process group, including children of the shell. */
function deploymentRunHook(string $script, string $directory, int $timeout): void
{
    $process = proc_open(
        ['timeout', '--kill-after=5s', $timeout . 's', 'sh', '-eu', '-c', $script],
        [0 => ['file', '/dev/null', 'r'], 1 => STDERR, 2 => STDERR],
        $pipes,
        $directory
    );

    if (!is_resource($process)) {
        throw new RuntimeException('cannot start deployment hook');
    }
    $status = proc_close($process);

    if ($status !== 0) {
        throw new RuntimeException("cannot run deployment hook (exit $status; timeout {$timeout}s)");
    }
}

/** Run every post-hook before reporting failures, including during cleanup. */
function deploymentRunPostHooks(array $scripts, string $directory, int $timeout): void
{
    $errors = [];

    foreach ($scripts as $script) {
        try {
            deploymentRunHook($script, $directory, $timeout);
        } catch (Throwable $error) {
            $errors[] = $error->getMessage();
        }
    }

    if ($errors !== []) {
        throw new RuntimeException("cannot run post hooks: " . implode("; ", $errors));
    }
}

/** The configured PHP binary and arguments, followed by a script and its arguments. */
function deploymentPHPCommand(array $input, string ...$arguments): array
{
    return [$input['php'] ?? PHP_BINARY, ...($input['php_args'] ?? []), ...$arguments];
}

function deploymentRunHelper(array $command, string $directory, string $root, string $name): void
{
    // This is deliberately only called for a new preparation, never for reuse.
    foreach ([$root, deploymentStatePath($root)] as $parent) {
        if (!deploymentRealDirectory($parent)) {
            throw new RuntimeException("Expected a real directory: $parent");
        }
    }

    $logs = deploymentStatePath($root, 'logs');
    deploymentDirectory($logs, 0700);

    if (!chmod($logs, 0700)) {
        throw new RuntimeException("Cannot protect deployment logs: $logs");
    }

    $file = path_join($logs, "$name.log");

    deploymentRegularFile($file, 'deployment log');
    // Exclusive creation preserves logs even after an unsuccessful preparation.
    $log = deploymentOpenPrivate($file);

    if ($log === false) {
        throw new RuntimeException("Cannot create deployment log (it may already exist): $file");
    }

    $process = null;
    $pipes = [];

    try {
        if (!chmod($file, 0600)) {
            throw new RuntimeException("Cannot protect deployment log: $file");
        }

        $process = proc_open($command, [
            0 => ['file', '/dev/null', 'r'],
            1 => ['pipe', 'w'],
            2 => ['redirect', 1],
        ], $pipes, $directory);

        if (!is_resource($process)) {
            throw new RuntimeException('Cannot start deployment helper');
        }

        $streamError = null;
        $targets = ['log' => $log, 'stderr' => STDERR];

        while (!feof($pipes[1])) {
            $bytes = @fread($pipes[1], 8192);

            if ($bytes === false || ($bytes === '' && !feof($pipes[1]))) {
                throw new RuntimeException('Cannot read deployment helper output');
            }

            foreach ($targets as $target => $stream) {
                try {
                    deploymentWriteAll($stream, $bytes, "deployment helper output to $target");
                } catch (Throwable $error) {
                    // Keep draining the helper and preserve its log even if the
                    // SSH client's stderr is no longer writable.
                    $streamError ??= $error;
                    unset($targets[$target]);
                }
            }
        }

        fclose($pipes[1]);
        unset($pipes[1]);
        $status = proc_close($process);
        $process = null;

        if ($status !== 0) {
            throw new RuntimeException("Deployment helper failed (exit $status); log: $file");
        }

        if ($streamError !== null) {
            throw $streamError;
        }
    } finally {
        if (is_resource($process)) {
            proc_terminate($process);
        }

        foreach ($pipes as $pipe) {
            if (is_resource($pipe)) {
                fclose($pipe);
            }
        }

        if (is_resource($process)) {
            proc_close($process);
        }

        fclose($log);
    }
}

/** Write to stderr without blocking past the deadline when the SSH client stops reading. */
function deploymentWriteBounded(string $bytes, int $deadline): bool
{
    $blocking = stream_get_meta_data(STDERR)['blocked'] ?? true;

    if (!@stream_set_blocking(STDERR, false)) {
        return false;
    }

    try {
        $offset = 0;

        while ($offset < strlen($bytes)) {
            if (hrtime(true) >= $deadline) {
                return false;
            }

            $written = @fwrite(STDERR, substr($bytes, $offset));

            if ($written === false) {
                return false;
            }

            if ($written === 0) {
                usleep(10000);

                continue;
            }

            $offset += $written;
        }

        return true;
    } finally {
        @stream_set_blocking(STDERR, $blocking);
    }
}

// Bound process execution and output forwarding independently of the SSH reader.
function deploymentRunBoundedCommand(array $command, string $directory, int $timeout, bool $capture = false, array $successCodes = [0]): string
{
    $process = null;
    $pipes = [];
    $output = '';

    try {
        $process = proc_open($command, [
            0 => ['file', '/dev/null', 'r'],
            1 => ['pipe', 'w'],
            2 => ['redirect', 1],
        ], $pipes, $directory);

        if (!is_resource($process)) {
            throw new RuntimeException('Cannot start deployment command');
        }

        stream_set_blocking($pipes[1], false);
        $deadline = hrtime(true) + $timeout * 1000000000;
        $exit = null;

        while (true) {
            $bytes = fread($pipes[1], 8192);

            if ($bytes === false) {
                throw new RuntimeException('Cannot read deployment command output');
            }

            if ($capture) {
                if (strlen($output) + strlen($bytes) > 1048576) {
                    throw new RuntimeException('Deployment command output exceeds 1 MiB');
                }

                $output .= $bytes;
            } elseif ($bytes !== '' && !deploymentWriteBounded($bytes, $deadline)) {
                throw new RuntimeException('Deployment command output blocked or unavailable');
            }

            $status = proc_get_status($process);

            if (!$status['running'] && $exit === null) {
                $exit = $status['exitcode'];
            }

            if (!$status['running'] && feof($pipes[1])) {
                break;
            }

            if (hrtime(true) >= $deadline) {
                throw new RuntimeException("Deployment command timed out after $timeout seconds");
            }

            if ($bytes === '') {
                usleep(10000);
            }
        }

        fclose($pipes[1]);
        $pipes = [];
        $closed = proc_close($process);
        $process = null;
        $exit = $exit === null || $exit < 0 ? $closed : $exit;

        if (!in_array($exit, $successCodes, true)) {
            throw new RuntimeException(basename($command[0]) . " failed (exit $exit)");
        }

        return $output;
    } finally {
        if (is_resource($process)) {
            // Do not wait indefinitely for a helper that ignores SIGTERM.
            proc_terminate($process);
            $grace = hrtime(true) + 200000000;

            do {
                if (isset($pipes[1])) {
                    @fread($pipes[1], 8192);
                }

                $running = proc_get_status($process)['running'];

                if (!$running) {
                    break;
                }

                usleep(10000);
            } while (hrtime(true) < $grace);

            if ($running) {
                proc_terminate($process, 9);
            }
        }

        foreach ($pipes as $pipe) {
            fclose($pipe);
        }

        if (is_resource($process)) {
            proc_close($process);
        }
    }
}

/** Report progress or a failure to the client on stderr. */
function deploymentLog(string $message): void
{
    deploymentWriteBounded($message . "\n", hrtime(true) + 250000000);
}
