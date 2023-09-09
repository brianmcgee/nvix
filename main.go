package main

import (
	"github.com/alecthomas/kong"
	"github.com/brianmcgee/tvix-store-nats/internal/cli"
)

func main() {
	ctx := kong.Parse(&cli.Cmd)
	ctx.FatalIfErrorf(ctx.Run())
}
