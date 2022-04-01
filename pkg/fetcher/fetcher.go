package fetcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/google/go-github/v43/github"
	"github.com/mitchellh/cli"
)

type Workflow struct {
	ID   int64
	Name string
}

// Client is an interface that exposes a subset of the GitHub API
type Client interface {
	ListWorkflows(ctx context.Context) ([]*Workflow, error)
}

type client struct {
	c         *github.Client
	repoOwner string
	repoName  string
}

// NewClient creates a Client from an existing go-github client.
func NewClient(ghClient *github.Client, repoOwner, repoName string) Client {
	return &client{
		c:         ghClient,
		repoOwner: repoOwner,
		repoName:  repoName,
	}
}

func (c *client) ListWorkflows(ctx context.Context) ([]*Workflow, error) {
	// We only fetch 1 page of workflows rather than paginating.
	// I hope we do not end up with more than 100 workflows configured.
	wkfls, _, err := c.c.Actions.ListWorkflows(ctx, c.repoOwner, c.repoName, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, err
	}

	res := []*Workflow{}
	for _, wkfl := range wkfls.Workflows {
		res = append(res, &Workflow{
			ID:   wkfl.GetID(),
			Name: wkfl.GetName(),
		})
	}
	return res, nil
}

// Fetcher is an interface that abstracts gathering test results from the github API.
type Fetcher interface {
	FindWorkflowRuns(ctx context.Context, workflow *Workflow) ([]*github.WorkflowRun, error)
	DownloadArtifactsForWorkflowRun(ctx context.Context, runID int64, path string) (string, error)
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

func New(ghClient *github.Client, ui cli.Ui, repoOwner, repoName, branchName string, maxAge time.Duration) (Fetcher, error) {
	return &githubFetcher{
		c:          ghClient,
		ui:         ui,
		repoOwner:  repoOwner,
		repoName:   repoName,
		branchName: branchName,
		maxAge:     maxAge,
	}, nil
}

func (g *githubFetcher) FindWorkflowRuns(ctx context.Context, workflow *Workflow) ([]*github.WorkflowRun, error) {
	return g.fetchRecentRunsForWorkflow(ctx, workflow.ID)
}

func (g *githubFetcher) DownloadArtifactsForWorkflowRun(ctx context.Context, runID int64, dir string) (string, error) {
	// We only expect a single artifact per job, so don't bother paginating here.
	artifacts, _, err := g.c.Actions.ListWorkflowRunArtifacts(ctx, g.repoOwner, g.repoName, runID, &github.ListOptions{})
	if err != nil {
		return "", err
	}

	count := artifacts.GetTotalCount()
	if count == 0 {
		return "", nil
	}
	if count > 1 {
		g.ui.Warn(fmt.Sprintf("Expected a single artifact per run, found: %d for run %d, only using the first", count, runID))
	}

	artifact := artifacts.Artifacts[0]
	downloadURL, _, err := g.c.Actions.DownloadArtifact(ctx, g.repoOwner, g.repoName, artifact.GetID(), true)
	if err != nil {
		return "", err
	}

	resp, err := g.c.Client().Get(downloadURL.String())
	if err != nil {
		return "", err
	}

	filePath := path.Join(dir, "artifact.zip")
	out, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return filePath, err
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
