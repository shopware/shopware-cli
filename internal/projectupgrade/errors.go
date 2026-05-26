package projectupgrade

import "errors"

var errNoShopwareInLock = errors.New("no shopware/core or shopware/platform entry found in composer.lock")
