package store

type Cli struct {
	Run  Run  `cmd:"" default:""`
	Init Init `cmd:""`
}
