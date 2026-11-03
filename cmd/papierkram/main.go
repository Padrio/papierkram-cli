// Command papierkram is the CLI for the Papierkram API.
package main

import (
	"os"

	"github.com/Padrio/papierkram-cli/internal/cli"
)

func main() {
	os.Exit(cli.Main())
}
