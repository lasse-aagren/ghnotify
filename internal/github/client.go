package github

import (
	"context"
	"errors"
	"net/http"

	ghapi "github.com/google/go-github/v72/github"
	"golang.org/x/oauth2"
)

// Client wraps go-github with our host information.
type Client struct {
	inner *ghapi.Client
	host  string
}

// NewClient returns a token-authenticated client for host.
// Use "github.com" for github.com or a bare hostname for GitHub Enterprise.
func NewClient(host, token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)

	var inner *ghapi.Client
	if host == "github.com" {
		inner = ghapi.NewClient(tc)
	} else {
		inner, _ = ghapi.NewClient(tc).WithEnterpriseURLs(
			"https://"+host+"/api/v3/",
			"https://"+host+"/api/uploads/",
		)
	}
	return &Client{inner: inner, host: host}
}

// IsUnauthorized reports whether err is a 401 from the GitHub API.
func IsUnauthorized(err error) bool {
	var ghErr *ghapi.ErrorResponse
	return errors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusUnauthorized
}
