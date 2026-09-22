<?php declare(strict_types=1);

/**
 * Dump a Shopware container XML for PHPStan and print its absolute path.
 *
 * Shopware's own phpstan-bootstrap.php looks for one hardcoded file
 * (StaticAnalyzeKernel + phpstan_dev). This script boots whatever kernel the
 * installation provides and resolves the XML from that kernel's build directory
 * and container class, so plugin checkouts and full projects share one path.
 *
 * Exit 0 prints the path. Exit 3 means this directory is not a Shopware install.
 */

use Composer\Autoload\ClassLoader;
use Shopware\Core\DevOps\StaticAnalyze\StaticAnalyzeKernel;
use Shopware\Core\Framework\Adapter\Kernel\KernelFactory;
use Shopware\Core\Framework\Plugin\KernelPluginLoader\StaticKernelPluginLoader;
use Shopware\Core\Kernel;
use Symfony\Component\Config\ConfigCache;
use Symfony\Component\Dotenv\Dotenv;

const DUMP_EXIT_SKIP = 3;

ini_set('display_errors', 'stderr');

try {
    $analyzedRoot = realpath($argv[1] ?? '');
    if ($analyzedRoot === false || !is_dir($analyzedRoot)) {
        fwrite(STDERR, "analyzed directory does not exist\n");
        exit(DUMP_EXIT_SKIP);
    }

    $shopware = findShopwareInstall($analyzedRoot);
    if ($shopware === null) {
        exit(DUMP_EXIT_SKIP);
    }

    echo dumpContainerXml($shopware, $analyzedRoot), "\n";
    exit(0);
} catch (Throwable $e) {
    fwrite(STDERR, $e->getMessage() . "\n");
    exit(1);
}

/**
 * @return array{root: string, autoload: string, synthetic: bool}|null
 */
function findShopwareInstall(string $start): ?array
{
    $coreInstall = null;
    $projectInstall = null;
    $dir = $start;

    while (true) {
        if (isShopwareRoot($dir)) {
            $install = ['root' => $dir, 'autoload' => $dir . '/vendor/autoload.php', 'synthetic' => true];
            $coreInstall ??= $install;
            if (is_file($dir . '/config/bundles.php')) {
                $install['synthetic'] = false;
                $projectInstall = $install;
                break;
            }
        }

        $parent = dirname($dir);
        if ($parent === $dir) {
            break;
        }
        $dir = $parent;
    }

    return $projectInstall ?? $coreInstall;
}

function isShopwareRoot(string $dir): bool
{
    if (!is_file($dir . '/vendor/autoload.php')) {
        return false;
    }

    // Composer installs shopware/core into vendor. The platform repository
    // keeps the same classes under src/Core and replaces the package.
    return is_file($dir . '/vendor/shopware/core/Kernel.php')
        || is_file($dir . '/src/Core/Kernel.php');
}

/**
 * @param array{root: string, autoload: string, synthetic: bool} $shopware
 */
function dumpContainerXml(array $shopware, string $analyzedRoot): string
{
    /** @var ClassLoader $classLoader */
    $classLoader = require $shopware['autoload'];

    if (!class_exists(KernelFactory::class) || !class_exists(StaticKernelPluginLoader::class)) {
        throw new RuntimeException('This Shopware version cannot boot a container for PHPStan');
    }

    $projectDir = $shopware['synthetic']
        ? prepareSyntheticProject($shopware['root'])
        : $shopware['root'];
    $projectDir = realpath($projectDir) ?: $projectDir;

    $_ENV['PROJECT_ROOT'] = $_SERVER['PROJECT_ROOT'] = $projectDir;
    bootProjectEnv($projectDir);

    $plugins = discoverPlugins($shopware['root'], $analyzedRoot, $projectDir);
    $pluginLoader = new StaticKernelPluginLoader($classLoader, null, $plugins);

    $kernelClass = class_exists(StaticAnalyzeKernel::class) ? StaticAnalyzeKernel::class : Kernel::class;
    KernelFactory::$kernelClass = $kernelClass;

    $kernel = KernelFactory::create('phpstan_dev', true, $classLoader, $pluginLoader);
    $xml = containerXmlPath($kernel);
    $php = dirname($xml) . '/' . pathinfo($xml, PATHINFO_FILENAME) . '.php';

    if (class_exists(ConfigCache::class) && is_file($xml) && (new ConfigCache($php, true))->isFresh()) {
        return $xml;
    }

    ob_start();
    try {
        $kernel->boot();
    } finally {
        ob_end_clean();
    }

    if (!is_file($xml)) {
        throw new RuntimeException('Shopware did not dump a container XML at ' . $xml);
    }

    return $xml;
}

