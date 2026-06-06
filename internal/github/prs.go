package github

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	ghapi "github.com/google/go-github/v72/github"
)

// maxConcurrentPRs caps the number of PRs fetched in parallel per search query.
const maxConcurrentPRs = 5

// FetchMyPRs returns all open pull requests authored by the authenticated user.
// since, if non-zero, restricts results to PRs updated at or after that time.
func (c *Client) FetchMyPRs(ctx context.Context, since time.Time) ([]PR, error) {
	return c.searchPRs(ctx, "is:pr is:open author:@me archived:false", since)
}

// FetchReviewRequests returns all open PRs where the authenticated user is a
// requested reviewer.
// since, if non-zero, restricts results to PRs updated at or after that time.
func (c *Client) FetchReviewRequests(ctx context.Context, since time.Time) ([]PR, error) {
	return c.searchPRs(ctx, "is:pr is:open review-requested:@me archived:false", since)
}

func (c *Client) searchPRs(ctx context.Context, query string, since time.Time) ([]PR, error) {
	if c.excludeQuery != "" {
		query += " " + c.excludeQuery
	}
	if !since.IsZero() {
		query += " updated:>" + since.UTC().Format("2006-01-02")
	}
	opts := &ghapi.SearchOptions{
		ListOptions: ghapi.ListOptions{PerPage: 100},
	}
	slog.Debug("searching PRs", "host", c.host, "query", query)
	result, _, err := c.inner.Search.Issues(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	slog.Debug("search returned issues", "host", c.host, "count", result.GetTotal())

	type entry struct {
		pr  *PR
		idx int
	}

	sem := make(chan struct{}, maxConcurrentPRs)
	ch := make(chan entry, len(result.Issues))
	var wg sync.WaitGroup

	for i, issue := range result.Issues {
		owner, repo := parseOwnerRepo(issue.GetRepositoryURL())
		if owner == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, issue *ghapi.Issue, owner, repo string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			slog.Debug("fetching PR detail", "host", c.host, "owner", owner, "repo", repo, "number", issue.GetNumber())
			pr, err := c.fetchPRDetail(ctx, owner, repo, issue)
			if err != nil {
				slog.Warn("fetch PR detail failed", "owner", owner, "repo", repo, "number", issue.GetNumber(), "err", err)
				return
			}

			slog.Debug("fetched PR detail",
				"host", c.host,
				"owner", owner,
				"repo", repo,
				"number", pr.Number,
				"title", pr.Title,
				"draft", pr.IsDraft,
				"ci", pr.CIStatus,
				"review", pr.ReviewState)

			ch <- entry{pr, idx}
		}(i, issue, owner, repo)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect and re-order by original search rank.
	ordered := make([]*PR, len(result.Issues))
	for e := range ch {
		ordered[e.idx] = e.pr
	}
	var prs []PR
	for _, pr := range ordered {
		if pr != nil {
			prs = append(prs, *pr)
		}
	}
	return prs, nil
}

// FetchPRDetail fetches full detail for a single PR by number.
func (c *Client) FetchPRDetail(ctx context.Context, owner, repo string, number int) (*PR, error) {
	issue := &ghapi.Issue{Number: &number}
	return c.fetchPRDetail(ctx, owner, repo, issue)
}

func (c *Client) fetchPRDetail(ctx context.Context, owner, repo string, issue *ghapi.Issue) (*PR, error) {
	number := issue.GetNumber()

	// Fetch PR metadata and review list concurrently — neither depends on the other.
	var ghPR *ghapi.PullRequest
	var reviews []*ghapi.PullRequestReview
	var getPRErr, listReviewsErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ghPR, _, getPRErr = c.inner.PullRequests.Get(ctx, owner, repo, number)
	}()
	go func() {
		defer wg.Done()
		reviews, _, listReviewsErr = c.inner.PullRequests.ListReviews(ctx, owner, repo, number, nil)
	}()
	wg.Wait()

	if getPRErr != nil {
		return nil, getPRErr
	}
	if listReviewsErr != nil {
		slog.Warn("list reviews failed", "owner", owner, "repo", repo, "number", number, "err", listReviewsErr)
	}

	sha := ghPR.GetHead().GetSHA()
	ciStatus := c.fetchCIStatus(ctx, owner, repo, sha)

	return &PR{
		Server:       c.host,
		Owner:        owner,
		Repo:         repo,
		Number:       number,
		Title:        ghPR.GetTitle(),
		URL:          ghPR.GetHTMLURL(),
		Author:       ghPR.GetUser().GetLogin(),
		HeadRef:      ghPR.GetHead().GetRef(),
		HeadSHA:      sha,
		IsDraft:      ghPR.GetDraft(),
		ReviewState:  aggregateReviewState(reviews),
		CIStatus:     ciStatus,
		Merge:        parseMergeability(ghPR),
		CommentCount: ghPR.GetComments() + ghPR.GetReviewComments(),
		UpdatedAt:    ghPR.GetUpdatedAt().Time,
	}, nil
}

