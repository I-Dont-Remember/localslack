// Command capctl is a development tool for the capabilities manifest:
// `capctl docs` regenerates docs/COMPATIBILITY.md, `capctl gaps` lists methods
// that still need a real-Slack recording. Run it from the repository root.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"localslack/internal/capabilities"
)

func main() {
	write := func(name string, data []byte) error {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		return os.WriteFile(name, data, 0o644) //nolint:gosec // generated, world-readable docs
	}
	if err := capabilities.Run(os.Args[1:], os.DirFS("."), write, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
