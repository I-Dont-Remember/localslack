// Command localslack is the local Slack API emulator.
package main

import (
	"github.com/alecthomas/kong"

	"localslack/internal/cli"
)

type app struct {
	Start cli.StartCmd `cmd:"" help:"Start the local Slack API server."`
}

func main() {
	a := app{}
	kctx := kong.Parse(&a)
	if err := kctx.Run(); err != nil {
		kctx.Fatalf("%v", err)
	}
}
