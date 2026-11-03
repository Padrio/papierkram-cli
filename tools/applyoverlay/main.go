// Command applyoverlay applies an OpenAPI Overlay to a JSON OpenAPI spec and
// writes the patched spec as deterministic JSON.
//
// Usage: go run ./tools/applyoverlay <spec.json> <overlay.yaml> <out.json>
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Padrio/papierkram-cli/internal/spec/patch"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "applyoverlay:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 {
		return errors.New("usage: applyoverlay <spec.json> <overlay.yaml> <out.json>")
	}
	spec, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	ov, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	out, err := patch.Apply(spec, ov)
	if err != nil {
		return err
	}
	return os.WriteFile(args[2], out, 0o644)
}
