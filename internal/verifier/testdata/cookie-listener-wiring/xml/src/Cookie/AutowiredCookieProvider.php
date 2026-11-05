<?php declare(strict_types=1);

namespace Test\TestPlugin\Cookie;

use Shopware\Core\Content\Cookie\Event\CookieGroupCollectEvent;

final class AutowiredCookieProvider
{
    public function __invoke(CookieGroupCollectEvent $event): void
    {
    }
}
