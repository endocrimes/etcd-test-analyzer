package cmd

import (
	"flag"

	"github.com/mitchellh/cli"
)

var generalOptions = `
	-github-token=<string>
		The token that should be used to authenticate against the GitHub API.
`

// Meta contains the meta-options and functionality that nearly every
// Nomad command inherits.
type Meta struct {
	UI cli.Ui

	// Whether to not-colorize output
	noColor bool

	// Whether to force colorized output
	forceColor bool

	// token is used for ACLs to access privileged information/increase rate limits
	token string
}

// FlagSet returns a FlagSet with the common flags that every
// command implements.
func (m *Meta) FlagSet(n string) *flag.FlagSet {
	f := flag.NewFlagSet(n, flag.ContinueOnError)
	f.BoolVar(&m.noColor, "no-color", false, "")
	f.BoolVar(&m.forceColor, "force-color", false, "")
	f.StringVar(&m.token, "token", "", "")

	return f
}
