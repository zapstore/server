package acl

import "testing"

func TestRawZapstoreYAMLURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "standard github URL",
			input: "https://github.com/user/repo",
			want:  "https://raw.githubusercontent.com/user/repo/HEAD/zapstore.yaml",
		},
		{
			name:  "github URL with trailing slash",
			input: "https://github.com/user/repo/",
			want:  "https://raw.githubusercontent.com/user/repo/HEAD/zapstore.yaml",
		},
		{
			name:  "github URL with .git suffix",
			input: "https://github.com/user/repo.git",
			want:  "https://raw.githubusercontent.com/user/repo/HEAD/zapstore.yaml",
		},
		{
			name:  "github URL with extra path segments",
			input: "https://github.com/user/repo/tree/main",
			want:  "https://raw.githubusercontent.com/user/repo/HEAD/zapstore.yaml",
		},
		{
			name:  "github URL with whitespace",
			input: "  https://github.com/user/repo  ",
			want:  "https://raw.githubusercontent.com/user/repo/HEAD/zapstore.yaml",
		},
		{
			name:    "non-github host",
			input:   "https://gitlab.com/user/repo",
			wantErr: true,
		},
		{
			name:    "missing repo path",
			input:   "https://github.com/user",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rawZapstoreYAMLURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (result: %s)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