function prepareSyntheticProject(string $shopwareRoot): string
{
    $projectDir = $shopwareRoot . '/var/cache/shopware-cli-phpstan';
    if (!is_dir($projectDir . '/config/packages') && !mkdir($projectDir . '/config/packages', 0777, true) && !is_dir($projectDir . '/config/packages')) {
        throw new RuntimeException('Unable to create ' . $projectDir);
    }

    writeIfChanged($projectDir . '/.env', <<<'ENV'
APP_ENV=phpstan_dev
APP_SECRET=shopwarecliphpstansecret000000000000
APP_URL=http://127.0.0.1
DATABASE_URL=mysql://_placeholder.test
MAILER_DSN=null://null
LOCK_DSN=flock

ENV);

    writeIfChanged($projectDir . '/config/bundles.php', syntheticBundlesPhp());

    return $projectDir;
}

function syntheticBundlesPhp(): string
{
    $candidates = [
        Symfony\Bundle\FrameworkBundle\FrameworkBundle::class => ['all' => true],
        Symfony\Bundle\MonologBundle\MonologBundle::class => ['all' => true],
        Symfony\Bundle\TwigBundle\TwigBundle::class => ['all' => true],
        Symfony\UX\TwigComponent\TwigComponentBundle::class => ['all' => true],
        Shopware\Core\Profiling\Profiling::class => ['all' => true],
        Shopware\Core\Framework\Framework::class => ['all' => true],
        Shopware\Core\System\System::class => ['all' => true],
        Shopware\Core\Content\Content::class => ['all' => true],
        Shopware\Core\Checkout\Checkout::class => ['all' => true],
        Shopware\Core\DevOps\DevOps::class => ['all' => true],
        Shopware\Core\Maintenance\Maintenance::class => ['all' => true],
        Shopware\Core\Service\Service::class => ['all' => true],
        Shopware\Administration\Administration::class => ['all' => true],
        Shopware\Storefront\Storefront::class => ['all' => true],
        Shopware\Elasticsearch\Elasticsearch::class => ['all' => true],
        Symfony\AI\McpBundle\McpBundle::class => ['all' => true],
    ];

    $lines = ["<?php declare(strict_types=1);\n\nreturn [\n"];
    foreach ($candidates as $class => $envs) {
        if (!class_exists($class)) {
            continue;
        }
        $lines[] = '    ' . var_export($class, true) . ' => ' . var_export($envs, true) . ",\n";
    }
    $lines[] = "];\n";

    return implode('', $lines);
}

function bootProjectEnv(string $projectDir): void
{
    if (!class_exists(Dotenv::class)) {
        return;
    }

    if (!is_file($projectDir . '/.env') && !is_file($projectDir . '/.env.dist') && !is_file($projectDir . '/.env.local.php')) {
        return;
    }

    (new Dotenv())->usePutenv()->bootEnv($projectDir . '/.env');
}

/**
 * @return list<array{name: string, baseClass: string, active: bool, path: string, version: string, autoload: array<string, mixed>, managedByComposer: bool, composerName: string}>
 */
