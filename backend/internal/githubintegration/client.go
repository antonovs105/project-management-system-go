package githubintegration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrGitHubNotFound reports that GitHub returned 404 for a requested resource.
var ErrGitHubNotFound = errors.New("github resource not found")

// ErrGitHubRequestFailed reports that GitHub returned an unexpected response.
var ErrGitHubRequestFailed = errors.New("github request failed")

// Client loads repository and commit metadata from GitHub.
type Client interface {
	Repository(ctx context.Context, owner, name string) (RemoteRepository, error)
	ListCommits(ctx context.Context, owner, name, branch string, since *time.Time, limit int) ([]RemoteCommit, error)
}

// HTTPClient is a small GitHub REST client for repository and commit reads.
type HTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// ClientOption customizes the GitHub HTTP client.
type ClientOption func(*HTTPClient)

// WithToken adds a bearer token for private repositories and higher rate limits.
func WithToken(token string) ClientOption {
	return func(c *HTTPClient) {
		c.token = strings.TrimSpace(token)
	}
}

// WithBaseURL changes the GitHub API base URL, primarily for tests.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *HTTPClient) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		}
	}
}

// WithHTTPClient replaces the default HTTP client, primarily for tests.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *HTTPClient) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// NewHTTPClient creates a GitHub REST client.
func NewHTTPClient(options ...ClientOption) *HTTPClient {
	client := &HTTPClient{
		baseURL:    "https://api.github.com",
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, option := range options {
		option(client)
	}
	return client
}

// Repository returns metadata for one GitHub repository.
func (c *HTTPClient) Repository(ctx context.Context, owner, name string) (RemoteRepository, error) {
	var response struct {
		FullName      string `json:"full_name"`
		HTMLURL       string `json:"html_url"`
		DefaultBranch string `json:"default_branch"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(name)), nil, &response); err != nil {
		return RemoteRepository{}, err
	}
	return RemoteRepository{
		Owner:         response.Owner.Login,
		Name:          response.Name,
		FullName:      response.FullName,
		HTMLURL:       response.HTMLURL,
		DefaultBranch: response.DefaultBranch,
	}, nil
}

// ListCommits returns recent commits for one repository branch.
func (c *HTTPClient) ListCommits(ctx context.Context, owner, name, branch string, since *time.Time, limit int) ([]RemoteCommit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := url.Values{}
	query.Set("per_page", fmt.Sprintf("%d", limit))
	if strings.TrimSpace(branch) != "" {
		query.Set("sha", strings.TrimSpace(branch))
	}
	if since != nil {
		query.Set("since", since.UTC().Format(time.RFC3339))
	}

	var response []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name  string     `json:"name"`
				Email string     `json:"email"`
				Date  *time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/commits", url.PathEscape(owner), url.PathEscape(name)), query, &response); err != nil {
		return nil, err
	}

	commits := make([]RemoteCommit, 0, len(response))
	for _, item := range response {
		commits = append(commits, RemoteCommit{
			SHA:         item.SHA,
			Message:     item.Commit.Message,
			AuthorName:  item.Commit.Author.Name,
			AuthorEmail: item.Commit.Author.Email,
			AuthoredAt:  item.Commit.Author.Date,
			HTMLURL:     item.HTMLURL,
		})
	}
	return commits, nil
}

// getJSON performs a GitHub GET request and decodes a JSON response.
func (c *HTTPClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "progo")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrGitHubRequestFailed
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		io.Copy(io.Discard, resp.Body)
		return ErrGitHubNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return ErrGitHubRequestFailed
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return ErrGitHubRequestFailed
	}
	return nil
}
