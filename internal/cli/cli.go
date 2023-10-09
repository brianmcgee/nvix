package cli

import (
	"github.com/brianmcgee/nvix/internal/cli/store"
)

var Cli struct {
	Store store.Cli `cmd:""`
}
