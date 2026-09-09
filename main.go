package main

import (
	"context"
	"os"

	"github.com/shopware/shopware-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute(context.Background()))
}
