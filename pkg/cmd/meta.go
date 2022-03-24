package cmd

import (
	"context"
	"flag"

	"github.com/google/go-github/v43/github"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/mitchellh/cli"
	"golang.org/x/oauth2"
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

func (m *Meta) GitHubClient() (*github.Client, error) {
	hc := cleanhttp.DefaultClient()
	if m.token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: m.token},
		)
		hc = oauth2.NewClient(context.TODO(), ts)
	}
	client := github.NewClient(hc)
	return client, nil
}
