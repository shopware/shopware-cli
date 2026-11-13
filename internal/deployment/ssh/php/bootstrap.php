<?php

if (\PHP_SAPI !== 'cli') {
    exit(1);
}

error_reporting(E_ALL);
ini_set('display_errors', 'stderr');
ini_set('html_errors', '0');
ini_set('log_errors', '0');

require_once __DIR__ . '/lib/layout.php';
require_once __DIR__ . '/lib/filesystem.php';
require_once __DIR__ . '/lib/process.php';
require_once __DIR__ . '/lib/shared.php';
require_once __DIR__ . '/lib/metadata.php';
require_once __DIR__ . '/lib/protocol.php';
require_once __DIR__ . '/lib/artifact.php';
require_once __DIR__ . '/lib/install.php';
require_once __DIR__ . '/lib/environment.php';
require_once __DIR__ . '/lib/permissions.php';
require_once __DIR__ . '/lib/cachetool.php';
require_once __DIR__ . '/lib/php_restart.php';
require_once __DIR__ . '/lib/retention.php';
