package tui

import "fmt"

// PrintBanner prints the Shopware CLI header box to stdout.
func PrintBanner() {
	fmt.Println()
	fmt.Println(RenderHeader())
	fmt.Println()
}
