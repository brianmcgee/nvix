package cli

type runCmd struct {
}

func (c *runCmd) Run() error {
	println("hello world")
	return nil
}
