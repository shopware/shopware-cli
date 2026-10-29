<?php
// Provider-specific fallback when no CacheTool adapter is enabled.
function deploymentRestartPHPProcesses(): void
{
    $deadline = hrtime(true) + 30000000000;
    $remaining = static function () use ($deadline): int {
        $seconds = (int) ceil(($deadline - hrtime(true)) / 1000000000);
        if ($seconds < 1) {
            throw new RuntimeException('PHP process restart timed out');
        }
        return $seconds;
    };
    $hostname = trim(deploymentRunBoundedCommand(['hostname', '-f'], '.', min(5, $remaining()), true));
    if ($hostname === '' || !preg_match('/\A[a-zA-Z0-9._-]+\z/', $hostname)) {
        throw new RuntimeException('Cannot determine the remote fully qualified hostname');
    }
    if (stripos($hostname, 'de-nserver.de') === false) {
        deploymentReportReset("Skipping PHP process restart: $hostname does not match de-nserver.de");
        return;
    }

    $self = getmypid();
    if ($self === false || $self <= 1) {
        throw new RuntimeException('Cannot identify deployment process for PHP restart');
    }
    $uidText = trim(deploymentRunBoundedCommand(['id', '-u'], '.', min(5, $remaining()), true));
    $uid = filter_var($uidText, FILTER_VALIDATE_INT, ['options' => ['min_range' => 0]]);
    if ($uid === false) {
        throw new RuntimeException('Cannot identify SSH user for PHP restart');
    }
    if ($uid === 0) {
        throw new RuntimeException('Refusing automatic PHP process restart for the root account; configure CacheTool instead');
    }
    $output = deploymentRunBoundedCommand(['pgrep', '-u', (string) $uid, 'php'], '.', min(5, $remaining()), true, [0, 1]);
    $pids = [];
    foreach (preg_split('/\s+/', trim($output), -1, PREG_SPLIT_NO_EMPTY) as $value) {
        $pid = filter_var($value, FILTER_VALIDATE_INT, ['options' => ['min_range' => 0]]);
        if (!preg_match('/\A[0-9]+\z/', $value) || $pid === false) {
            throw new RuntimeException('Invalid PHP process identifier');
        }
        // Never signal PID 0 (a process group), PID 1, or this deployment process.
        if ($pid > 1 && $pid !== $self) {
            $pids[$pid] = true;
        }
    }
    deploymentReportReset("Restarting PHP processes on $hostname for SSH user $uid");
    $count = 0;
    $error = null;
    foreach (array_keys($pids) as $pid) {
        try {
            deploymentRunBoundedCommand(['kill', '-TERM', (string) $pid], '.', min(5, $remaining()));
            $count++;
        } catch (Throwable $failure) {
            $error ??= $failure;
        }
    }
    deploymentReportReset("PHP processes signaled with SIGTERM: $count");
    if ($error !== null) {
        throw new RuntimeException("Could not restart all PHP processes: {$error->getMessage()}");
    }
}
