<?php declare(strict_types=1);

namespace Test\TestPlugin\Cookie;

use Shopware\Storefront\Framework\Cookie\CookieProviderInterface;

class LegacyCookieProvider implements CookieProviderInterface
{
    public function __construct(private readonly CookieProviderInterface $inner)
    {
    }

    public function getCookieGroups(): array
    {
        return $this->inner->getCookieGroups();
    }
}