// fetchCIStatus aggregates modern check runs for the given SHA.
func (c *Client) fetchCIStatus(ctx context.Context, owner, repo, sha string) CIStatus {
	if sha == "" {
		return CIUnknown
	}

	runs, _, err := c.inner.Checks.ListCheckRunsForRef(ctx, owner, repo, sha,
		&ghapi.ListCheckRunsOptions{ListOptions: ghapi.ListOptions{PerPage: 100}})
	if err != nil {
		runs = nil
	}

	return aggregateCIStatus(repo, runs)
}

// --- aggregation helpers -----------------------------------------------------

func aggregateReviewState(reviews []*ghapi.PullRequestReview) ReviewState {
	// Take the most recent review per reviewer; DISMISSED overrides prior state.
	latest := make(map[string]string)
	for _, r := range reviews {
		login := r.GetUser().GetLogin()
		state := r.GetState()
		switch state {
		case "DISMISSED":
			delete(latest, login)
		case "APPROVED", "CHANGES_REQUESTED":
			latest[login] = state
		}
	}
	hasApproved, hasChanges := false, false
	for _, s := range latest {
		switch s {
		case "APPROVED":
			hasApproved = true
		case "CHANGES_REQUESTED":
			hasChanges = true
		}
	}
	if hasChanges {
		return ReviewChangesRequested
	}
	if hasApproved {
		return ReviewApproved
	}
	return ReviewPending
}

func aggregateCIStatus(repo string, runs *ghapi.ListCheckRunsResults) CIStatus {
	failing, pending, passing := false, false, false

	slog.Debug("aggregating CI status", "repo", repo)

	if runs != nil {
		for _, run := range runs.CheckRuns {
			slog.Debug("check run",
				"repo", repo,
				"name", run.GetName(),
				"status", run.GetStatus(),
				"conclusion", run.GetConclusion(),
			)
			switch run.GetConclusion() {
			case "success", "skipped", "neutral":
				passing = true
			case "failure", "cancelled", "timed_out", "action_required":
				failing = true
			case "": // still running
				switch run.GetStatus() {
				case "in_progress", "queued", "waiting":
					pending = true
				}
			}
		}
	}

	switch {
	case failing:
		return CIFailing
	case pending:
		return CIPending
	case passing:
		return CIPassing
	default:
		return CIUnknown
	}
}

func parseMergeability(pr *ghapi.PullRequest) Mergeability {
	switch pr.GetMergeableState() {
	case "clean":
		return MergeMergeable
	case "dirty":
		return MergeConflicted
	case "blocked", "behind", "unstable":
		return MergeBlocked
	default:
		return MergeUnknown
	}
}

// parseOwnerRepo extracts owner and repo from a GitHub repository URL.
// Works for both github.com and GHE: ".../repos/owner/repo"
func parseOwnerRepo(repoURL string) (owner, repo string) {
	_, rest, ok := strings.Cut(repoURL, "/repos/")
	if !ok {
		return "", ""
	}
	owner, repo, ok = strings.Cut(rest, "/")
	if !ok {
		return "", ""
	}
	return owner, strings.TrimRight(repo, "/")
}
