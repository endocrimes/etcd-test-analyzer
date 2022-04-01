package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/endocrimes/etcd-test-analyzer/pkg/fetcher"
	"github.com/endocrimes/etcd-test-analyzer/pkg/unzipper"
	"github.com/fatih/color"
	"github.com/google/go-github/v43/github"
	"github.com/joshdk/go-junit"
	"github.com/kataras/tablewriter"
	"github.com/lensesio/tableprinter"
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

	-workflow=<string>
		Name of the workflow to inspect.
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
	var workflowName string
	var maxAge time.Duration

	flags.StringVar(&repoSlug, "repo", "etcd-io/etcd", "")
	flags.StringVar(&branchName, "branch", "main", "")
	flags.StringVar(&workflowName, "workflow", "", "")
	flags.DurationVar(&maxAge, "max-age", 7*24*time.Hour, "")

	if err := flags.Parse(args); err != nil {
		return 1
	}

	segments := strings.Split(repoSlug, "/")
	if len(segments) != 2 {
		a.Meta.UI.Error(fmt.Sprintf("invalid repo slug: expected form org/repo, but %d segments were found", len(segments)))
		return 1
	}

	gh, err := a.Meta.GitHubClient()
	if err != nil {
		a.Meta.UI.Error(fmt.Sprintf("failed to setup github client, err: %v", err))
		return 1
	}

	a.Meta.UI.Info(color.GreenString("Fetching workflow runs from GitHub"))

	fc := fetcher.NewClient(gh, segments[0], segments[1])

	workflows, err := fc.ListWorkflows(context.Background())
	if err != nil {
		a.Meta.UI.Error(fmt.Sprintf("failed to find workflows: %v", err))
		return 1
	}

	for _, workflow := range workflows {
		if workflowName != "" && !strings.Contains(workflow.Name, workflowName) {
			continue
		}

		a.Meta.UI.Info(fmt.Sprintf("Analyzing workflow %q", workflow.Name))

		ui := prefixedUI(fmt.Sprintf("    [%s]: ", workflow.Name), a.Meta.UI)
		f, err := fetcher.New(gh, ui, segments[0], segments[1], branchName, maxAge)
		if err != nil {
			a.Meta.UI.Error(fmt.Sprintf("failed to setup fetcher, err: %v", err))
			return 1
		}

		ui.Info("Finding workflow runs")

		runs, err := f.FindWorkflowRuns(context.Background(), workflow)
		if err != nil {
			a.Meta.UI.Error(fmt.Sprintf("failed to list runs for %s, err: %v", workflow.Name, err))
			return 1
		}

		ui.Info("Downloading artifacts")
		results, err := downloadArtifacts(f, ui, workflow, runs)
		if err != nil {
			a.Meta.UI.Error(fmt.Sprintf("failed to process results for %s, err: %v", workflow.Name, err))
			return 1
		}

		totalRuns := len(results)
		totalFails := 0
		totalPasses := 0

		testTable := make(map[string]TableEntry)

		for _, res := range results {
			if res.Failed() {
				totalFails++
			} else {
				totalPasses++
			}

			for _, suite := range res.TestResults {
				if suite.Totals.Failed > 0 || suite.Totals.Error > 0 {
					for _, t := range suite.Tests {
						if t.Status == junit.StatusFailed || t.Status == junit.StatusError {
							entry, ok := testTable[t.Name]
							if !ok {
								entry = TableEntry{
									TestName:    t.Name,
									TestPackage: t.Classname,
								}
							}

							entry.FailureCount++
							testTable[t.Name] = entry
						}
					}
				}
			}
		}

		a.Meta.UI.Info(fmt.Sprintf("Runs: %d, Pass: %d, Fail: %d, Pcnt: %f", totalRuns, totalPasses, totalFails, float64(totalPasses)/float64(totalRuns)))

		printer := tableprinter.New(os.Stdout)

		entries := []TableEntry{}
		for _, entry := range testTable {
			entries = append(entries, entry)
		}

		sort.Slice(entries, func(i, j int) bool {
			return entries[j].FailureCount <= entries[i].FailureCount
		})

		printer.BorderTop, printer.BorderBottom, printer.BorderLeft, printer.BorderRight = true, true, true, true
		printer.CenterSeparator = "│"
		printer.ColumnSeparator = "│"
		printer.RowSeparator = "─"
		printer.HeaderBgColor = tablewriter.BgBlackColor
		printer.HeaderFgColor = tablewriter.FgGreenColor
		printer.Print(entries)
	}

	return 0
}

type TableEntry struct {
	TestName     string `header:"Test Name"`
	TestPackage  string `header:"Package"`
	FailureCount int    `header:"Failure Count"`
	LastFailure  time.Time
}

type ProcessedRunArtifact struct {
	Run              *github.WorkflowRun
	TestResults      []junit.Suite
	AggregatedTotals junit.Totals
}

func (p *ProcessedRunArtifact) Failed() bool {
	return p.AggregatedTotals.Failed > 0 || p.AggregatedTotals.Error > 0
}

func downloadArtifacts(f fetcher.Fetcher, ui cli.Ui, workflow *fetcher.Workflow, runs []*github.WorkflowRun) ([]*ProcessedRunArtifact, error) {
	rootDir, err := os.MkdirTemp("", "test-results-")
	if err != nil {
		return nil, err
	}

	ui.Info(fmt.Sprintf("Downloading artifacts for %s to %q", workflow.Name, rootDir))

	result := []*ProcessedRunArtifact{}
	for _, run := range runs {
		runDir := path.Join(rootDir, fmt.Sprintf("%d", run.GetID()))

		err := os.MkdirAll(runDir, os.ModePerm)
		if err != nil {
			return nil, err
		}

		zipPath, err := f.DownloadArtifactsForWorkflowRun(context.Background(), run.GetID(), runDir)
		if err != nil {
			return nil, err
		}

		if zipPath == "" {
			ui.Warn("no artifacts found")
			continue
		}

		targetDir := path.Join(runDir, "artifacts")
		err = unzipper.Unzip(zipPath, targetDir)
		if err != nil {
			return nil, err
		}

		suites, err := junit.IngestDir(targetDir)
		if err != nil {
			return nil, err
		}

		totals := junit.Totals{}

		aggregate := func(l, r junit.Totals) junit.Totals {
			nl := l
			nl.Duration += r.Duration
			nl.Error += r.Error
			nl.Failed += r.Failed
			nl.Passed += r.Passed
			nl.Skipped += r.Skipped
			nl.Tests += r.Tests
			return nl
		}

		for _, suite := range suites {
			suite.Aggregate()
			totals = aggregate(totals, suite.Totals)
		}

		res := &ProcessedRunArtifact{
			Run:              run,
			AggregatedTotals: totals,
			TestResults:      suites,
		}

		result = append(result, res)
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
