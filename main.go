package main

import (
	"log"
	"os"

	"github.com/endocrimes/etcd-test-analyzer/pkg/cmd"
	"github.com/endocrimes/etcd-test-analyzer/pkg/version"
	"github.com/mattn/go-colorable"
	"github.com/mitchellh/cli"
)

func main() {
	c := cli.NewCLI("etcd-test-analyzer", version.GetVersion().FullVersionNumber(true))
	c.Args = os.Args[1:]

	meta := new(cmd.Meta)
	if meta.UI == nil {
		meta.UI = &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      colorable.NewColorableStdout(),
			ErrorWriter: colorable.NewColorableStderr(),
		}
	}

	c.Commands = map[string]cli.CommandFactory{
		"run": func() (cli.Command, error) {
			return &cmd.RunCommand{
				Meta: meta,
			}, nil
		},
	}

	exitStatus, err := c.Run()
	if err != nil {
		log.Println(err)
	}

	os.Exit(exitStatus)
}