function discoverPlugins(string $shopwareRoot, string $analyzedRoot, string $projectDir): array
{
    $plugins = [];
    $add = static function (string $directory) use (&$plugins, $projectDir): void {
        $info = pluginInfoFromComposer($directory, $projectDir);
        if ($info === null) {
            return;
        }
        $plugins[$info['baseClass']] = $info;
    };

    $add($analyzedRoot);
    $add($shopwareRoot);

    if (class_exists(Composer\InstalledVersions::class)) {
        foreach (Composer\InstalledVersions::getAllRawData() as $data) {
            if (!is_array($data)) {
                continue;
            }

            $packages = [];
            if (isset($data['root']) && is_array($data['root'])) {
                $packages[] = $data['root'];
            }
            foreach ($data['versions'] ?? [] as $package) {
                if (is_array($package)) {
                    $packages[] = $package;
                }
            }

            foreach ($packages as $package) {
                if (($package['type'] ?? '') !== 'shopware-platform-plugin') {
                    continue;
                }
                $path = $package['install_path'] ?? '';
                if (is_string($path) && $path !== '') {
                    $add($path);
                }
            }
        }
    }

    foreach (['custom/plugins', 'custom/static-plugins'] as $relative) {
        $directory = $shopwareRoot . '/' . $relative;
        if (!is_dir($directory)) {
            continue;
        }
        foreach (scandir($directory) ?: [] as $entry) {
            if ($entry === '.' || $entry === '..') {
                continue;
            }
            $add($directory . '/' . $entry);
        }
    }

    return array_values($plugins);
}

/**
 * @return array{name: string, baseClass: string, active: bool, path: string, version: string, autoload: array<string, mixed>, managedByComposer: bool, composerName: string}|null
 */
function pluginInfoFromComposer(string $directory, string $projectDir): ?array
{
    $directory = realpath($directory) ?: '';
    if ($directory === '' || !is_file($directory . '/composer.json')) {
        return null;
    }

    try {
        $json = json_decode((string) file_get_contents($directory . '/composer.json'), true, 512, JSON_THROW_ON_ERROR);
    } catch (JsonException) {
        return null;
    }

    if (!is_array($json) || ($json['type'] ?? '') !== 'shopware-platform-plugin') {
        return null;
    }

    $class = $json['extra']['shopware-plugin-class'] ?? '';
    if (!is_string($class) || $class === '') {
        return null;
    }

    $alreadyLoaded = class_exists($class);
    $insideProject = str_starts_with($directory . '/', rtrim(str_replace('\\', '/', $projectDir), '/') . '/');
    if (!$alreadyLoaded && !$insideProject) {
        return null;
    }

    $parts = explode('\\', $class);

    return [
        'name' => $parts[array_key_last($parts)] ?? $class,
        'baseClass' => $class,
        'active' => true,
        'path' => $directory,
        'version' => is_string($json['version'] ?? null) ? $json['version'] : '1.0.0',
        'autoload' => is_array($json['autoload'] ?? null) ? $json['autoload'] : [],
        'managedByComposer' => $alreadyLoaded,
        'composerName' => is_string($json['name'] ?? null) ? $json['name'] : $class,
    ];
}

function containerXmlPath(object $kernel): string
{
    $buildDir = null;
    if (method_exists($kernel, 'getBuildDir')) {
        $buildDir = $kernel->getBuildDir();
    } elseif (method_exists($kernel, 'getCacheDir')) {
        $buildDir = $kernel->getCacheDir();
    }

    if (!is_string($buildDir) || $buildDir === '') {
        throw new RuntimeException('Shopware kernel did not expose a build directory');
    }

    $method = (new ReflectionObject($kernel))->getMethod('getContainerClass');
    $class = $method->invoke($kernel);
    if (!is_string($class) || $class === '') {
        throw new RuntimeException('Shopware kernel did not expose a container class');
    }

    return $buildDir . '/' . $class . '.xml';
}

function writeIfChanged(string $path, string $contents): void
{
    if (is_file($path) && file_get_contents($path) === $contents) {
        return;
    }

    if (file_put_contents($path, $contents) === false) {
        throw new RuntimeException('Unable to write ' . $path);
    }
}
