// Command localslack is the LocalSlack API emulator.
package main

import (
	"github.com/alecthomas/kong"

	"localslack/internal/cli"
)

type app struct {
	Start cli.StartCmd `cmd:"" help:"Start the LocalSlack server."`
	Stop  cli.StopCmd  `cmd:"" help:"Stop the background LocalSlack server."`
	Reset cli.ResetCmd `cmd:"" help:"Reset a running server's state (POST /_localslack/reset)."`
	Apps  cli.AppsCmd  `cmd:"" help:"Register and inspect apps on a running server."`
}

func main() {
	a := app{}
	kctx := kong.Parse(&a)
	if err := kctx.Run(); err != nil {
		kctx.Fatalf("%v", err)
	}
}
