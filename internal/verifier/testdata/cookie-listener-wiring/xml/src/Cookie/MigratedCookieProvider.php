<?php declare(strict_types=1);

namespace Test\TestPlugin\Cookie;

use Psr\Log\LoggerInterface;
use Shopware\Core\Content\Cookie\Event\CookieGroupCollectEvent;

class MigratedCookieProvider
{
    public function __construct(private readonly LoggerInterface $logger)
    {
    }

    public function __invoke(CookieGroupCollectEvent $event): void
    {
    }
}
