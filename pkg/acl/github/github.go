package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"gopkg.in/yaml.v3"
)

const (
	githubHost   = "github.com"
	rawGithubURL = "https://raw.githubusercontent.com"
	apiBaseURL   = "https://api.github.com"
)

var (
	ErrUnsupportedHost  = fmt.Errorf("github.com is the only supported host")
	ErrInvalidPath      = fmt.Errorf("invalid repository path")
	ErrZapstoreNotFound = fmt.Errorf("zapstore.yaml not found")
)

// Client is a GitHub client that can fetch repository metadata and zapstore.yaml files.
type Client struct {
	http   *http.Client
	config Config
}

// NewClient creates a new Client with the given configuration.
func NewClient(c Config) Client {
	return Client{
		http:   &http.Client{Timeout: c.Timeout},
		config: c,
	}
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.config.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.Token)
	}
}

// RepoInfo holds the information fetched from a GitHub repository.
type RepoInfo struct {
	// When the repository was created
	CreatedAt time.Time

	// The number of stars the repository has received
	Stars int

	// The pubkey in the zapstore.yaml file. If not found, it will be an empty string.
	Pubkey string
}

// zapstoreConfig represents the relevant fields of a zapstore.yaml file.
type zapstoreConfig struct {
	Pubkey string `yaml:"pubkey"`
}

// githubResponse represents the relevant fields of the GitHub repo API response.
type githubResponse struct {
	StargazersCount int    `json:"stargazers_count"`
	CreatedAt       string `json:"created_at"`
}

// RepoInfo fetches the repository metadata and zapstore.yaml for the given GitHub repo URL.
// It returns a RepoInfo containing the creation date, star count, and pubkey from zapstore.yaml.
func (c *Client) RepoInfo(ctx context.Context, repoURL string) (RepoInfo, error) {
	owner, repo, err := ParseURL(repoURL)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("failed to parse repo URL: %w", err)
	}

	stars, createdAt, err := c.fetchRepoMeta(ctx, owner, repo)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("failed to fetch repo metadata: %w", err)
	}

	pubkey, err := c.fetchPubkey(ctx, owner, repo)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("failed to fetch zapstore.yaml: %w", err)
	}

	return RepoInfo{
		CreatedAt: createdAt,
		Stars:     stars,
		Pubkey:    pubkey,
	}, nil
}

// fetchRepoMeta calls the GitHub REST API to retrieve the star count and creation date of a repo.
func (c *Client) fetchRepoMeta(ctx context.Context, owner, repo string) (stars int, createdAt time.Time, err error) {
	url := fmt.Sprintf("%s/repos/%s/%s", apiBaseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, time.Time{}, fmt.Errorf("unexpected status %d from GitHub API: %s", resp.StatusCode, string(body))
	}

	var response githubResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to decode response: %w", err)
	}

	createdAt, err = time.Parse(time.RFC3339, response.CreatedAt)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to parse created_at %q: %w", response.CreatedAt, err)
	}
	return response.StargazersCount, createdAt, nil
}

// fetchPubkey fetches the zapstore.yaml from the repository and returns the pubkey field.
// It supports both npub and hex pubkeys.
func (c *Client) fetchPubkey(ctx context.Context, owner, repo string) (string, error) {
	rawURL := fmt.Sprintf("%s/%s/%s/HEAD/zapstore.yaml", rawGithubURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrZapstoreNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d fetching zapstore.yaml: %s", resp.StatusCode, string(body))
	}

	var z zapstoreConfig
	if err := yaml.NewDecoder(resp.Body).Decode(&z); err != nil {
		return "", fmt.Errorf("failed to decode zapstore.yaml: %w", err)
	}

	if nostr.IsValidPublicKey(z.Pubkey) {
		return z.Pubkey, nil
	}

	if strings.HasPrefix(z.Pubkey, "npub1") {
		_, data, err := nip19.Decode(z.Pubkey)
		if err != nil {
			return "", fmt.Errorf("failed to decode npub: %w", err)
		}
		return data.(string), nil
	}
	return "", nil
}

// ParseURL parses a GitHub repository URL and returns the owner and repo name.
// It accepts URLs with or without the .git suffix, trailing slashes, and extra sub-paths.
func ParseURL(repoURL string) (owner, repo string, err error) {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return "", "", fmt.Errorf("failed to parse URL: %w", err)
	}
	if u.Host != githubHost {
		return "", "", ErrUnsupportedHost
	}

	cleaned := strings.TrimSuffix(path.Clean(u.Path), ".git")
	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	if len(parts) < 2 {
		return "", "", ErrInvalidPath
	}
	return parts[0], parts[1], nil
}
