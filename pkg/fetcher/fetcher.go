package fetcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/go-github/v43/github"
	"github.com/mitchellh/cli"
)

// Fetcher is an interface that abstracts gathering test results from the github API.
type Fetcher interface {
	FindWorkflowRuns(ctx context.Context) (map[*github.Workflow][]*github.WorkflowRun, error)
	DownloadArtifactsForWorkflowRun(ctx context.Context, runID int64, path string) (bool, error)
}

type githubFetcher struct {
	c          *github.Client
	ui         cli.Ui
	repoOwner  string
	repoName   string
	branchName string
	maxAge     time.Duration
}

var _ Fetcher = &githubFetcher{}

func New(ghClient *github.Client, ui cli.Ui, repoSlug, branchName string, maxAge time.Duration) (Fetcher, error) {
	segments := strings.Split(repoSlug, "/")
	if len(segments) != 2 {
		return nil, fmt.Errorf("invalid repo slug: expected form org/repo, but %d segments were found", len(segments))
	}

	return &githubFetcher{
		c:          ghClient,
		ui:         ui,
		repoOwner:  segments[0],
		repoName:   segments[1],
		branchName: branchName,
		maxAge:     maxAge,
	}, nil
}

func (g *githubFetcher) FindWorkflowRuns(ctx context.Context) (map[*github.Workflow][]*github.WorkflowRun, error) {
	g.ui.Info("Fetching workflows")
	// We only fetch 1 page of workflows rather than paginating.
	// I hope we do not end up with more than 100 workflows configured.
	wkfls, _, err := g.c.Actions.ListWorkflows(ctx, g.repoOwner, g.repoName, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, err
	}
	g.ui.Info(fmt.Sprintf("Retrieved %d workflows", wkfls.GetTotalCount()))

	runsByWorkflow := make(map[*github.Workflow][]*github.WorkflowRun)
	for _, wkfl := range wkfls.Workflows {
		runs, err := g.fetchRecentRunsForWorkflow(ctx, wkfl.GetID())
		if err != nil {
			return nil, err
		}

		runsByWorkflow[wkfl] = runs
	}

	return runsByWorkflow, nil
}

func (g *githubFetcher) DownloadArtifactsForWorkflowRun(ctx context.Context, runID int64, dir string) (bool, error) {
	// We only expect a single artifact per job, so don't bother paginating here.
	artifacts, _, err := g.c.Actions.ListWorkflowRunArtifacts(ctx, g.repoOwner, g.repoName, runID, &github.ListOptions{})
	if err != nil {
		return false, err
	}

	count := artifacts.GetTotalCount()
	if count == 0 {
		return false, nil
	}
	if count > 1 {
		g.ui.Warn(fmt.Sprintf("Expected a single artifact per run, found: %d for run %d, only using the first", count, runID))
	}

	artifact := artifacts.Artifacts[0]

	g.ui.Info(fmt.Sprintf("Downloading artifact (%d/%s) for run: %d", artifact.GetID(), artifact.GetName(), runID))
	_, resp, err := g.c.Actions.DownloadArtifact(ctx, g.repoOwner, g.repoName, artifact.GetID(), true)
	if err != nil {
		return false, err
	}

	out, err := os.Create(path.Join(dir, "artifact.zip"))
	if err != nil {
		return false, err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return true, err
}

func (g *githubFetcher) fetchRecentRunsForWorkflow(ctx context.Context, workflowID int64) ([]*github.WorkflowRun, error) {
	rc := &runCollector{maxAge: g.maxAge}
	page := 1

	for {
		runs, resp, err := g.c.Actions.ListWorkflowRunsByID(ctx, g.repoOwner, g.repoName, workflowID, &github.ListWorkflowRunsOptions{
			Branch: g.branchName,
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: 100,
			},
		})
		if err != nil {
			if _, ok := err.(*github.RateLimitError); ok {
				g.ui.Warn(fmt.Sprintf("hit rate limit: %q", resp.Rate.Reset))
				continue
			}
			if _, ok := err.(*github.AbuseRateLimitError); ok {
				g.ui.Warn(fmt.Sprintf("hit secondary rate limit: %q", resp.Rate.Reset))
				continue
			}

			return rc.runs, err
		}

		for _, run := range runs.WorkflowRuns {
			admitted := rc.addRun(run)
			if !admitted {
				break
			}
		}

		page = resp.NextPage
		if page == 0 {
			break
		}
	}

	return rc.runs, nil
}

type runCollector struct {
	runs []*github.WorkflowRun

	maxAge   time.Duration
	maxCount int
}

func (r *runCollector) addRun(run *github.WorkflowRun) bool {
	if r.maxAge != 0 {
		oldestDate := time.Now().Add(-r.maxAge)
		if run.GetCreatedAt().Before(oldestDate) {
			return false
		}
	}

	if r.maxCount > 0 && len(r.runs) >= r.maxCount {
		return false
	}

	r.runs = append(r.runs, run)
	return true
}
