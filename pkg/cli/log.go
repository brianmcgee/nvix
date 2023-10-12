package cli

import (
	"github.com/charmbracelet/log"
)

type LogOptions struct {
	Verbosity int `name:"verbose" short:"v" type:"counter" default:"0" env:"NVIX_LOG_LEVEL" help:"Set the verbosity of logs e.g. -vv"`
}

func (lo *LogOptions) ConfigureLogger() {
	if lo.Verbosity == 0 {
		log.SetLevel(log.WarnLevel)
	} else if lo.Verbosity == 1 {
		log.SetLevel(log.InfoLevel)
	} else if lo.Verbosity >= 2 {
		log.SetLevel(log.DebugLevel)
	}
}
