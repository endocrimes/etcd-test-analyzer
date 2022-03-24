package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/endocrimes/etcd-test-analyzer/pkg/fetcher"
	"github.com/fatih/color"
	"github.com/google/go-github/v43/github"
	"github.com/mitchellh/cli"
)

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

	-max-age=<duration>
		Max age of workflow runs that should be fetched for analysis.
		Default is '168h'
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

type workflowPathInfo struct {
	Run  *github.WorkflowRun
	Path string
}

func (a *RunCommand) Run(args []string) int {
	flags := a.Meta.FlagSet(a.Name())
	flags.Usage = func() { a.Meta.UI.Output(a.Help()) }

	var repoSlug string
	var branchName string
	var maxAge time.Duration

	flags.StringVar(&repoSlug, "repo", "etcd-io/etcd", "")
	flags.StringVar(&branchName, "branch", "main", "")
	flags.DurationVar(&maxAge, "max-age", 7*24*time.Hour, "")

	if err := flags.Parse(args); err != nil {
		return 1
	}

	gh, err := a.Meta.GitHubClient()
	if err != nil {
		a.Meta.UI.Error(fmt.Sprintf("failed to setup github client, err: %v", err))
		return 1
	}

	a.Meta.UI.Info(color.GreenString("Fetching workflow runs from GitHub"))

	f, err := fetcher.New(gh, prefixedUI("    => ", a.Meta.UI), repoSlug, branchName, maxAge)
	if err != nil {
		a.Meta.UI.Error(fmt.Sprintf("failed to setup fetcher, err: %v", err))
		return 1
	}

	runsByWkfl, err := f.FindWorkflowRuns(context.Background())
	if err != nil {
		a.Meta.UI.Error(fmt.Sprintf("failed to fetch, err: %v", err))
		return 1
	}

	a.Meta.UI.Info("Downloading artifacts")
	_, err = downloadArtifacts(f, runsByWkfl)
	if err != nil {
		a.Meta.UI.Error(fmt.Sprintf("failed to fetch artifacts, err: %v", err))
		return 1
	}

	a.Meta.UI.Info("Unzipping artifacts")

	return 0
}

func downloadArtifacts(f fetcher.Fetcher, runsByWorkflow map[*github.Workflow][]*github.WorkflowRun) (map[*github.Workflow][]*workflowPathInfo, error) {
	rootDir, err := os.MkdirTemp("", "test-results-")
	if err != nil {
		return nil, err
	}

	result := make(map[*github.Workflow][]*workflowPathInfo)
	for wkfl, runs := range runsByWorkflow {
		wkflDir := path.Join(rootDir, fmt.Sprintf("%d", wkfl.GetID()))
		err := os.Mkdir(wkflDir, 0770)
		if err != nil {
			return nil, err
		}

		runsInfo := []*workflowPathInfo{}
		for idx := range runs {
			run := runs[idx]
			runDir := path.Join(wkflDir, fmt.Sprintf("%d", run.GetID()))
			err := os.Mkdir(runDir, 0770)
			if err != nil {
				return nil, err
			}

			found, err := f.DownloadArtifactsForWorkflowRun(context.Background(), run.GetID(), runDir)
			if err != nil {
				return nil, err
			}

			if !found {
				// No Artifacts for run
				continue
			}

			runsInfo = append(runsInfo, &workflowPathInfo{Run: run, Path: path.Join(runDir, "artifact.zip")})
		}

		result[wkfl] = runsInfo
	}

	return result, nil
}

func prefixedUI(prefix string, ui cli.Ui) cli.Ui {
	return &cli.PrefixedUi{
		AskPrefix:       prefix,
		AskSecretPrefix: prefix,
		OutputPrefix:    prefix,
		InfoPrefix:      prefix,
		ErrorPrefix:     prefix,
		WarnPrefix:      prefix,
		Ui:              ui,
	}
}
