# AGENTS.md

vanitykeygen (VKG) is a distributed vanity SSH key generator with a
client/server architecture. A central server manages target regex
patterns and collects matches; one or more clients spin up seeker
goroutines that generate ED25519 keys at high speed and report hits
back. Single Go binary, embedded SQLite (`modernc.org/sqlite`, no CGO),
embedded web dashboard.

If you're here to understand the project, start with the
[README](README.md). Everything below is what you need to contribute
code.

## Build & Test

All workflows go through [just](https://just.systems/). Prefer the
justfile over calling `go` directly — it handles version injection via
`-ldflags`, cross-compilation targets, and ad-hoc codesigning of macOS
binaries.

```bash
just build              # Build for current platform → dist/
just build-all          # linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
just ci                 # Full CI gate: fmt-check + mod-tidy-check + vet + lint + test + build-all
just test               # Tests only
just lint               # golangci-lint
just fmt-check          # gofmt check
just run-server         # Build and run the server locally
just run-client         # Build and run a client locally
```

`just ci` must pass locally before every push. No exceptions. Don't
rely on GitHub Actions to catch what you could have caught locally —
the workflow at `.github/workflows/ci.yml` runs the same `just ci`
target.

## Code Conventions

- **Go 1.25+** required (see `go.mod`). Builds are `CGO_ENABLED=0` —
  do not introduce dependencies that require cgo. SQLite is provided by
  the pure-Go `modernc.org/sqlite` driver.
- **Conventional commits**: `feat:`, `fix:`, `docs:`, `refactor:`,
  `test:`, `chore:`.
- **Prefer the standard library**. Third-party imports add supply chain
  risk, version churn, and transitive deps. The current dep set is
  intentionally tiny (`golang.org/x/crypto`, `modernc.org/sqlite`) —
  discuss the trade-off before adding to it.
- **Context propagation**: `Run` entry points receive a `ctx` from
  `cmd/vkg`. Always pass that `ctx` through to downstream calls
  (HTTP, store, goroutines). Reserve `context.Background()` for true
  top-level roots and short-lived shutdown timeouts (e.g. the graceful
  shutdown context in `pkg/server/server.go`); never use it inside a
  request handler that already has a `ctx`.
- **Error handling**: Handle errors explicitly. No silent swallowing.
  Wrap with context (`fmt.Errorf("doing X: %w", err)`) where it helps
  debugging. Always close HTTP response bodies, and drain them before
  closing if you need keep-alive reuse.
- **Tests**: Table-driven where it fits. Run with `-race` when adding
  concurrent code (the seeker pool, SSE hub, store access).
- **Logging**: Structured via `slog`. INFO = operator story (server
  started, client connected, target activated), DEBUG = deep
  troubleshooting, WARN = degraded, ERROR = broken. Include relevant
  context fields (client ID, target ID, seeker ID) — see
  `seekerStatus.LogValue` in `pkg/client/main.go` for the pattern.
- **Go doc comments**: Every exported symbol gets a doc comment
  starting with its name that reads as a complete sentence. Every
  package gets `// Package foo ...`. Write comments that explain *why*,
  not just *what* — the signature already says what.
- **HTTP API**: The REST surface is documented in the [README](README.md#api).
  When adding endpoints, keep JSON field names `snake_case` and reuse
  existing types in `pkg/vkg` rather than duplicating shapes.
- **Database access**: Goes through `pkg/store`. Keep SQL out of
  handlers — add a method to `Store` and call it from the handler.

## Architecture at a Glance

- **Server** (`pkg/server`): REST API on port 8080, embedded web
  dashboard, SSE hub for live updates, SQLite persistence. Manages
  targets, matches, and the connected client fleet.
- **Client** (`pkg/client`): Registers with the server, polls for the
  active target every 20s, heartbeats every 15s, runs N seeker
  goroutines (default `NumCPU`) that generate ED25519 keypairs and test
  against the target regex.
- **Keygen** (`pkg/keygen`): ED25519 keypair generation and OpenSSH
  fingerprint / authorized_key formatting.
- **Store** (`pkg/store`): SQLite schema, migrations, and queries.
- **Web** (`web/`): Embedded static assets served by the server.

See the diagram and API table in the [README](README.md#architecture).

## Security

- **Private keys** for matches are stored in the server's SQLite DB and
  returned by `GET /api/matches/{id}`. Treat the DB as sensitive — it
  contains usable SSH private keys.
- **No TLS termination** in the binary itself. Run behind a reverse
  proxy (e.g. Caddy, nginx) for production, or expose only on a trusted
  network.
- **Default bind address** is `0.0.0.0` for container compatibility.
  Override with `-b 127.0.0.1` for host-only deployments.
- **HTTP clients** must not disable TLS verification.

## Contributing

### Pull Requests

- **All commits must be signed.** See the per-repo signing setup in
  [CLAUDE.md](CLAUDE.md). PRs with unsigned commits will not merge.
- Run `just ci` locally before pushing.
- Keep PRs focused — one logical change per PR.
- Use conventional commit format for PR titles and commits.
- Reference issues: `Refs #NNN` or `Closes #NNN` in commit bodies.
- **Update docs in the same PR.** If your change affects behavior
  documented in `README.md` (CLI flags, env vars, API surface,
  architecture), update it before requesting review. GoDoc on exported
  symbols counts as documentation.

### Common Review Feedback

- **Context propagation** — Don't use `context.Background()` where a
  `ctx` is already in scope. Goroutines need the parent context so they
  shut down cleanly.
- **Unbounded data** — List endpoints accept `?limit=`. Don't add
  endpoints that return unbounded slices.
- **Race conditions** — Shared state needs mutex guards. The seeker
  pool and SSE hub are the obvious hotspots; add `-race` runs when
  touching them.
- **Silent failures** — If something can fail, log it. DEBUG is fine
  for noise, but silent drops waste debugging time later.

### Review Culture

Leave PRs clean and reflective of reality. Open review threads, stale
descriptions, and unchecked test-plan items signal unfinished work.

When addressing review feedback: fix the issue, reply to the thread
with the commit hash and a one-line explanation, then resolve the
conversation. If deferring, say why before resolving.
