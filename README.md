# Zapstore Server

A Nostr relay and Blossom server for the Zapstore app ecosystem.

## Features

### Nostr Relay
- Full Nostr relay implementation using [rely](https://github.com/pippellia-btc/rely)
- NIP-11 relay information document
- NIP-42 authentication support
- Configurable allowed event kinds with structure validation
- SQLite-based event storage
- Filter specificity scoring to reject overly vague queries

### Blossom Server
- [Blossom](https://github.com/hzrd149/blossom) protocol implementation for blob storage
- [Bunny CDN](https://bunny.net/) integration for scalable blob delivery
- Configurable allowed media types (APKs, images)
- Deduplication: blobs are checked before upload to save bandwidth
- Local SQLite metadata store with CDN redirect for downloads

### Access Control (ACL)
- Hot-reloadable CSV-based allow/block lists for:
  - Pubkeys (allowed and blocked)
  - Event IDs (blocked)
  - Blob hashes (blocked)
- Configurable unknown pubkey policy:
  - `ALLOW` - allow all unknown pubkeys
  - `BLOCK` - block all unknown pubkeys
  - `VERTEX` - use Vertex DVM reputation filtering

### Vertex DVM Integration
- Reputation-based access control for unknown pubkeys
- Supports multiple ranking algorithms:
  - Global PageRank
  - Personalized PageRank
  - Follower count
- Configurable reputation threshold
- In-memory LRU cache for rank lookups

### Rate Limiting
- Token bucket rate limiting per IP group
- Configurable initial tokens, max tokens, and refill rate
- Different costs for different operations (connections, events, queries, uploads)
- Penalty system for misbehaving clients

## Running

### Prerequisites

- Go 1.25 or later
- A BunnyCDN account with a storage zone configured

### Build and Run

```bash
# Clone the repository
git clone https://github.com/zapstore/server.git
cd server

# Create and configure .env file
cp .env.example .env
# Edit .env with your configuration

# Build
go build --tags "fts5" -o zapstore-server ./cmd

# Run
./zapstore-server
```

### Data Directory Structure

On first run, the server creates the following structure:

```
$SYSTEM_DIRECTORY_PATH/
├── data/
│   ├── relay.db      # SQLite database for relay events
│   └── blossom.db    # SQLite database for blob metadata
└── acl/
    ├── pubkeys_allowed.csv
    ├── pubkeys_blocked.csv
    ├── events_blocked.csv
    └── blobs_blocked.csv
```

### ACL File Format

ACL files are CSV with two columns: identifier and reason. Lines starting with `#` are comments.

```csv
# Allowed pubkeys
# pubkey,reason
npub1abc...,Trusted developer
abc123...,Another trusted user
```

Files are hot-reloaded when modified - no server restart required.

### Endpoints

- **Relay**: `ws://localhost:3334` (or your configured port)
- **Blossom**: `http://localhost:3335` (or your configured port)

## License

MIT
