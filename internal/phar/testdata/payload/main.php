<?php
require __DIR__ . '/lib/message.php';

file_put_contents($argv[1], json_encode([
    'archive' => Phar::running(false),
    'args' => array_slice($argv, 2),
    'message' => message(),
    'cwd' => getcwd(),
]));

if ($argv[2] === 'exit') {
    exit(7);
}
if ($argv[2] === 'fatal') {
    undefined_function();
}
echo "done\n";
