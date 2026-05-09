# vanitykeygen

Distributed vanity SSH key generator. Search for ED25519 SSH keys whose
fingerprint or authorized key string matches a regex pattern.

A central server manages target patterns and collects matches. One or more
clients generate keys at high speed across multiple goroutines, testing each
against the active target and reporting hits back to the server.

## Quick start

```bash
# Build
just build

# Start the server (creates vkg.db on first run)
just run-server

# In another terminal, connect a client
just run-client
```

Open http://localhost:8080 for the web dashboard.

## Web UI

The embedded web dashboard provides:

- **Dashboard** — live stats (active clients, key rate, total matches), active target, recent matches
- **Targets** — create, edit, delete, and activate regex patterns
- **Matches** — browse all matches, view full key details including the private key
- **Fleet** — monitor connected clients, seeker counts, per-client key rates

Updates stream in real time via Server-Sent Events.

## Architecture

```
┌──────────┐     HTTP/JSON     ┌──────────┐
│  Client   │◄────────────────►│  Server   │
│  (seeker  │  register        │  (REST    │
│  gorout.) │  heartbeat       │   API)    │
│           │  target poll     │           │
│           │  match report    │  SQLite   │
└──────────┘                   │  Web UI   │
                               │  SSE hub  │
┌──────────┐                   └──────────┘
│  Client   │◄────────────────►     │
└──────────┘                        │
                               ┌──────────┐
                               │ Browser   │
                               │ Dashboard │
                               └──────────┘
```

## Server

```
vkg server [options]

  -p int    Listen port (default 8080)
  -b string Bind address (default "" for all)
  -d string SQLite database path (default "vkg.db")
  -t string Default target pattern (seeds DB on first run)
```

Environment variables: `VKG_DB_PATH`, `VKG_TARGET`

### API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/targets` | List targets (`?limit=`) |
| POST | `/api/targets` | Create a target |
| GET | `/api/targets/active` | Get the active compiled patterns |
| GET | `/api/targets/{id}` | Get a target |
| PUT | `/api/targets/{id}` | Update a target |
| DELETE | `/api/targets/{id}` | Delete a target |
| GET | `/api/matches` | List matches (`?target_id=&limit=`) |
| GET | `/api/matches/{id}` | Get a match with full key material |
| POST | `/api/matches` | Submit a match (used by clients) |
| POST | `/api/clients/register` | Register a client |
| POST | `/api/clients/heartbeat` | Client heartbeat |
| GET | `/api/clients` | List connected clients (`?limit=`) |
| GET | `/api/stats` | Aggregate fleet statistics |
| GET | `/api/events` | SSE stream (match, client_update, target_update) |

## Client

```
vkg client [options]

  -s string Server URI (default "https://vkg")
  -n int    Number of seeker goroutines (default: NumCPU)
```

Environment variables: `VKG_SERVER_URI`

The client registers with the server on startup, polls for the active target
pattern every 20 seconds, sends heartbeats every 15 seconds, and reports
matches immediately.

## Container

```bash
# Login to GHCR
just ghcr-login

# Build and push multi-arch image (linux/amd64 + linux/arm64)
just package v1.0.0

# Or attach to a GitHub release
just release v1.0.0
```

```bash
# Run from container
docker run -p 8080:8080 -v vkgdata:/vkgdata ghcr.io/nugget/vanitykeygen
```

## Build recipes

Requires [just](https://github.com/casey/just).

| Recipe | Description |
|--------|-------------|
| `just build` | Build native binary |
| `just build-static` | Build static binary (no CGO) |
| `just test` | Run tests |
| `just vet` | Run go vet |
| `just run-server` | Build and run server |
| `just run-client` | Build and run client |
| `just package [tag]` | Build and push multi-arch container to ghcr.io |
| `just release <tag>` | Package and attach manifest to GitHub release |
| `just clean` | Remove build artifacts |

## License

See [LICENSE](LICENSE) for details.
