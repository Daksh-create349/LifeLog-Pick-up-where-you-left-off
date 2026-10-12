# Contributing to LIFELOG

Thank you for your interest in contributing to LIFELOG.

LIFELOG is a local-first, privacy-respecting computer memory engine for macOS. We welcome issues, bug reports, and pull requests that adhere to our architectural and privacy requirements.

## Core Architectural Mandates

Before submitting code, ensure your changes follow these non-negotiable principles:

1. **Pure Go / Zero CGO**:
   - The project must compile cleanly with `CGO_ENABLED=0`.
   - SQLite persistence uses `modernc.org/sqlite`. Do not introduce dependencies requiring CGO or native system toolchains.

2. **Privacy by Design**:
   - Never read, persist, or transmit file contents. LIFELOG only collects file metadata (path, size, modification time, event type).
   - Never capture or store terminal stdout, stderr, or shell input streams.
   - Never transmit data over the network. Zero cloud synchronization, zero analytics, zero telemetry.
   - All Git operations must be local and read-only. Never fetch remote URLs or authenticate against remote repositories.
   - All secrets, tokens, passwords, and sensitive arguments must be redacted or omitted before persistence.

3. **Restricted File Permissions**:
   - Storage directories must use `0700` permissions.
   - Storage files (SQLite database, configuration, exported snapshots) must use `0600` permissions.
   - Exports must never overwrite or write into the application data directory (`$LIFELOG_DATA_DIR`).

4. **Schema Migrations**:
   - Schema alterations must be forward-only and registered in `internal/storage/storage.go`.
   - Never rewrite old migrations.
   - New tables or entities must define appropriate FTS5 trigger cleanup or be explicitly documented.

## Development Setup

Prerequisites:
- macOS (Apple Silicon `darwin/arm64` or Intel `darwin/amd64`)
- Go 1.22 or higher (verified on Go 1.27.2)
- Git 2.20+

Build from source:

```bash
git clone https://github.com/Daksh-create349/LifeLog-Pick-up-where-you-left-off.git
cd LifeLog-Pick-up-where-you-left-off
go build -o lifelog .
```

## Running Tests

All pull requests must pass the complete verification suite:

```bash
# Code formatting
gofmt -l .

# Static analysis
go vet ./...

# Unit and integration tests
go test -count=1 ./...

# Race condition detection
go test -count=1 -race ./...

# Pure Go build check
CGO_ENABLED=0 go build -o /tmp/lifelog-check .
```

Use `t.TempDir()` and isolated test directories in all tests. Never write to user home directories or real data locations in test suites.

## Pull Request Guidelines

1. Fork the repository and create your branch from `main`.
2. Keep changes focused and atomic. Avoid unrelated refactoring.
3. Add regression tests for any bug fixes.
4. Ensure documentation in `README.md` and `context.md` reflects any changes to CLI flags or schema.
5. Do not include emojis in documentation or commit messages.
6. Submit your pull request with a descriptive title and summary of changes.
