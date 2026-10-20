<?php
// Keep the active link beside deployment storage, never inside it.
function deploymentCurrentPath(array $input, string $root): string
{
    $name = $input['current'] ?? 'current';
    if ($root === '' || $root === '/' || $root[0] !== '/'
        || !is_string($name) || $name === '' || $name === '.' || $name === '..'
        || strpbrk($name, "/\\") !== false || preg_match('/[\x00-\x1f\x7f]/', $name)
        || in_array($name, ['releases', 'shared', 'runtime', '.shopware-cli'], true)
        || str_starts_with($name, '.current-')) {
        throw new RuntimeException('cannot resolve deployment layout: invalid active symlink path');
    }
    return "$root/$name";
}
