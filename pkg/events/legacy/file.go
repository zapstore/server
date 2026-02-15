package legacy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

const KindFile = 1063

var SupportedFileMIMEs = []string{
	"application/vnd.android.package-archive",
	"application/zip",
	"application/gzip",
	"application/x-tar",
	"application/x-xz",
	"application/x-bzip2",
	"application/x-executable",
	"application/x-mach-binary",
	"application/x-executable; format=elf; arch=x86-64",
	"application/x-executable; format=elf; arch=arm",
	"application/x-mach-binary; arch=x86-64",
	"application/x-mach-binary; arch=arm64",
}

// File represents a parsed File Metadata event (kind 1063).
// It's a legacy event, superseded by the Software Asset event (kind 3063).
// Learn more here: https://github.com/franzaps/nips/blob/applications/94.md
type File struct {
	// Required fields
	Hash             string   // SHA-256 hash of the file
	URLs             []string // URLs of the asset (may be Blossom URLs)
	MIME             string   // MIME type of the asset
	Version          string   // Version (e.g., "1.2.3", "0.1.2-rc1")
	VersionCode      string   // Version code (Android)
	Platforms        []string // Platform identifiers (at least one required)
	APKSignatureHash string   // APK signing certificate sha256 hash
	MinSDKVersion    string   // Minimum SDK version
	TargetSDKVersion string   // Target SDK version
}

func (f File) Validate() error {
	if f.Hash == "" {
		return fmt.Errorf("missing 'x' tag (SHA-256 hash)")
	}
	if err := ValidateHash(f.Hash); err != nil {
		return fmt.Errorf("invalid SHA-256 hash in 'x' tag: %w", err)
	}

	if len(f.URLs) == 0 {
		return fmt.Errorf("missing 'url' or 'fallback' tags")
	}

	if f.MIME == "" {
		return fmt.Errorf("missing 'm' tag")
	}
	if !slices.Contains(SupportedFileMIMEs, f.MIME) {
		return fmt.Errorf("unsupported MIME type: %s; supported types are %s", f.MIME, strings.Join(SupportedFileMIMEs, ", "))
	}

	if f.Version == "" {
		return fmt.Errorf("missing 'version' tag")
	}

	if len(f.Platforms) == 0 {
		return fmt.Errorf("missing 'f' tags")
	}
	// Platform identifier validation removed for legacy migration compatibility

	if f.MIME == "application/vnd.android.package-archive" {
		if f.VersionCode == "" {
			return fmt.Errorf("missing 'version_code' tag")
		}
		if f.APKSignatureHash == "" {
			return fmt.Errorf("missing 'apk_signature_hash' tag")
		}
		if err := ValidateHash(f.APKSignatureHash); err != nil {
			return fmt.Errorf("invalid APK signing certificate sha256 hash in 'apk_signature_hash' tag: %w", err)
		}
		if f.MinSDKVersion == "" {
			return fmt.Errorf("missing 'min_sdk_version' tag")
		}
		if f.TargetSDKVersion == "" {
			return fmt.Errorf("missing 'target_sdk_version' tag")
		}
	}
	return nil
}

// ParseFile extracts a File from a nostr.Event.
// Returns an error if the event kind is wrong or if duplicate singular tags are found.
func ParseFile(event *nostr.Event) (File, error) {
	if event.Kind != KindFile {
		return File{}, fmt.Errorf("invalid kind: expected %d, got %d", KindFile, event.Kind)
	}

	f := File{}
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "x":
			if f.Hash != "" {
				return File{}, fmt.Errorf("duplicate 'x' tag")
			}
			f.Hash = tag[1]

		case "url", "fallback":
			f.URLs = append(f.URLs, tag[1])

		case "m":
			if f.MIME != "" {
				return File{}, fmt.Errorf("duplicate 'm' tag")
			}
			f.MIME = tag[1]

		case "version":
			if f.Version != "" {
				return File{}, fmt.Errorf("duplicate 'version' tag")
			}
			f.Version = tag[1]

		case "version_code":
			if f.VersionCode != "" {
				return File{}, fmt.Errorf("duplicate 'version_code' tag")
			}
			f.VersionCode = tag[1]

		case "f":
			f.Platforms = append(f.Platforms, tag[1])

		case "apk_signature_hash":
			if f.APKSignatureHash != "" {
				return File{}, fmt.Errorf("duplicate 'apk_signature_hash' tag")
			}
			f.APKSignatureHash = tag[1]

		case "min_sdk_version":
			if f.MinSDKVersion != "" {
				return File{}, fmt.Errorf("duplicate 'min_sdk_version' tag")
			}
			f.MinSDKVersion = tag[1]

		case "target_sdk_version":
			if f.TargetSDKVersion != "" {
				return File{}, fmt.Errorf("duplicate 'target_sdk_version' tag")
			}
			f.TargetSDKVersion = tag[1]
		}
	}
	return f, nil
}

func ValidateFile(event *nostr.Event) error {
	file, err := ParseFile(event)
	if err != nil {
		return err
	}
	return file.Validate()
}
