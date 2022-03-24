package cmd

import "strings"

var (
	analyzeSynopsis = "Analyze test failures from a target repository"

	analyzeHelp = `
Usage: etcd-test-analyzer run [options] [args]

	This command will run the test analyzer with the given parameters.
	The analyzer can be pointed towards various branches across different periods
	of time for the purpose of gathering different statistics.

General Options:

` + generalOptions + `

Run Options:

	-repo=<string>
		The repo slug that should be used as the target for data collection.
		Default is 'etcd-io/etcd'.

	-branch=<string>
		The branch name that should be used for gathering statistics in the target
		repo.
		Default is 'main'.
`
)

type RunCommand struct {
	Meta *Meta
}

func (a *RunCommand) Help() string {
	return strings.TrimSpace(analyzeHelp)
}

func (a *RunCommand) Synopsis() string {
	return strings.TrimSpace(analyzeSynopsis)
}

func (a *RunCommand) Name() string {
	return "run"
}

func (a *RunCommand) Run(args []string) int {
	flags := a.Meta.FlagSet(a.Name())
	flags.Usage = func() { a.Meta.UI.Output(a.Help()) }

	var repoSlug string
	var branchName string

	flags.StringVar(&repoSlug, "repo", "etcd-io/etcd", "")
	flags.StringVar(&branchName, "branch", "main", "")

	if err := flags.Parse(args); err != nil {
		return 1
	}

	return 0
}
