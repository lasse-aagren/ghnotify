package github

import (
	"context"
	"log"
	"strings"

	ghapi "github.com/google/go-github/v72/github"
)

// FetchMyPRs returns all open pull requests authored by the authenticated user.
func (c *Client) FetchMyPRs(ctx context.Context) ([]PR, error) {
	return c.searchPRs(ctx, "is:pr is:open author:@me archived:false")
}

// FetchReviewRequests returns all open PRs where the authenticated user is a
// requested reviewer.
func (c *Client) FetchReviewRequests(ctx context.Context) ([]PR, error) {
	return c.searchPRs(ctx, "is:pr is:open review-requested:@me archived:false")
}

func (c *Client) searchPRs(ctx context.Context, query string) ([]PR, error) {
	opts := &ghapi.SearchOptions{
		ListOptions: ghapi.ListOptions{PerPage: 100},
	}
	result, _, err := c.inner.Search.Issues(ctx, query, opts)
	if err != nil {
		return nil, err
	}

	var prs []PR
	for _, issue := range result.Issues {
		owner, repo := parseOwnerRepo(issue.GetRepositoryURL())
		if owner == "" {
			continue
		}
		pr, err := c.fetchPRDetail(ctx, owner, repo, issue.GetNumber())
		if err != nil {
			log.Printf("ghnotify: fetch PR detail %s/%s#%d: %v", owner, repo, issue.GetNumber(), err)
			continue
		}
		prs = append(prs, *pr)
	}
	return prs, nil
}

func (c *Client) fetchPRDetail(ctx context.Context, owner, repo string, number int) (*PR, error) {
	ghPR, _, err := c.inner.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}

	reviews, _, err := c.inner.PullRequests.ListReviews(ctx, owner, repo, number, nil)
	if err != nil {
		log.Printf("ghnotify: list reviews %s/%s#%d: %v", owner, repo, number, err)
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
		IsDraft:      ghPR.GetDraft(),
		ReviewState:  aggregateReviewState(reviews),
		CIStatus:     ciStatus,
		Merge:        parseMergeability(ghPR),
		CommentCount: ghPR.GetComments() + ghPR.GetReviewComments(),
		UpdatedAt:    ghPR.GetUpdatedAt().Time,
	}, nil
}

// fetchCIStatus aggregates both legacy commit statuses and modern check runs.
func (c *Client) fetchCIStatus(ctx context.Context, owner, repo, sha string) CIStatus {
	if sha == "" {
		return CIUnknown
	}

	var legacyState string
	combined, _, err := c.inner.Repositories.GetCombinedStatus(ctx, owner, repo, sha, nil)
	if err == nil {
		legacyState = combined.GetState()
	}

	runs, _, err := c.inner.Checks.ListCheckRunsForRef(ctx, owner, repo, sha,
		&ghapi.ListCheckRunsOptions{ListOptions: ghapi.ListOptions{PerPage: 100}})
	if err != nil {
		runs = nil
	}

	return aggregateCIStatus(legacyState, runs)
}

// --- aggregation helpers -----------------------------------------------------

func aggregateReviewState(reviews []*ghapi.PullRequestReview) ReviewState {
	// Take the most recent review per reviewer; DISMISSED overrides prior state.
	latest := make(map[string]string)
	for _, r := range reviews {
		login := r.GetUser().GetLogin()
		state := r.GetState()
		if state == "DISMISSED" {
			delete(latest, login)
		} else if state == "APPROVED" || state == "CHANGES_REQUESTED" {
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

func aggregateCIStatus(legacyState string, runs *ghapi.ListCheckRunsResults) CIStatus {
	failing, pending, passing := false, false, false

	switch legacyState {
	case "success":
		passing = true
	case "failure", "error":
		failing = true
	case "pending":
		pending = true
	}

	if runs != nil {
		for _, run := range runs.CheckRuns {
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
	const marker = "/repos/"
	idx := strings.Index(repoURL, marker)
	if idx < 0 {
		return "", ""
	}
	rest := repoURL[idx+len(marker):]
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], strings.TrimRight(parts[1], "/")
}
