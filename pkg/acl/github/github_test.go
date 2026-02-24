package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

var ctx = context.Background()

func TestRepoInfo(t *testing.T) {
	config := NewConfig()
	client := NewClient(config)

	info, err := client.RepoInfo(ctx, "https://github.com/pippellia-btc/TEST")
	if err != nil {
		t.Fatal(err)
	}

	expected := RepoInfo{
		CreatedAt: time.Unix(1771865178, 0).UTC(),
		Stars:     0,
		Pubkey:    "8d555b569d5c4c28c7d489e1d581248b1469d3fce288f32d50dbc53869f32e0e",
	}

	if info != expected {
		t.Errorf("RepoInfo mismatch: got %v, want %v", info, expected)
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		owner, repo string
		err         error
	}{
		{
			name:  "non-GitHub host",
			input: "https://gitlab.com/user/repo",
			err:   ErrUnsupportedHost,
		},
		{
			name:  "no host at all",
			input: "github.com/user/repo",
			err:   ErrUnsupportedHost,
		},
		{
			name:  "missing repo segment",
			input: "https://github.com/user",
			err:   ErrInvalidPath,
		},
		{
			name:  "only host, no path",
			input: "https://github.com",
			err:   ErrInvalidPath,
		},
		{
			name:  "standard URL",
			input: "https://github.com/user/repo",
			owner: "user",
			repo:  "repo",
		},
		{
			name:  "URL with .git suffix",
			input: "https://github.com/user/repo.git",
			owner: "user",
			repo:  "repo",
		},
		{
			name:  "URL with trailing slash",
			input: "https://github.com/user/repo/",
			owner: "user",
			repo:  "repo",
		},
		{
			name:  "URL with .git suffix and trailing slash",
			input: "https://github.com/user/repo.git/",
			owner: "user",
			repo:  "repo",
		},
		{
			name:  "URL with leading and trailing whitespace",
			input: "  https://github.com/user/repo  ",
			owner: "user",
			repo:  "repo",
		},
		{
			name:  "URL with sub-path is truncated to user/repo",
			input: "https://github.com/user/repo/tree/main",
			owner: "user",
			repo:  "repo",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			owner, repo, err := ParseURL(test.input)
			if !errors.Is(err, test.err) {
				t.Fatalf("ParseURL(%q) unexpected error: got %v, want %v", test.input, err, test.err)
			}
			if owner != test.owner {
				t.Errorf("ParseURL(%q) owner: got %q, want %q", test.input, owner, test.owner)
			}
			if repo != test.repo {
				t.Errorf("ParseURL(%q) repo: got %q, want %q", test.input, repo, test.repo)
			}
		})
	}
}
