package github

import (
	"context"

	ghapi "github.com/google/go-github/v72/github"
)

// Approve submits an approving review for the given PR.
func (c *Client) Approve(ctx context.Context, owner, repo string, number int) error {
	_, _, err := c.inner.PullRequests.CreateReview(ctx, owner, repo, number,
		&ghapi.PullRequestReviewRequest{Event: ghapi.Ptr("APPROVE")})
	return err
}
