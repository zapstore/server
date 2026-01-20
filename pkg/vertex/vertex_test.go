package vertex

import (
	"context"
	"os"
	"testing"
)

const (
	stranger = "34dc3acca6f7720af2948dc34a0aee4506b1366c29e8e88efab37c330e1132af"
	fran     = "726a1e261cc6474674e8285e3951b3bb139be9a773d1acf49dc868db861a1c11"
	pip      = "f683e87035f7ad4f44e0b98cfbd9537e16455a92cd38cefc4cb31db7557f5ef2"
)

// Run this test with:
//
// SECRET_KEY=<your_secret_key> go test
//
// Where <your_secret_key> is a secret key with enough credits for signing the Vertex DVM requests.
func TestFilter_Allow(t *testing.T) {
	tests := []struct {
		name      string
		pubkey    string
		algorithm Algorithm
		want      bool
	}{
		{
			name:      "zero threshold always allows",
			pubkey:    stranger,
			algorithm: Algorithm{Sort: SortGlobal, Threshold: 0.0},
			want:      true,
		},
		{
			name:      "stranger below threshold",
			pubkey:    stranger,
			algorithm: Algorithm{Sort: SortGlobal, Threshold: 0.0000001},
			want:      false,
		},
		{
			name:      "known pubkey above threshold",
			pubkey:    fran,
			algorithm: Algorithm{Sort: SortGlobal, Threshold: 0.0000001},
			want:      true,
		},
		{
			name:      "known pubkey above threshold (followers count)",
			pubkey:    fran,
			algorithm: Algorithm{Sort: SortFollowers, Threshold: 1000},
			want:      true,
		},
		{
			name:      "known pubkey above threshold (personalized)",
			pubkey:    fran,
			algorithm: Algorithm{Sort: SortPersonalized, Threshold: 0.0000001, Source: pip},
			want:      true,
		},
	}

	config := NewConfig()
	config.SecretKey = os.Getenv("SECRET_KEY")
	if config.SecretKey == "" {
		t.Fatalf("SECRET_KEY environment variable is not set")
	}

	filter, err := NewFilter(config)
	if err != nil {
		t.Fatalf("failed to create a new filter: %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter.config.Algorithm = test.algorithm
			filter.cache.Purge() // clear the cache

			allow, err := filter.Allow(context.Background(), test.pubkey)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if allow != test.want {
				t.Errorf("expected %v, got %v", test.want, allow)
			}
		})
	}
}
