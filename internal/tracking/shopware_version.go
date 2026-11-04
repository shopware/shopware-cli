package tracking

import (
	"regexp"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
)

// ShopwareVersionLookup holds one optional background lookup result.
type ShopwareVersionLookup <-chan string

// Ready consumes the version if available, without waiting for filesystem I/O.
func (lookup ShopwareVersionLookup) Ready() string {
	select {
	case version := <-lookup:
		return version
	default:
		return ""
	}
}

// StartShopwareVersion starts a best-effort lookup for eligible project commands.
func StartShopwareVersion(commandName string) ShopwareVersionLookup {
	return startShopwareVersion(commandName, projectShopwareVersion)
}

func startShopwareVersion(commandName string, lookup func() string) ShopwareVersionLookup {
	if !strings.HasPrefix(commandName, "project.") || commandName == "project.create" || !enabled() {
		return nil
	}
	result := make(chan string, 1)
	go func() {
		// Buffer the result so finishing never depends on a consumer.
		result <- lookup()
	}()
	return result
}

// Keep arbitrary development branch names out of telemetry.
var shopwareVersionPattern = regexp.MustCompile(`^(\d+\.\d+\.\d+\.\d+(-rc\d+)?|dev-trunk)$`)

func projectShopwareVersion() string {
	projectRoot, err := shop.FindClosestShopwareProject(false)
	if err != nil {
		return ""
	}
	version := shop.DetectShopwareVersion(projectRoot)
	if !shopwareVersionPattern.MatchString(version) {
		return ""
	}
	return version
}
