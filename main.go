package main

import (
	"github.com/alecthomas/kong"
	"github.com/brianmcgee/nvix/internal/cli"
)

func main() {
	ctx := kong.Parse(&cli.Cli)
	ctx.FatalIfErrorf(ctx.Run())
}
