package tui

import "fmt"

func PrintBanner() {
	banner := `
                @@@@@@@@@@
           @@@@@@@@@@@@@@@@@@
         @@@@@@@@@@@@@@@@@@@@@@@
       @@@@@@@@@@@@@@@
      @@@@@@@@@@
     @@@@@@@@@        @@@@@@@
    @@@@@@@@@@      @@@@@@@@@@@@
    @@@@@@@@@@       @@@@@@@@@@@@@
    @@@@@@@@@@@        @@@@@@@@@@
     @@@@@@@@@@@@          @@@@@
      @@@@@@@@@@@@@@
        @@@@@@@@@@@@@@@@@@
          @@@@@@@@@@@@@@@@@@@@
           @@@@@@@@@@@@@@@@@@@
              @@@@@@@@@@@@@`

	fmt.Println(BlueText.Render(banner))
	fmt.Println()
	fmt.Println(BlueText.Bold(true).Render("  Welcome to Shopware!"))
	fmt.Println()
}
