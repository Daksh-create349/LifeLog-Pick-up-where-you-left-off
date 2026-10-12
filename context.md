# LIFELOG — Context and State of Truth

## 1. Product Vision and Scope

LIFELOG is an open-source, local-first personal computer memory system built specifically for macOS. The system allows users to reconstruct and navigate their computing history across projects and time.

Future capabilities will encompass:
- Project activity observation and resumption context.
- Local filesystem change monitoring (pure-Go recursive snapshot polling via `fs_poll`; native FSEvents omitted to preserve pure-Go / zero-CGO requirement).
- Git repository tracking and branch/commit chronological context.
- Terminal session and command execution indexing.
- Local semantic and full-text search without cloud dependence.

Prompt 1 established the Go project foundation and CLI runner.
Prompt 2 established local configuration management and persistent SQLite storage.
Prompt 3 established the first real collector: the Filesystem Memory Engine, recording file creation, modification, and deletion events in SQLite.
Prompt 4 established the Git Repository Memory Engine, recording local repository discovery, branch tracking, commit history, working-tree changes, and observation timelines in SQLite.
Prompt 5 established the Privacy-First Terminal Memory Engine, recording explicitly executed commands, working directories, execution duration, and exit statuses with conservative secret redaction and zero stdout/stderr capture in SQLite.
Prompt 6 established the Unified Search and Context Query Engine, implementing full-text SQLite FTS5 search across filesystem events, Git history, repository observations, and terminal executions with real-time trigger synchronization, explainable ranking, safe parameterization, and chronological timeline aggregation.

---

## 2. Current Architecture and File Structure

```
/Users/dakshsrivastava/Desktop/LifeLog
├── .gitignore                    # Excludes build binaries, OS metadata, and test artifacts
├── LICENSE                       # MIT License
├── CONTRIBUTING.md               # Contribution guidelines and architectural mandates
├── CODE_OF_CONDUCT.md            # Contributor Covenant Code of Conduct
├── README.md                     # Comprehensive project documentation
├── context.md                    # Continuous project state of truth
├── go.mod                        # Go module declaration (github.com/lifelog/lifelog, Go 1.27.2)
├── go.sum                        # Checksums for pure-Go SQLite driver
├── main.go                       # Thin executable entry point
├── main_test.go                  # Main package integration tests and binary acceptance suite
├── docs/
│   └── images/
│       └── watcher-demo.png      # Filesystem watcher screenshot
└── internal/
    ├── cli/
    │   ├── checkpoint.go         # Checkpoint CLI commands (create, list, show, delete)
    │   ├── cli.go                # CLI argument parsing, flags, and exit code handler
    │   ├── cli_test.go           # Unit test suite for CLI behaviors and exit codes
    │   ├── doctor.go             # Read-only system diagnostics command
    │   ├── events.go             # Query command for stored filesystem events
    │   ├── export.go             # Project context export command (markdown/json)
    │   ├── git.go                # Git commands: git scan, git status, git log, git branches, repos
    │   ├── init.go               # Idempotent storage initialization command
    │   ├── package.go            # Project resumption packaging command
    │   ├── projects.go           # Multi-project overview command
    │   ├── resume.go             # Project resumption report command
    │   ├── search.go             # Search and timeline commands: lifelog search, lifelog timeline
    │   ├── status.go             # Storage and application health report command
    │   ├── terminal.go           # Terminal commands: terminal record, history, inspect, delete, clear
    │   ├── version.go            # Application version and runtime platform metadata
    │   └── watch.go              # Directory monitoring command with graceful signal shutdown
    ├── collector/
    │   ├── collector.go          # Pluggable Collector interface and error definitions
    │   ├── poller.go             # Pure-Go recursive snapshot directory poller (fs_poll)
    │   ├── poller_benchmark_test.go # Directory scan benchmark (510 items, ReportAllocs)
    │   ├── poller_reliability_test.go # Transient error, Unicode, symlink loop, rename tests
    │   └── poller_test.go        # Lifecycle, symlink safety, and data-dir exclusion unit tests
    ├── config/
    │   ├── config.go             # Typed configuration, validation, atomic save (tempfile + rename)
    │   └── config_test.go        # Config loading, default generation, and validation tests
    ├── events/
    │   ├── event.go              # Filesystem event data model and validation
    │   └── event_test.go         # Unit tests for event validation
    ├── git/
    │   ├── client.go             # Safe Git observer (rev-parse, status -z, log, branch tracking)
    │   ├── client_test.go        # Git integration tests (spaces, Unicode, empty repos, worktrees, dirty trees)
    │   ├── git_reliability_test.go # Detached HEAD, empty repo, worktree tests
    │   └── types.go              # Domain models (RepoState, Commit, BranchInfo, FileChange)
    ├── paths/
    │   ├── paths.go              # Application Support path resolution and LIFELOG_DATA_DIR override
    │   └── paths_test.go         # Path resolution, canonicalization, and directory permission tests
    ├── resumption/
    │   ├── checkpoint.go         # Checkpoint record formatting and schema validation
    │   ├── checkpoint_reliability_test.go # Malformed payload and unsupported version tests
    │   ├── checkpoint_test.go    # Checkpoint serialization unit tests
    │   ├── export.go             # Context export formatting and safe atomic file writer
    │   ├── export_test.go        # Export formatting, atomic writing, and permissions tests
    │   ├── package.go            # Portable bundle packaging and validation
    │   ├── package_test.go       # Packaging tests and JSON round-trip tests
    │   ├── renderer.go           # Text report formatting for resumption context
    │   ├── session.go            # Session clustering algorithm and inactivity gap grouping
    │   ├── session_test.go       # Session synthesis and clustering unit tests
    │   ├── synthesis.go          # Session synthesis, inactivity clustering, evidence inference
    │   ├── synthesis_test.go     # Session clustering and synthesis test suite
    │   └── types.go              # Resumption, Session, and Package domain models
    ├── search/
    │   ├── sanitizer.go          # FTS query tokenizer, escaper, and explainable relevance scoring
    │   ├── sanitizer_test.go     # Unit tests for query parsing, unicode, and scoring
    │   └── types.go              # Domain models (Domain, Query, ResultItem, SearchResult)
    ├── storage/
    │   ├── checkpoint_storage.go # Checkpoint CRUD and queries
    │   ├── git_storage.go        # Transactional repository, commit, branch, and snapshot persistence
    │   ├── integrity_test.go     # FTS triggers, wildcard escaping, sibling isolation, concurrent writes
    │   ├── migration_acceptance_test.go # Forward migration v5/v6->v7, triggers, persistence, rollback
    │   ├── project_storage.go    # Multi-project discovery across all domain tables
    │   ├── project_storage_test.go # Unit tests for project discovery and path deduplication
    │   ├── resumption_storage.go # Resumption report queries and filesystem summary
    │   ├── search_storage.go     # FTS5 search execution, LIKE fallback, and index rebuild
    │   ├── storage.go            # SQLite manager, WAL mode, migrations v1-v7, busy timeout
    │   ├── storage_benchmark_test.go # Search FTS and project discovery benchmarks
    │   ├── storage_test.go       # Migration, persistence round-trip, query, and deduplication tests
    │   └── terminal_storage.go   # Transactional terminal execution record persistence and management
    └── terminal/
        ├── executor.go           # Subprocess execution, output passthrough, and signal handling
        ├── executor_test.go      # Integration tests for command execution, exit codes, and cancellation
        ├── sanitizer.go          # Secret redactor, flag maskers, token heuristics, and tool omittance
        ├── sanitizer_test.go     # Comprehensive unit tests for sensitive tools and flag redaction
        └── types.go              # Domain models (Record, RecordFilter, SanitizedResult)
```

---

## 3. Git Observer Strategy & Safety Design

### Principles
1. **Installed Git via `os/exec.CommandContext`**:
   - Uses system Git executable; reports actionable errors if missing or unusable.
   - Arguments are always passed as string slices; shell execution strings (`sh -c`) are strictly forbidden.
   - Bounded execution timeouts (5s to 15s) and bounded outputs prevent runaway processes.
2. **Canonical Root Resolution & Worktree Support**:
   - Discovers repository roots via `git rev-parse --show-toplevel`.
   - Worktrees with `.git` pointer files are recognized seamlessly.
   - Canonicalizes filesystem symlinks via `filepath.EvalSymlinks` (handling `/var` -> `/private/var` on macOS).
   - Rejects the root filesystem `/`, user's home directory `$HOME`, and `$LIFELOG_DATA_DIR`.
   - Prevents recursive filesystem crawling; only user-targeted paths are registered.
3. **Strictly Read-Only Operations**:
   - Commands executed: `git --version`, `git rev-parse`, `git status --porcelain -z -uall`, `git log`, `git for-each-ref`.
   - Never runs `checkout`, `commit`, `reset`, `pull`, `push`, or `fetch`.
4. **Zero Network & Privacy by Design**:
   - Never inspects, extracts, or persists remote URLs (preventing private repo leakage or credential exposure).
   - Never reads credentials, SSH keys, or source-file contents.
   - Zero network access.

---

## 4. Privacy-First Terminal Memory Strategy

### Core Principles
1. **Explicit Opt-in Only**:
   - Users deliberately invoke `lifelog terminal record [--shell] [--] <command> [args...]`.
   - No implicit shell interception, no modification of shell initialization files (`~/.zshrc`, `~/.bashrc`), and no reading of shell history files (`.zsh_history`, `.bash_history`).
   - No continuous background daemon.
2. **Subprocess Execution & Output Passthrough**:
   - Direct execution via `os/exec` with argument arrays by default, avoiding shell parsing hazards.
   - Shell invocation (`/bin/sh -c`) is an explicit, clearly labeled opt-in mode (`--shell`).
   - Preserves user experience by streaming stdout, stderr, and stdin directly to the active terminal.
   - **Zero Output / Input Collection**: Command output (stdout/stderr) and interactive input are **never captured, indexed, or stored**.
   - Preserves child exit status faithfully; returns child exit codes (e.g. 0, 1, 42) and signals (130 for SIGINT).
3. **Conservative Secret Redaction Engine**:
   - **Sensitive Flags**: Redacts `--password`, `--passwd`, `--token`, `--api-key`, `--secret`, `--auth`, `--credentials`, database `-p` flags in `--flag=VAL` and `--flag VAL` forms.
   - **Assignment Arguments**: Redacts `KEY=VALUE` when key name contains `PASS`, `TOKEN`, `SECRET`, `KEY`, `AUTH`, `CRED`.
   - **HTTP Headers**: Redacts values in `Authorization:`, `X-Api-Key:`, and token headers.
   - **Embedded Credentials in URLs**: Masks `http(s)://user:secret@host` to `http(s)://user:[REDACTED]@host`.
   - **Pattern Heuristics**: Redacts GitHub tokens (`ghp_...`), AWS keys (`AKIA...`), Slack tokens (`xoxb-...`), OpenAI keys (`sk-...`), and private keys.
   - **Credential Manager Omission**: If command involves sensitive credential tools (`pass`, `security`, `op`, `vault`, `gpg`, `ssh-add`, `docker login`, `gh auth`, `aws configure`), command details are completely omitted (`[DETAILS OMITTED: sensitive authentication or credential tool]`).
   - **No Unredacted Fallback**: LIFELOG never falls back to persisting raw unredacted command lines.
   - **No Environment Scraping**: Arbitrary process environments are never stored.

---

## 5. Unified Search Architecture & Ranking Design

### Core Principles
1. **SQLite FTS5 Native Full-Text Search**:
   - Compiles and runs out-of-the-box via `modernc.org/sqlite` with zero CGO dependencies.
   - Virtual table `search_index` indexes `title`, `content`, and `working_path` with `UNINDEXED` columns for `domain`, `record_id`, and `timestamp`.
2. **Real-Time Trigger Synchronization**:
   - Triggers on `filesystem_events`, `git_commits`, `git_branches`, `git_branch_log`, `git_repositories`, and `terminal_records` ensure `search_index` is updated atomically on inserts and deletes.
   - When a terminal record or Git repository is deleted, the corresponding `search_index` entries are purged immediately, preventing stale search hits.
3. **Idempotent Index Rebuild**:
   - `db.RebuildSearchIndex(ctx)` purges and safely repopulates `search_index` from all persistent source tables.
4. **Parameterized Fallback**:
   - Query sanitization quotes and escapes tokens for FTS (`BuildFTSQuery`).
   - If malformed syntax or wildcards cause an FTS parser error, the engine seamlessly falls back to parameterized `LIKE` matching without failing the command.
5. **Explainable Relevance Scoring**:
   - Exact title/subject match bonus: +100.0.
   - Title contains full query bonus: +50.0.
   - Working path contains full query bonus: +35.0 / +20.0.
   - Content contains full query bonus: +15.0.
   - BM25 rank score scaled from FTS rank.
   - Recency tie-breaker: newer events rank higher when scores are identical.
6. **Activity Timeline**:
   - Merges stored events across filesystem, git, and terminal in chronological order via `lifelog timeline`.

---

## 6. Database Schema and Migrations

SQLite Schema Version: **7**

### Migration 1: `initial_metadata_table`
- Table `schema_migrations` (`version INTEGER PRIMARY KEY`, `name TEXT`, `applied_at TIMESTAMP`).
- Table `metadata` (`key TEXT PRIMARY KEY`, `value TEXT`, `updated_at TIMESTAMP`).

### Migration 2: `filesystem_events_table`
- Table `filesystem_events` (`id TEXT PRIMARY KEY`, `event_type TEXT`, `path TEXT`, `size INTEGER`, `mod_time TIMESTAMP`, `is_dir BOOLEAN`, `source TEXT`, `created_at TIMESTAMP`).

### Migration 3: `git_repository_memory`
- Table `git_repositories` (`id TEXT PRIMARY KEY`, `path TEXT NOT NULL UNIQUE`, `is_bare BOOLEAN`, `created_at`, `updated_at`).
- Table `git_commits` (`repository_id`, `commit_hash`, `author_name`, `author_time`, `subject`, `discovered_at`).
- Table `git_snapshots` (`id TEXT PRIMARY KEY`, `repository_id`, `branch`, `is_detached`, `head_commit_hash`, `observed_at`).
- Table `git_working_tree_changes` (`snapshot_id`, `path`, `status`).
- Table `git_branches` (`repository_id`, `name`, `commit_hash`, `observed_at`).
- Table `git_branch_log` (`id TEXT PRIMARY KEY`, `repository_id`, `branch_name`, `old_commit_hash`, `new_commit_hash`, `observed_at`).

### Migration 4: `terminal_records_table`
- Table `terminal_records` (`id TEXT PRIMARY KEY`, `working_dir TEXT`, `executable TEXT`, `sanitized_args TEXT`, `sanitized_command TEXT`, `start_time TIMESTAMP`, `end_time TIMESTAMP`, `duration_ms INTEGER`, `exit_code INTEGER`, `collection_mode TEXT`, `is_redacted BOOLEAN`, `is_omitted BOOLEAN`, `omission_reason TEXT`, `created_at TIMESTAMP`).

### Migration 5: `search_index_fts5`
```sql
CREATE VIRTUAL TABLE IF NOT EXISTS search_index USING fts5(
    domain UNINDEXED,
    record_id UNINDEXED,
    title,
    content,
    working_path,
    timestamp UNINDEXED
);

-- Populates existing records from filesystem_events, git_commits, git_branches, git_branch_log, git_repositories, terminal_records
-- Creates synchronization triggers (insert/delete) keeping search_index in sync automatically.
```

### Migration 6: `checkpoints_table`
- Table `checkpoints` (`id TEXT PRIMARY KEY`, `project_path TEXT NOT NULL`, `label TEXT NOT NULL`, `created_at TIMESTAMP NOT NULL`, `format_version TEXT NOT NULL DEFAULT '1.0.0'`, `payload TEXT NOT NULL`).
- Index `idx_checkpoints_project_created` on `(project_path, created_at)`.
- Deliberately excluded from FTS5 `search_index` to prevent search noise.

### Migration 7: `fix_search_delete_triggers`
- Fixes missing deletion triggers for Git entities in FTS5 `search_index`:
  - `trg_git_commits_delete`: Removes commit from `search_index` on `git_commits` deletion.
  - `trg_git_branches_delete`: Removes branch from `search_index` on `git_branches` deletion.
  - `trg_git_branch_log_delete`: Removes transition log entry on `git_branch_log` deletion.
  - `trg_git_repos_cascade_delete`: Cascades repository purge across `search_index WHERE domain = 'git' AND working_path = old.path`.

---

## 7. Commands Executed and Verification Results

1. `gofmt -l .`
   - Result: 0 formatting differences.
2. `go vet ./...`
   - Result: Passed with 0 issues.
3. `go test -v -cover ./...`
   - Result: Passed across all packages:
     - `internal/search`: 80.9% coverage
     - `internal/storage`: 72.5% coverage
     - `internal/cli`: 67.7% coverage
     - `internal/terminal`: 89.7% coverage
     - `internal/git`: 82.2% coverage
     - `internal/collector`: 82.4% coverage
     - `internal/events`: 100% coverage
     - `internal/config`: 72.9% coverage
     - `internal/paths`: 64.3% coverage
4. `go test -race ./...`
   - Result: Clean pass across all packages with zero race conditions.
5. `go build -o lifelog .`
   - Result: Binary compiled cleanly.
6. End-to-End CLI Verification:
   - Initialized storage with schema v5.
   - Recorded filesystem events (`watch`).
   - Scanned Git repo with commits and branches (`git scan`).
   - Recorded terminal commands and sensitive tools (`terminal record`).
   - Queried unique domain searches: `search --domain filesystem`, `search --domain git`, `search --domain terminal`.
   - Executed cross-domain search (`search "payment"`).
   - Tested limits and post-query flags (`search "payment" --limit 1`).
   - Tested unified chronological timeline (`timeline --limit 5`).
   - Reopened CLI with fresh process context; verified persistent search hits.
   - Tested deletion synchronization (`terminal delete <id>` -> removed from search index immediately).
   - Verified earlier commands (`status`, `doctor`, `repos`, `events`) continue functioning without regression.
7. `git diff --check`
   - Result: Clean.

---

## 8. Prompt 7: Resumption Context & Activity Session Synthesis Engine (Completed)

### 8.1 Architectural Implementation
- **New Package `internal/resumption`**:
  - `types.go`: Typed models (`ActivityItem`, `Session`, `PathChange`, `ObservedFilesystem`, `ObservedGit`, `ObservedTerminal`, `ResumptionReport`, `Options`).
  - `session.go`: Deterministic session clustering with inactivity gap threshold (`DefaultInactivityGap = 30m`), out-of-order handling, duplicate activity deduplication, duration computation, and concise factual summaries.
  - `synthesis.go`: Multi-domain context synthesis engine (`BuildResumptionReport`) querying filesystem, Git, and terminal memories with project path attribution, latest activity tracking, evidence-backed activity inference (*"Recent recorded activity suggests..."*), and transparent unavailable notices.
  - `renderer.go`: Formats report into three clear, standardized sections: `1. OBSERVED ACTIVITY`, `2. DERIVED ACTIVITY SESSIONS & SYNTHESIS`, `3. UNAVAILABLE CONTEXT & PRIVACY BOUNDARIES`.
- **Storage Layer Enhancements (`internal/storage/resumption_storage.go`)**:
  - `GetProjectFilesystemSummary`: Aggregates total event counts, type-grouped counts, and recent bounded events for a project path.
  - `FindGitRepoForPath`: Identifies registered Git repository covering target path with dual clean/eval symlink support and longest-path matching for nested repositories.
  - `ListTerminalRecordsForProject`: Queries explicitly recorded commands executed in or under the project directory.
  - `QueryGitCommitsSince` and `QueryGitBranchLogSince`: Retrieves repository commits and branch tip movements with optional time filtering.
- **Path Canonicalization & macOS Symlink Parity (`internal/paths`)**:
  - `paths.Canonical`: Resolves absolute, cleaned paths with symlink evaluation fallback.
  - `paths.IsSubpath`: Robust prefix boundary checking to prevent false matches against sibling directories (e.g., `/project-other` vs `/project`).
  - Dual matching against `pathClean` and `pathEval` across all SQLite queries ensures full compatibility on macOS where `/var` symlinks to `/private/var`.
- **CLI Subcommand `lifelog resume` (`internal/cli/resume.go`, `internal/cli/cli.go`)**:
  - Syntax: `lifelog resume <project-path> [--limit N] [--window 7d] [--since RFC3339] [--gap 30m]`
  - Supports flexible argument ordering, comprehensive parameter validation, and helpful empty-state feedback.

### 8.2 Database and Schema
- No new database migrations or schema alterations were needed. All activity sessions and resumption reports are computed on-the-fly from persisted SQLite records without requiring session tables (current schema v7).

### 8.3 Privacy and Safety Boundaries
- **No File Content or Diff Inspection**: File contents, source diffs, and ASTs are never read or stored.
- **No Terminal Output Scraping**: stdout/stderr are never collected.
- **Privacy Omission Respect**: Inherits `[DETAILS OMITTED]` for sensitive tools and `[REDACTED]` for sensitive flags/passwords.
- **Strict Project Attribution**: Unrelated activities are excluded; nested repositories are attributed to the closest matching root.

---

## 9. Verification & Test Suite Results (Prompt 7)

1. `gofmt -l .`
   - Result: 0 formatting differences.
2. `go vet ./...`
   - Result: Passed with 0 issues.
3. `go test -v ./...`
   - Result: Passed across all packages (11/11).
4. `go test -race ./...`
   - Result: Clean pass across all packages with zero race conditions.
5. `go build -o lifelog .`
   - Result: Binary compiled cleanly.
6. End-to-End CLI Verification:
   - Initialized storage with schema v5.
   - Initialized Git repo, committed changes, created working-tree modifications.
   - Scanned Git repo (`git scan`).
   - Recorded terminal commands, including sensitive tool omission (`terminal record`).
   - Executed `lifelog resume <project-path>`.
   - Verified formatted output with all three sections (`Observed`, `Derived`, `Unavailable`).
   - Verified activity sessions (`session-001`), inference text, and privacy guarantees.
   - Tested flags: `--limit`, `--gap`, `--window`.
   - Verified clean empty state and path isolation on unrelated project directory.
7. `git diff --check`
   - Result: Clean.

---

## 10. Prompt 8: Project Context Export & Explicit Resume Checkpoints (Completed)

### 10.1 Architectural Implementation
- **Project Context Export (`internal/resumption/export.go`, `internal/cli/export.go`)**:
  - `ExportEnvelope`: Standardized envelope (`SchemaVersion: "1.0.0"`, `ExportedAt`, `Generator: "LIFELOG"`, `Report`).
  - `FormatJSON`: Indented, machine-readable JSON output that is round-trip testable.
  - `FormatMarkdown`: Human-readable Markdown export organizing information into distinct `Observed`, `Derived`, and `Unavailable` sections.
  - `SafeWriteExportFile`: Atomic write implementation using a temporary file in the destination directory and atomic rename. Rejects writes directed into the protected LIFELOG data directory (`dataDir`), enforces overwrite protection unless `--force` is specified, and creates files with `0600` permissions.
  - `lifelog export <path> [--format markdown|json] [--output <path>] [--force] [--limit N] [--window 7d] [--gap 30m]`: Exports to stdout or safe file destination.
- **Explicit Resume Checkpoints (`internal/storage/checkpoint_storage.go`, `internal/resumption/checkpoint.go`, `internal/cli/checkpoint.go`)**:
  - Schema Migration 6: Forward-only migration creating `checkpoints` table (`id`, `project_path`, `label`, `created_at`, `format_version`, `payload`) with an index on `(project_path, created_at)`. Kept excluded from FTS5 index to prevent search index pollution.
  - `CreateCheckpoint`: Validates non-empty label (<= 256 chars), captures immutable resumption state via `BuildResumptionReport`, generates unique sortable ID (`chk_<nano>_<rand>`), and persists to SQLite.
  - `RenderCheckpoint`: Unmarshals saved historical payload directly to render text, Markdown, or JSON. Strictly preserves historical snapshot immutability; viewing an old checkpoint never recalculates from current live data.
  - `ListCheckpoints`: Retrieves checkpoints in reverse-chronological order, with path canonicalization and macOS `/var` -> `/private/var` symlink parity.
  - `DeleteCheckpoint`: Removes checkpoint by ID, requiring explicit `--confirm` flag to avoid accidental deletion.
  - CLI Subcommands:
    - `lifelog checkpoint create <path> --label "..."`
    - `lifelog checkpoint list [<path>]`
    - `lifelog checkpoint show <id> [--format text|markdown|json]`
    - `lifelog checkpoint delete <id> --confirm`

### 10.2 Database and Schema
- Database schema upgraded to **version 6** via migration `checkpoints_table`.
- Full backward compatibility maintained for existing records and databases created under schemas v1–v5.

### 10.3 Privacy and Safety Boundaries
- Checkpoints are created strictly upon deliberate user invocation. No implicit background checkpoints are created.
- Export files and checkpoints inherit all privacy guarantees: zero file contents, zero Git diffs, zero shell stdout/stderr, and preserved `[DETAILS OMITTED]` / `[REDACTED]` tokens.
- Protected application storage (`lifelog.db`, `config.json`) cannot be targeted for export file output.

---

## 11. Verification & Test Suite Results (Prompt 8)

1. `gofmt -l .`
   - Result: 0 formatting differences.
2. `go vet ./...`
   - Result: Passed with 0 issues.
3. `go test -v ./...`
   - Result: Passed across all packages (11/11).
4. `go test -race ./...`
   - Result: Clean pass across all packages with zero race conditions.
5. `go build -o lifelog .`
   - Result: Binary compiled cleanly.
6. End-to-End CLI Verification:
   - Initialized storage with schema v6.
   - Initialized Git repository, committed changes, recorded terminal execution.
   - Exported resumption context to Markdown file; verified file permissions (`0600`) and content structure.
   - Exported resumption context to JSON; verified envelope schema.
   - Created named checkpoint (`checkpoint create ... --label "..."`); verified ID generation.
   - Added post-checkpoint terminal execution; verified checkpoint immutability (`checkpoint show` did not mutate).
   - Listed checkpoints (`checkpoint list`).
   - Deleted checkpoint with `--confirm` (`checkpoint delete ... --confirm`).
   - Confirmed earlier commands (`status`, `doctor`, `search`, `timeline`, `resume`) remain intact.
7. `git diff --check`
   - Result: Clean.

---

## 12. Project Context Packaging & Multi-Project Discovery

### 12.1 Project Context Packaging (`package`)
- **Package Model (`internal/resumption/package.go`)**:
  - Encapsulates resumption context into a standardized portable container (`package_version: "1.0.0"`, `generator: "LIFELOG"`, `app_version: "0.1.0"`).
  - Explicit domain accounting: separates `included_domains` (domains with recorded evidence) from `empty_domains` (domains with no observations).
  - Display name derived from `filepath.Base(canonical_path)` with defensive fallback for root paths.
  - Serialization formats:
    - **JSON** (`FormatPackageJSON`): Deterministic indented JSON, round-trip testable.
    - **Markdown** (`FormatPackageMarkdown`): Human-readable Markdown cleanly separating Observed Facts, Derived Synthesis, and Unavailable Context / Privacy Boundaries.
  - Command: `lifelog package <project-path> [--format json|markdown] [--output <path>] [--force] [--limit N] [--window 7d] [--gap 30m]`

### 12.2 Multi-Project Overview (`projects`)
- **Dynamic Discovery (`internal/storage/project_storage.go`)**:
  - Dynamically discovers all projects known to LIFELOG without requiring a separate table or filesystem crawling.
  - Aggregates evidence across:
    1. `git_repositories`: Explicitly registered Git repos (`path`, `created_at`, `updated_at`).
    2. `checkpoints`: Explicitly saved project checkpoints (`project_path`).
    3. `terminal_records`: Recorded command working directories (`working_dir`).
    4. `filesystem_events`: Monitored file modification paths (`path`, `is_dir`).
- **Attribution & Safety Rules**:
  - Non-project roots (filesystem root `/`, home directory `~`, and LIFELOG data directory) are defensively excluded.
  - Canonical path deduplication resolves macOS `/var` vs `/private/var` symlinks.
  - Nested Git repositories are isolated by longest canonical prefix matching (`len(path) DESC`), ensuring nested repositories are never confused with parents.
  - Sibling directories (e.g. `/workspace/proj_1` vs `/workspace/proj_10`) are isolated via exact boundary matching (`WHERE (path = ? OR path LIKE ? || '/%')`).
  - Metrics include: canonical path, display name, existence on disk (`os.Stat`), Git status, active branch, checkpoint count, activity counts per domain, and un-fabricated timestamps (`nil` when unavailable).
  - Sorting options: `recent` (default: newest activity first, unknown timestamps last), `name` (alphabetical by display name), `activity` (descending total event count).
  - Command: `lifelog projects [--sort recent|name|activity] [--limit N] [--window 7d] [--json]`

---

## 13. Authoritative Database Schema & Migration Registry (Schema Version 7)

The database schema is strictly managed by forward-only migrations registered in `internal/storage/storage.go`. The current authoritative schema version is **Version 7**:

| Version | Migration Name | Key Tables / Triggers Created | Purpose |
| :---: | :--- | :--- | :--- |
| **1** | `initial_metadata_table` | `metadata` | Key-value store for initialization timestamp and database state. |
| **2** | `filesystem_events_table` | `filesystem_events` | Stores filesystem modification events with path and created_at indexes. |
| **3** | `git_memory_tables` | `git_repositories`, `git_commits`, `git_snapshots`, `git_working_tree_changes`, `git_branches`, `git_branch_log` | Stores Git repositories, commit history, branch logs, and working-tree snapshots with foreign-key cascade constraints. |
| **4** | `terminal_records_table` | `terminal_records` | Stores explicitly recorded terminal commands with sanitized metadata, duration, and exit codes. |
| **5** | `search_index_fts5` | `search_index` (FTS5), insertion triggers | Cross-domain full-text search index and real-time triggers on base tables. |
| **6** | `checkpoints_table` | `checkpoints` | Immutable project resumption snapshots with project_path and created_at indexes. |
| **7** | `fix_search_delete_triggers` | `trg_git_commits_delete`, `trg_git_branches_delete`, `trg_git_branch_log_delete`, `trg_git_repos_cascade_delete` | Real-time cleanup triggers on `search_index` when Git commits, branches, or repositories are deleted. |

### 13.1 SQLite Concurrency & Locking Configuration
Database connection string configured in `storage.Open()`:
```go
dsn := fmt.Sprintf("%s?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL&_txlock=immediate", dbPath)
```
- **WAL Mode**: Enables concurrent readers alongside a writer.
- **Immediate Locking (`_txlock=immediate`)**: Every transaction starts with `BEGIN IMMEDIATE` to prevent deferred lock-upgrade deadlocks under concurrent write workloads.
- **Busy Timeout (`_busy_timeout=5000`)**: Serializes concurrent writer transactions, waiting up to 5,000ms before returning `SQLITE_BUSY`.
- **Foreign Keys (`_foreign_keys=on`)**: Enforces referential integrity and cascading deletes across child tables.

### 13.2 Trigger Cascade Safety
In SQLite, foreign-key cascade actions delete child rows before/during parent row deletion and do not fire child triggers unless recursive triggers are enabled.
Trigger `trg_git_repos_cascade_delete` operates directly on `search_index`:
```sql
CREATE TRIGGER IF NOT EXISTS trg_git_repos_cascade_delete AFTER DELETE ON git_repositories BEGIN
    DELETE FROM search_index WHERE domain = 'git' AND working_path = old.path;
END;
```
Because `search_index` stores `working_path = r.path` for all repository records, all associated commits, branches, and logs are purged without relying on child-table lookups during cascade execution.

---

## 14. Privacy Model, Secret Redaction & Security Boundaries

### 14.1 Verified Privacy Guarantees
- **Zero File Content Collection**: Filesystem observation inspects directory structure and metadata (`os.Stat`, `d.Info()`); file contents are never read or indexed.
- **Zero Subprocess Output Capture**: Terminal executor connects subprocess standard streams directly to user terminal streams (`os.Stdout`, `os.Stderr`, `os.Stdin`); stdout/stderr are never intercepted, recorded, or stored in SQLite.
- **Zero Remote Networking**: Git observer uses strictly local commands (`rev-parse`, `symbolic-ref`, `log`, `for-each-ref`, `status --porcelain=v1 -z`). Never contacts remote servers or stores remote URLs/credentials.
- **Secret Redaction Pipeline (`internal/terminal/sanitizer.go`)**:
  - Flags: Sensitive flags in `--flag=value` and `--flag value` forms (`--password`, `--token`, `--api-key`, `--secret`, `--auth`, `--credentials`, `-p`) have values masked with `[REDACTED]`.
  - Assignments: Inline assignments such as `DATABASE_PASSWORD=xxx` have values masked with `[REDACTED]`.
  - Headers: HTTP headers (`Authorization: Bearer xxx`, `X-Api-Key: xxx`) are masked.
  - URLs: Credentials inside URLs (`https://user:pass@host`) are sanitized to `https://user:[REDACTED]@host`.
  - Known Token Patterns: Masks GitHub tokens (`ghp_`), AWS keys (`AKIA`), Slack tokens (`xoxb-`), and OpenAI keys (`sk-`).
  - Sensitive Tools: Tools whose operation is inherently sensitive (`pass`, `security`, `op`, `vault`, `gpg`, `ssh-add`, `docker login`, `gh auth`) have all arguments omitted: `[DETAILS OMITTED: sensitive authentication or credential tool]`.
  - Verification: Direct queries on raw SQLite tables (`terminal_records`, `search_index`) confirm synthetic secrets are 100% absent from storage.

### 14.2 Known Heuristic Limitations
- Heuristic regex redaction identifies standard flag conventions, assignment patterns, and known token prefixes. Arbitrary secrets passed as untagged positional arguments to custom programs cannot be detected by heuristics alone.

---

## 15. File Safety, Atomic Writing & Path Protection

- **Atomic Writes (`SafeWriteExportFile`)**: File exports and packages write first to a hidden temporary file (`.<name>.tmp-*`) in the destination directory, set restrictive `0600` permissions, sync buffers to disk (`tempFile.Sync()`), and atomically rename (`os.Rename`) over the destination.
- **Automatic Cleanup**: Temporary files are deleted on any write or rename failure.
- **Overwrite Protection**: Destination files are never overwritten silently; requires explicit `--force`.
- **Protected Storage Exclusion**: Writing into internal LIFELOG storage (`~/Library/Application Support/LifeLog` or `$LIFELOG_DATA_DIR`) is rejected, including via symlink evaluation on destination path and parent directories (`filepath.EvalSymlinks`).
- **TOCTOU Note**: On POSIX filesystems without `renameat2(RENAME_NOREPLACE)`, a minor race condition exists if another local process mutates symlink targets in the parent directory between path canonicalization and atomic rename.

---

## 16. Performance Benchmarks (Audited on Apple M4, macOS arm64)

Benchmarks measured using `go test -bench=.` with `b.ReportAllocs()`:

| Benchmark | Iterations | Measured Time | Allocations / Op | Throughput |
| :--- | :---: | :---: | :---: | :---: |
| `BenchmarkFSPoller_Scan` (510 items) | 1,036 | **1.16 ms/op** | 487 KB / 3,731 allocs | ~860 full tree scans/sec |
| `BenchmarkStorage_SearchFTS` | 15,918 | **75.2 µs/op** | 1.9 KB / 51 allocs | ~13,290 queries/sec |
| `BenchmarkStorage_DiscoverProjects` | 2,486 | **463.0 µs/op** | 102 KB / 1,959 allocs | ~2,160 project discovers/sec |

*Note on Prior Mismeasurement*: Prompt 10 reported `0.23 ns/op` for `BenchmarkFSPoller_Scan` because the loop called a getter (`MonitoredRoot()`) rather than executing `scan()`. Prompt 11 corrected the benchmark to perform the full recursive directory walk and metadata extraction.

---

## 17. Known Limitations & Unresolved Issues

- **Snapshot Polling Window**: Rapid file creations and deletions that occur entirely between polling intervals (e.g. within 1 second) cannot be detected without kernel hooks (FSEvents). FSEvents was deliberately omitted to preserve the pure-Go zero-CGO mandate.
- **Git State Granularity**: Git repository tracking reflects state at the moment of `lifelog git scan`. Rapid branch switches or commit amends occurring between scans are not captured.
- **SQLite Concurrency Throughput**: Under high concurrent writer volume, transactions queue behind the 5,000ms busy timeout.

---

## 18. Prompt 12: Independent Release Verification & Acceptance Audit

### 18.1 Environment & Platform
- **Operating System**: macOS Darwin 24.3.0 (`darwin/arm64`)
- **Installed Go Version**: `go1.27.2 darwin/arm64`
- **Module Declared Version**: `go 1.27.2` (in `go.mod`)
- **Authoritative Database Schema**: Schema Version 7 (Migrations 1–7)

### 18.2 Verification Suite Results
| Check Command | Exit Code | Result | Details |
| :--- | :---: | :---: | :--- |
| `gofmt -l .` | 0 | **PASS** | 0 unformatted Go files. |
| `go vet ./...` | 0 | **PASS** | 0 warnings or issues across all packages. |
| `go test -count=1 ./...` | 0 | **PASS** | 11/11 packages pass cleanly. |
| `go test -count=1 -race ./...` | 0 | **PASS** | 11/11 packages pass with zero data races. |
| `go build -o /tmp/lifelog-release-check .` | 0 | **PASS** | Binary compiles cleanly without warnings or CGO. |
| `git diff --check` | 0 | **PASS** | 0 whitespace errors or conflict markers. |

### 18.3 CLI Acceptance Verification (Subprocess Execution)
All 17 CLI commands were executed against the compiled `/tmp/lifelog-release-check` binary in an isolated temporary environment (`$LIFELOG_DATA_DIR` pointing to fresh temporary directory):
1. `--help` & `version`: Returned expected usage and version string (`0.1.0`). Exit code 0.
2. `init`: Initialized directory layout, config, and schema v7. Exit code 0.
3. `status`: Reported healthy application state and storage paths. Exit code 0.
4. `doctor`: Executed all diagnostics with 0 failures. Exit code 0.
5. `events`: Queried recorded events with limits and path filters. Exit code 0.
6. `git scan`, `repos`, `git status`, `git log`, `git branches`: Inspected and persisted real Git repository state. Exit code 0.
7. `terminal record`: Executed command, streamed output, redacted sensitive token (`--token=ghp_...`). Exit code 0.
8. `terminal history`, `inspect`, `delete`, `clear`: Managed recorded executions. Exit code 0.
9. `search` & `timeline`: Searched records across all domains; confirmed secret token was 100% absent. Exit code 0.
10. `resume`: Synthesized multi-domain activity sessions and evidence inferences. Exit code 0.
11. `export`: Exported valid JSON with `0600` permissions. Exit code 0.
12. `checkpoint create`, `list`, `show`, `delete`: Managed immutable historical snapshots. Exit code 0.
13. `package`: Generated portable package bundle with domain accounting. Exit code 0.
14. `projects`: Discovered known projects across Git, checkpoints, and terminal records. Exit code 0.
15. Protected path rejection: Rejected export into `$LIFELOG_DATA_DIR/lifelog.db` with non-zero exit code.
16. Invalid command handling: Rejected unknown commands with exit code 2 and usage guidance.
17. Direct SQLite inspection: Confirmed 0 secret leaks in raw `terminal_records` and `search_index`.

---

## 19. v0.1.0 Release Recommendation

**READY FOR INITIAL RELEASE (v0.1.0)**.
The codebase satisfies all release gate requirements:
1. Pure-Go zero-CGO architecture strictly preserved.
2. Local-first privacy boundaries rigorously verified (zero file content reading, zero terminal output capture, zero remote networking).
3. Schema migrations v1 through v7 verified with automated upgrade and rollback tests.
4. Comprehensive test coverage passing with `-race` across all packages.
5. Production CLI acceptance verified via subprocess binary execution.
6. Single source of truth reconciled across all documentation.

---

## 20. LIFELOG V2 — Architecture, Product Scope & Migration Plan

### 20.1 Current V1 Architecture & Feature Inventory

LIFELOG V1 is an established, production-grade Go CLI application (tagged `v0.1.0`, schema version 7) designed for local-first computing memory on macOS.

#### Subsystem Inventory:
1. **CLI Dispatcher (`internal/cli`)**: 17 subcommands (`init`, `status`, `doctor`, `watch`, `events`, `git`, `repos`, `terminal`, `search`, `timeline`, `resume`, `package`, `projects`, `export`, `checkpoint`, `help`, `version`).
2. **Filesystem Observation Engine (`internal/collector`)**: Pure-Go directory poller (`fs_poll`) scanning target directories recursively. Stores metadata (`path`, `size`, `mod_time`, `is_dir`, `event_type`). Deduplicates identical sequential events. Zero file content reading.
3. **Git Memory Engine (`internal/git`)**: Safe, local read-only execution of Git commands (`rev-parse`, `status -z`, `log`, `for-each-ref`). Discovers repository roots, tracks branch tips, records commit subjects, and captures working-tree changes without remote network access or URL inspection.
4. **Terminal Memory Engine (`internal/terminal`)**: Explicit opt-in command runner (`lifelog terminal record -- <cmd>`). Heuristically redacts flags (`--token`, `--password`, `--api-key`), environment assignments, and bearer tokens. Omits arguments for credential tools (`security`, `op`, `pass`, `vault`, `gpg`, `ssh-add`). Streams stdout/stderr directly to user; never buffers or persists output.
5. **Unified Search & Timeline Engine (`internal/search`, `internal/storage`)**: SQLite FTS5 search index (`search_index`) synchronized in real time via database triggers on filesystem, Git, and terminal tables. Provides BM25 + recency ranking and escaped SQL `LIKE` fallback.
6. **Resumption & Session Synthesis Engine (`internal/resumption`)**: Clusters chronological events into sessions separated by an inactivity gap threshold (default 30m). Separates facts into Observed, Derived, and Unavailable categories. Resolves canonical macOS `/var` vs `/private/var` symlink aliases.
7. **Export & Checkpoint Engine (`internal/resumption`)**: Generates portable Markdown/JSON packages with atomic file writes (`0600` permissions) and destination boundary protection against writing into `$LIFELOG_DATA_DIR`. Persists immutable historical checkpoints in the `checkpoints` table.
8. **Multi-Project Discovery (`internal/storage/project_storage.go`)**: Discovers projects from persisted activity records across all domains without crawling the filesystem.
9. **SQLite Storage Layer (`internal/storage`)**: Pure-Go SQLite (`modernc.org/sqlite`) operating in WAL mode with immediate transaction locking (`_txlock=immediate`), foreign keys enabled, and a 5,000ms busy timeout. Authoritative schema version 7 across 7 forward-only migrations.

---

### 20.2 V1 Feature Preservation & Migration Matrix

Every V1 feature continues to operate in V2. The Go backend binary will function in dual mode: as a standalone CLI for developers, and as a headless background daemon/sidecar when launched by the Electron desktop shell.

| V1 Feature / CLI Command | V1 Implementation | V2 Desktop UI Manifestation | Preservation Strategy |
| :--- | :--- | :--- | :--- |
| `lifelog init`, `doctor`, `status` | `internal/cli/init.go`, `doctor.go`, `status.go` | First-run onboarding flow & Settings -> Diagnostics health view | Retained verbatim in CLI; exposed via IPC to Electron. |
| `lifelog watch` | `internal/collector/poller.go` | Folders view: add/remove monitored directories, live event stream | Daemon runs watchers; GUI displays real-time badges & event logs. |
| `lifelog events` | `internal/storage/storage.go` | Interactive event log with path filters and date pickers | Query API wrapped in IPC handler; pagination supported. |
| `lifelog git scan` / `status` / `log` | `internal/git/client.go`, `internal/storage/git_storage.go` | Projects view -> Git State card (branches, uncommitted changes, commit feed) | Backend executes local read-only commands; renders visual branch badges. |
| `lifelog terminal record` / `history` | `internal/terminal/executor.go`, `sanitizer.go` | Terminal Activity view & search integration; optional shell alias helper | CLI workflow preserved 100%; GUI allows filtering, viewing redaction notes, and clearing history. |
| `lifelog search` | `internal/storage/search_storage.go` | Global Search overlay (`Cmd+K`) with instant multi-domain results | FTS5 queries executed via IPC; search results returned in <10ms. |
| `lifelog timeline` | `internal/storage/search_storage.go` | Unified Chronological Timeline feed with domain filters | Continuous scrollable feed aggregating FS, Git, Terminal, Apps, System. |
| `lifelog resume` | `internal/resumption/synthesis.go` | Project Resumption Dashboard view with session cards | Synthesis engine computes report; rendered as rich visual desktop UI. |
| `lifelog package` / `export` | `internal/resumption/package.go`, `export.go` | "Export Context" modal with Markdown/JSON download and clipboard copy | Safe atomic writer reused; native macOS file-save dialog integration. |
| `lifelog checkpoint` | `internal/resumption/checkpoint.go` | Checkpoints manager: snapshot list, historical view, create, delete | Checkpoint storage preserved; GUI provides one-click snapshot buttons. |
| `lifelog projects` | `internal/storage/project_storage.go` | Projects Grid overview: active vs dormant cards, sorting by activity | Backend discovery reused; renders visual project cards. |

---

### 20.3 Proposed V2 Architecture & Component Diagram

LIFELOG V2 adopts a decoupled, multi-process architecture consisting of an **Electron Desktop Shell** and a **Reused Go Core Daemon**, communicating over an authenticated **Unix Domain Socket (UDS)**:

```mermaid
flowchart TD
    subgraph DesktopApp["Electron Desktop Application"]
        subgraph Renderer["Renderer Process (Chromium GUI)"]
            UI["Desktop UI (React / Tailwind / Design System)<br/>Dashboard • Timeline • Projects • Apps • Search"]
        end

        subgraph Preload["Preload Script (Context Isolation)"]
            BRIDGE["Minimal Preload Bridge<br/>contextBridge.exposeInMainWorld('lifelog', api)<br/>Strictly Typed Event Emitters & RPC Calls"]
        end

        subgraph MainProcess["Electron Main Process (Node.js)"]
            LIFECYCLE["App Lifecycle, Tray Icon, Dock Menu, Native Windows"]
            SETTINGS["User Preferences (Launch at Login, Window State)"]
            CLIENT["IPC Client (UDS Connection Pool & Token Handshake)"]
            NOTIFY["macOS Native Notifications & System Tray"]
        end
    end

    subgraph IPCBoundary["Local OS IPC Boundary (0600 Permissions)"]
        UDS[("Unix Domain Socket<br/>~/Library/Application Support/LifeLog/lifelog.sock<br/>Restricted UID • Ephemeral Handshake Token")]
    end

    subgraph GoDaemon["Go Core Daemon (Reused & Extended V1 Codebase)"]
        DAEMON["Daemon Server (JSON-RPC 2.0 / Stream Dispatcher)"]

        subgraph TrackingEngines["Activity Observers"]
            FS_ENG["Filesystem Poller / Watcher (V1)"]
            GIT_ENG["Git Observer (V1)"]
            TERM_ENG["Terminal Memory Engine (V1)"]
            APP_ENG["macOS App Usage Observer (V2 Native Helper)"]
            SYS_ENG["System Sleep / Wake / Idle Observer (V2)"]
            BROWSER_ENG["Browser Domain Integration (V2 Optional)"]
        end

        subgraph CoreEngines["Core Processing & Storage"]
            SYNTH_ENG["Resumption & Session Synthesis Engine (V1)"]
            SEARCH_ENG["SQLite FTS5 Unified Search Engine (V1)"]
            EXP_ENG["Package & Checkpoint Manager (V1)"]
            DB[("SQLite Database (Schema v7 -> v10)<br/>WAL Mode • modernc.org/sqlite • Zero CGO")]
        end
    end

    UI <-->|IPC| BRIDGE
    BRIDGE <-->|ipcRenderer / ipcMain| CLIENT
    CLIENT <-->|JSON-RPC over UDS| UDS
    UDS <-->|JSON-RPC Handshake| DAEMON

    DAEMON --> TrackingEngines
    TrackingEngines --> CoreEngines
    CoreEngines --> DB
```

#### Communication Mechanism Comparison & Justification:

1. **Unauthenticated Local HTTP Server (`localhost:PORT`)**:
   - *Security Risk*: **REJECTED**. Any website or script running in any browser tab on the user's machine can send requests to `localhost` and exfiltrate private computer memory without user knowledge.
2. **Standard I/O Pipes (`stdin`/`stdout`)**:
   - *Security Assessment*: Safe and isolated to parent/child process, but lacks multi-client support (e.g., CLI querying daemon while GUI is open) and makes binary streaming complex.
3. **Authenticated Unix Domain Socket (UDS)**:
   - *Security Assessment*: **SELECTED**.
   - Accessible only by processes running under the current user's UID (file permissions `0600` on the socket file in `~/Library/Application Support/LifeLog/lifelog.sock`).
   - Browser tabs cannot connect to Unix Domain Sockets.
   - Electron Main process generates an ephemeral cryptographically secure token at launch passed via environment variable to the Go child process; all socket frames must pass this handshake token.
   - High throughput, low CPU overhead, zero network attack surface.

---

### 20.4 Tracking-Source Matrix (7 Sources)

| Source | Data Collected | macOS Permissions Required | Collection Mechanism | Privacy Boundaries & Limitations | Resource Cost | Enable / Disable Behavior |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **1. Application Usage** | App bundle ID, app name, active window start/end UTC timestamps, duration. | **None** required for active app (`NSWorkspace.shared.frontmostApplication`). Accessibility permission required *only* if window titles are enabled. | Native helper listening to `NSWorkspaceDidActivateApplicationNotification` via event-driven callback. | Captures app identity only. **Zero keystrokes, zero window text, zero DOM inspection**. Window titles sanitized or disabled by default. | Negligible (<0.1% CPU). Purely event-driven; zero polling loop. | Toggle in Settings. Immediately halts event processing when disabled. |
| **2. Filesystem Activity** | File path, size, modification time, event type (`created`, `modified`, `deleted`). Metadata only. | Standard filesystem access for explicitly selected directories. Full Disk Access **not required**. | Reused pure-Go `fs_poll` engine. Directories chosen explicitly by user. | **Zero file content inspection**. Broad roots (`/`, `$HOME`) rejected. Symlinks not followed into parent trees. | ~1.16 ms per 510 items (Apple M4). Idle CPU <0.2%. | Per-folder watcher toggle. Can be paused globally. |
| **3. Git Activity** | Branch names, HEAD commit hash/subject/author/time, working-tree file status list. | Read access to repository folder. | Reused `internal/git` client executing local `git rev-parse`, `status -z`, `log`, `for-each-ref`. | **Zero remote URLs, zero credentials, zero network calls**. Only runs local read-only commands. | Low (<15ms per repo scan). Triggered on folder changes or manual refresh. | Automatic per registered repo or on-demand. |
| **4. Terminal Activity** | Explicitly executed command string, directory, duration, exit code. | None. | `lifelog terminal record -- <cmd>` CLI runner or optional shell alias (`logrun`). | **Zero stdout/stderr capture**. Heuristic secret redaction masks `--token`, `--password`, etc. Credential tools (`op`, `security`, `pass`) omitted. | Sub-millisecond execution overhead. | Opt-in execution only. No background shell interception without deliberate command invocation. |
| **5. Browser Activity** | Active domain/host (e.g. `github.com`), active start/end timestamps, duration. | Safari / Chrome Automation permission (`com.apple.security.automation.apple-events`) OR lightweight browser extension. | Optional local browser extension sending domain changes to UDS, or AppleScript query to frontmost tab on browser activation. | **Strictly opt-in (disabled by default)**. Captures domain only. **Never reads cookies, passwords, URL query parameters, browser profiles, or history databases**. Ignored in private/incognito mode. | Event-driven on tab switch; <0.2% CPU. | Disabled by default. Requires explicit opt-in toggle in Privacy settings. |
| **6. System Activity** | System sleep, wake, screen lock, screen unlock, user idle threshold events. | None. | `NSWorkspace.willSleepNotification`, `didWakeNotification`, and `CGEventSourceSecondsSinceLastEventType`. | Records system state transitions only (Sleep, Wake, Idle). Zero biometric, camera, or sensor access. | Zero measurable CPU (OS notification hooks). | Toggle in Settings. |
| **7. Unified Memory** | Cross-domain chronological timeline, activity session clustering, full-text search, resumption packages. | None (internal SQLite operations). | Reused `internal/resumption` clustering algorithm, SQLite FTS5 index, and database triggers. | Evidence-backed inferences only. Clear separation of Observed facts vs Derived deductions vs Unavailable context. | Query latency <10ms for timeline, <1ms for FTS search. | Always available for browsing recorded local data. |

---

### 20.5 Electron and Go Process Lifecycle Design

#### Process Roles:
1. **Electron Main (`main.ts`)**:
   - Manages application window lifecycle (Single instance lock via `app.requestSingleInstanceLock()`).
   - Spawns the Go binary as a managed child process (`child_process.spawn`) with `--daemon --socket=<path>`.
   - Creates macOS menu bar extra (`NSStatusItem` / Tray) with Quick Status and Pause toggle.
   - Manages auto-launch at login via `app.setLoginItemSettings()`.
2. **Preload Script (`preload.ts`)**:
   - Sandboxed execution (`contextIsolation: true`, `nodeIntegration: false`).
   - Exposes safe, typed methods: `window.lifelog.getTimeline()`, `window.lifelog.getResumption()`, `window.lifelog.pauseTracking()`.
3. **Renderer Process (`renderer/`)**:
   - Pure UI rendering. Sandboxed without access to `node:fs` or `node:child_process`.
   - Communicates strictly via `ipcRenderer.invoke`.
4. **Go Daemon Process (`lifelog daemon`)**:
   - Listens on `~/Library/Application Support/LifeLog/lifelog.sock`.
   - Accepts incoming JSON-RPC connections from Electron Main.
   - Manages background filesystem pollers, app event subscribers, and SQLite connection pool.

#### Lifecycle Transitions:
```mermaid
stateDiagram-v2
    [*] --> ElectronLaunch: User opens LIFELOG
    ElectronLaunch --> SpawnDaemon: Main process initializes
    SpawnDaemon --> Handshake: Spawn 'lifelog daemon --socket=...'
    Handshake --> Ready: Token verification over UDS
    Ready --> ActiveTracking: Normal operation (UI connected)

    ActiveTracking --> Paused: User clicks 'Pause Tracking'
    Paused --> ActiveTracking: User clicks 'Resume Tracking'

    ActiveTracking --> SleepMode: OS willSleepNotification
    SleepMode --> WakeRecovery: OS didWakeNotification
    WakeRecovery --> ActiveTracking: Catch-up Git & FS scan

    ActiveTracking --> GracefulShutdown: App quit requested (Cmd+Q)
    GracefulShutdown --> StopDaemon: Send SIGTERM / shutdown RPC
    StopDaemon --> CloseDB: Flush WAL, commit transactions, remove socket
    CloseDB --> [*]: Process exits cleanly (code 0)
```

#### Crash Recovery & Watchdog:
- If the Go daemon crashes unexpectedly, Electron Main captures the exit code, logs the error, waits a 1-second backoff, and automatically restarts the daemon.
- If more than 3 crashes occur within 60 seconds, Electron enters Safe Mode, displays a diagnostic recovery dialog, and offers to run `lifelog doctor`.

---

### 20.6 Proposed Database Changes & Migration Strategy

#### Guiding Rules:
- **Zero changes to existing V1 tables**. All existing data in `filesystem_events`, `git_repositories`, `git_commits`, `git_snapshots`, `terminal_records`, `search_index`, and `checkpoints` remains 100% intact.
- **Forward-only migrations** registered in `internal/storage/storage.go`.

#### New Migrations (v8 through v10):

##### Migration 8: `v8_app_usage_schema`
```sql
CREATE TABLE IF NOT EXISTS app_sessions (
    id TEXT PRIMARY KEY,
    app_bundle_id TEXT NOT NULL,
    app_name TEXT NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    duration_ms INTEGER NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_app_sessions_start ON app_sessions(start_time);
CREATE INDEX IF NOT EXISTS idx_app_sessions_bundle ON app_sessions(app_bundle_id);

-- Search Index Trigger for App Sessions
CREATE TRIGGER IF NOT EXISTS trg_app_sessions_insert AFTER INSERT ON app_sessions BEGIN
    INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
    VALUES ('app', new.id, new.app_name, new.app_name || ' ' || new.app_bundle_id, new.app_name, new.start_time);
END;

CREATE TRIGGER IF NOT EXISTS trg_app_sessions_delete AFTER DELETE ON app_sessions BEGIN
    DELETE FROM search_index WHERE domain = 'app' AND record_id = old.id;
END;
```

##### Migration 9: `v9_browser_activity_schema`
```sql
CREATE TABLE IF NOT EXISTS browser_sessions (
    id TEXT PRIMARY KEY,
    browser_name TEXT NOT NULL,
    domain TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    duration_ms INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_browser_sessions_start ON browser_sessions(start_time);
CREATE INDEX IF NOT EXISTS idx_browser_sessions_domain ON browser_sessions(domain);

-- Search Index Trigger for Browser Sessions
CREATE TRIGGER IF NOT EXISTS trg_browser_sessions_insert AFTER INSERT ON browser_sessions BEGIN
    INSERT INTO search_index(domain, record_id, title, content, working_path, timestamp)
    VALUES ('browser', new.id, new.domain, new.domain || ' ' || new.title || ' ' || new.browser_name, new.domain, new.start_time);
END;

CREATE TRIGGER IF NOT EXISTS trg_browser_sessions_delete AFTER DELETE ON browser_sessions BEGIN
    DELETE FROM search_index WHERE domain = 'browser' AND record_id = old.id;
END;
```

##### Migration 10: `v10_system_events_schema`
```sql
CREATE TABLE IF NOT EXISTS system_events (
    id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL, -- 'sleep', 'wake', 'lock', 'unlock', 'idle_start', 'idle_end'
    timestamp TIMESTAMP NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    metadata TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_system_events_time ON system_events(timestamp);
```

---

### 20.7 GUI Information Architecture & Screen Specifications

#### 1. Onboarding & First-Run Setup:
- **Visual Style**: Clean setup modal with system font typography and privacy badges.
- **Components**:
  - Welcome explanation: "LIFELOG is your local-first computer memory. Nothing leaves your Mac."
  - Data directory verification: Confirms `~/Library/Application Support/LifeLog` with `0700` permissions.
  - Source configuration: Checkboxes to enable/disable App Usage, Filesystem, Git, Terminal, Browser (disabled by default).
  - Directory selection: Quick button to select initial project folders to watch.

#### 2. Shell & Top Navigation Bar:
- **Left**: LIFELOG wordmark, active workspace name, live tracking indicator (Pulsing green dot = Recording, Amber = Paused).
- **Center**: Quick Search bar (`Cmd+K`).
- **Right**: Global Pause button (dropdown: Pause 15m, 1h, Until Tomorrow), Status indicator, Settings gear.

#### 3. Main Views (Sidebar Navigation):

```text
┌─────────────────┬──────────────────────────────────────────────────────────────┐
│  LIFELOG        │  [● Tracking Active]      [🔍 Search Cmd+K]      [Pause ▾]   │
├─────────────────┼──────────────────────────────────────────────────────────────┤
│  📊 Overview    │  TODAY'S ACTIVITY SUMMARY                                    │
│  ⏱️ Timeline    │  4h 12m Active Work  •  184 File Changes  •  3 Commits       │
│  📁 Projects    │ ┌───────────────────────────┬──────────────────────────────┐ │
│  💻 Apps        │ │ Active Project: LifeLog   │ Top Apps Today               │ │
│  🔍 Search      │ │ Branch: main (Clean)      │ • VS Code      2h 45m        │ │
│  ⚙️ Settings    │ │ Last: modified poller.go  │ • Terminal        48m        │ │
│                 │ │ [Resume Project]          │ • Slack           39m        │ │
│                 │ └───────────────────────────┴──────────────────────────────┘ │
│                 │ RECENT CHRONOLOGICAL TIMELINE                                │
│                 │  17:04  git     commit "feat: v0.1.0 release"                │
│                 │  17:01  terminal echo "test" exit=0                          │
│                 │  16:45  files    modified 3 files in internal/resumption     │
└─────────────────┴──────────────────────────────────────────────────────────────┘
```

- **View 1: Overview Dashboard**:
  - Focus metrics: Active computer time today, top projects, total filesystem events.
  - Active project card: Current directory, branch, uncommitted changes, "Resume" action button.
  - App distribution breakdown: Horizontal segmented bar showing time spent across Code, Terminal, Browser, Communication.
- **View 2: Unified Chronological Timeline**:
  - Multi-lane chronological feed with source filter chips: `All`, `Files`, `Git`, `Terminal`, `Apps`, `System`.
  - Date navigation: Quick jump to Today, Yesterday, or custom date picker.
  - Expandable event cards: Displays exact UTC timestamp, relative path, duration, and metadata details.
- **View 3: Projects & Resumption Hub**:
  - Grid of all known projects discovered by LIFELOG.
  - One-click "Resume" opens the full Resumption Report with Observed facts, Derived activity sessions, and Unavailable context.
  - "Export Package" button generates Markdown context package.
  - Checkpoint manager tab: List saved checkpoints, inspect snapshot details, capture new checkpoint.
- **View 4: Application Usage Breakdown**:
  - Daily & weekly application usage charts.
  - Session timeline: Chronological blocks of foreground application usage.
- **View 5: Global Search Overlay (`Cmd+K`)**:
  - Instant modal with keyboard navigation (Up/Down/Enter).
  - Searches across all 7 domains with token highlighting and domain icons.
- **View 6: Settings & Privacy Center**:
  - Per-source toggles: Enable/disable Filesystem, Git, Terminal, Apps, Browser, System individually.
  - Monitored folder manager: Add/remove watched paths.
  - Data Retention settings: Keep records for 30 days, 90 days, 1 year, or Indefinitely.
  - Danger Zone: "Export Complete Database" and "Permanently Wipe All Data" (requires typed confirmation).

---

### 20.8 Threat Model & Privacy Review

| Threat Scenario | Risk Level | Mitigation & Technical Enforcement |
| :--- | :---: | :--- |
| **Local malicious process exfiltrates memory data** | Medium | Database and UDS socket have restrictive `0600` permissions. Ephemeral secret handshake token required to establish UDS session. Only same-UID processes can connect. |
| **Malicious browser website attempts localhost fetch** | Critical | **Zero HTTP endpoints**. Local HTTP server is strictly rejected; browsers cannot communicate with Unix Domain Sockets. |
| **Accidental secret persistence in terminal recording** | High | Heuristic regex masks known token formats (`ghp_...`, AWS keys, bearer tokens) and passwords. Credential tools (`op`, `security`, `pass`) omit arguments entirely. Terminal output is never captured. |
| **Sensitive browser URL / query token leakage** | High | Browser integration captures domain only (e.g. `github.com`), never full URLs, query strings, or authorization headers. Disabled by default. |
| **Electron XSS / Remote Code Execution** | Medium | Context isolation enabled, `nodeIntegration: false`, strict Content Security Policy (`CSP`) forbidding `unsafe-eval` and remote script loading. Minimal preload API surface. |
| **Unbounded disk consumption** | Low | SQLite WAL checkpointing, database size diagnostics, configurable retention policies, and automatic pruning of stale event batches. |

---

### 20.9 Performance Targets & Measurement Methodology

#### Targets:
- **Idle CPU Utilization**: < 0.5% average CPU on Apple Silicon when no computer activity occurs.
- **Active Editing CPU**: < 2.0% CPU during rapid typing/editing in a watched project.
- **Memory Footprint (RSS)**:
  - Go Core Daemon: < 35 MB RSS.
  - Electron Desktop App: < 150 MB combined RSS (Main + Renderer + GPU).
- **Background Wakeups**: Zero background polling loops when idle (event-driven OS hooks for app and sleep notifications).
- **Event Latency**: < 5 ms from OS event to SQLite transaction commit.
- **Database Growth Rate**: ~5 to 15 MB per month of active developer usage.

#### Measurement Methodology:
1. **Automated Micro-Benchmarks**:
   - Go benchmark suite (`go test -bench=. -benchmem`) measuring database inserts, FTS search, and session clustering.
2. **Process Resource Sampling**:
   - Automated profiling script running `ps -o %cpu,rss -p <pid>` at 1s intervals during simulated 1-hour work sessions.
3. **Apple Instruments Integration**:
   - Profiling with `Time Profiler` and `Allocations` instruments on macOS to verify zero thread wakeups during idle state.

---

### 20.10 Phased Implementation Roadmap

```mermaid
gantt
    title LIFELOG V2 Phased Implementation Roadmap
    dateFormat  YYYY-MM-DD
    section Phase 1: IPC & Daemon
    Go Daemon Mode & UDS Server          :p1_1, 2026-10-13, 3d
    JSON-RPC 2.0 Protocol & Handshake    :p1_2, after p1_1, 2d
    section Phase 2: Electron Shell
    Electron Scaffold & Sandboxing       :p2_1, after p1_2, 3d
    Preload Bridge & Tray Status Menu    :p2_2, after p2_1, 2d
    section Phase 3: Core Desktop UI
    Design System & Shell Layout         :p3_1, after p2_2, 3d
    Overview, Timeline & Resume Views    :p3_2, after p3_1, 4d
    section Phase 4: App Usage Tracking
    Native macOS App Observer Helper     :p4_1, after p3_2, 3d
    Migration v8 & App Usage UI          :p4_2, after p4_1, 3d
    section Phase 5: Privacy & Polish
    Settings, Retention & Data Wipe      :p5_1, after p4_2, 3d
    System Sleep/Wake Integration        :p5_2, after p5_1, 2d
    section Phase 6: Packaging & Release
    macOS Notarization & DMG Build       :p6_1, after p5_2, 3d
```

#### Phase Breakdown:
- **Phase 1: Go Daemon & IPC Bridge**:
  - Add `lifelog daemon --socket=<path>` subcommand to Go binary.
  - Implement JSON-RPC 2.0 server over Unix Domain Socket with handshake token.
  - Acceptance Criteria: Electron can invoke queries (`getTimeline`, `getProjects`, `getResumption`) over socket with <5ms latency.
- **Phase 2: Electron Desktop Shell & Preload Bridge**:
  - Initialize Electron project with TypeScript, context isolation, and secure CSP.
  - Create native macOS menu bar item (Tray) with live recording indicator.
  - Acceptance Criteria: Electron launches Go child process, handles clean shutdown, and restarts on crash.
- **Phase 3: Core Desktop Interface (V1 Features in GUI)**:
  - Implement design system (Apple SF Pro, dark/light themes, card layouts).
  - Build Dashboard, Chronological Timeline, Projects Hub, and Global Search (`Cmd+K`).
  - Acceptance Criteria: Full parity with all V1 CLI features visible and interactive in the desktop GUI.
- **Phase 4: Application Usage Tracking**:
  - Implement native macOS application activation observer.
  - Register Migration 8 (`v8_app_usage_schema`) and FTS triggers.
  - Build App Usage breakdown screens in GUI.
  - Acceptance Criteria: Frontmost app changes tracked with zero keystroke or window content capture.
- **Phase 5: Privacy Controls & System Activity**:
  - Implement System Sleep/Wake/Idle tracking (Migration 10).
  - Implement per-source pause/resume controls and permanent data wipe dialog.
  - Build configurable data retention pruner.
  - Acceptance Criteria: User can pause tracking, wipe data, and toggle each source independently.
- **Phase 6: macOS Packaging & Release**:
  - Package universal macOS application (`darwin-arm64` and `darwin-x64`).
  - Configure code signing and Apple Developer notarization pipeline.
  - Acceptance Criteria: Distributable DMG installs cleanly and passes Gatekeeper.

---

### 20.11 Risks, macOS Limitations & Unresolved Decisions

1. **Frontmost Application vs Background Activity**:
   - *Limitation*: `NSWorkspace` accurately detects the active frontmost app, but does not capture background audio, video playback, or split-screen windows without Accessibility permissions.
   - *Decision*: Maintain strict privacy boundary; track frontmost application only. Avoid requiring intrusive Accessibility permissions for basic app tracking.
2. **Native macOS Helper Architecture**:
   - *Decision*: Compile a small standalone helper executable (`lifelog-app-helper`) or link via pure Go `syscall` to maintain zero-CGO in the main Go binary.
3. **Frontend Framework Selection**:
   - *Recommendation*: **React + Tailwind CSS + Lucide Icons + Vite** inside Electron Renderer. Offers fast rendering, mature component ecosystem, and clean desktop design patterns while avoiding heavy runtime overhead.
4. **App Store Sandbox Restrictions**:
   - *Limitation*: Mac App Store sandboxing prevents watching arbitrary directories outside containerized folders.
   - *Decision*: Distribute outside Mac App Store via notarized DMG installer with Developer ID signing.

---

### 20.12 Recommended Initial V2 Milestone

**Milestone V2-M1: "Go Daemon IPC Bridge & Desktop Shell Scaffold"**

#### Objective:
Establish the Electron-to-Go bridge without breaking any existing V1 CLI functionality, enabling live V1 data to render in a desktop window.

#### Scope of V2-M1:
1. Add `lifelog daemon` command in Go backend accepting JSON-RPC commands over Unix Domain Socket.
2. Scaffold Electron project in `desktop/` directory with TypeScript, secure context isolation, and minimal preload bridge.
3. Implement Main Process daemon supervisor (spawn, token handshake, SIGTERM shutdown).
4. Build the core Desktop Shell rendering the Overview Dashboard, Live Timeline, and Project Resumption report.
5. Automated test suite verifying UDS communication, process lifecycle, and zero regression on existing V1 CLI tests.

---

## 21. LIFELOG V2 — Daemon & Secure Local IPC Foundation (Implemented)

### 21.1 Overview & Implemented Scope

The Go background daemon and local IPC layer have been implemented and validated with zero modifications to existing schema migrations (schema version 7 retained) and zero regressions on existing V1 CLI commands.

#### Deliverables Summary:
1. **Daemon Subcommand (`internal/cli/daemon.go`)**:
   - Registered under `lifelog daemon [--socket=<path>]`.
   - Default socket path derived securely via `paths.Resolve().Socket` (`~/Library/Application Support/LifeLog/lifelog.sock`).
   - Clean signal handling for `os.Interrupt` and `syscall.SIGTERM`.
   - Graceful shutdown draining in-flight requests before removing socket.
2. **IPC Engine (`internal/ipc`)**:
   - `types.go`: Typed JSON-RPC 2.0 requests, responses, structured errors, and domain payloads.
   - `socket.go`: Strict socket path validation, parent directory permission enforcement (`0700`), stale-socket ownership and liveliness verification, and macOS 103-character path length checks.
   - `server.go`: Concurrent Unix domain socket server with connection pooling (max 32), bounded line reader (max 1 MiB), request timeouts (15s), and idle deadlines (120s).
   - `handlers.go`: Strict input validation and routing for `auth`, `health`, `getProjects`, `getTimeline`, and `getResumption`.
   - `client.go`: Type-safe JSON-RPC 2.0 client for automated integration tests and programmatic access.
   - `ipc_test.go`: 9 test suites covering authentication, malformed payloads, parameter boundary checks, resource limits, permissions, stale socket handling, concurrency, and data fidelity.

---

### 21.2 Protocol & Authentication Details

#### JSON-RPC 2.0 Wire Framing:
- Framing: Newline-delimited JSON (`\n` terminated lines) over Unix domain socket.
- Maximum payload size: 1 MiB (`1 << 20` bytes). Requests exceeding this limit are terminated with `CodeResourceExceeded` (`-32002`).
- Protocol version: Exactly `"2.0"`. Any mismatched version returns `CodeInvalidRequest` (`-32600`).

#### Error Codes:
- `CodeParseError` (`-32700`): Malformed JSON syntax.
- `CodeInvalidRequest` (`-32600`): Missing required protocol fields (`jsonrpc`, `method`).
- `CodeMethodNotFound` (`-32601`): Unrecognized method name.
- `CodeInvalidParams` (`-32602`): Parameter type mismatch, out-of-range bounds, or invalid filter values.
- `CodeInternalError` (`-32603`): Storage, query, or runtime synthesis error.
- `CodeUnauthorized` (`-32001`): Unauthenticated connection attempting application query, or invalid token.
- `CodeResourceExceeded` (`-32002`): Payload exceeds 1 MiB, or connection pool is saturated.
- `CodeServerShuttingDown` (`-32003`): Server in active draining state during graceful shutdown.

#### Authentication Handshake:
- Source: The parent process (Electron Main) supplies a secret token via `LIFELOG_IPC_TOKEN` in the child process environment. If unset, the daemon generates a 256-bit cryptographically secure token (`crypto/rand`) and displays it once to stdout.
- Security Boundary: The token is never written to disk, never placed in command-line arguments (which are observable via `ps`), and never logged.
- Comparison: Constant-time comparison via `crypto/subtle.ConstantTimeCompare` prevents timing attacks.
- Session Lifecycle: New connections start in an unauthenticated state (`authenticated = false`). The client must send an `auth` handshake request first:
  ```json
  {"jsonrpc":"2.0","id":1,"method":"auth","params":{"token":"..."}}
  ```
- If valid, the session is marked authenticated and assigned an ephemeral session identifier (`sess_<hex>`). Subsequent requests on the same connection do not repeat the token, preventing token propagation across application query logs.

#### Limits of Same-User Process Isolation:
- macOS file permissions (`0600` on socket, `0700` on parent directory) guarantee that processes running under different user accounts cannot connect to the socket.
- However, Unix domain socket permissions cannot prevent another process running as the *same user* from attempting connection.
- The cryptographically secure token passed privately through the child environment creates an authentication barrier preventing unauthorized same-user processes from issuing requests to the daemon.

---

### 21.3 Implemented & Verified Read-Only Methods

All implemented methods reuse existing V1 core business logic and database queries without duplicating code or creating new database tables:

1. `auth`:
   - Validates secret token using constant-time comparison.
   - Activates authenticated state for the current socket connection.
2. `health`:
   - Returns daemon uptime, PID, binary version (`0.1.0`), schema version (`7`), database path, and ping integrity check.
   - Protected: Requires authenticated session.
3. `getProjects`:
   - Invokes `db.DiscoverProjects(ctx, opts)` with validated `sort` (`recent`, `name`, `activity`), bounded `limit` (1-500, default 50), and lookback `window` / `since` timestamps.
4. `getTimeline`:
   - Invokes `db.Search(ctx, query)` with validated `domain` (`all`, `filesystem`, `git`, `terminal`), bounded `limit` (1-500, default 20), `offset` (>= 0), and time filters.
5. `getResumption`:
   - Invokes `resumption.BuildResumptionReport(ctx, db, canonPath, opts)` with validated project path, `limit`, inactivity `gap`, and time window.

---

### 21.4 Socket Lifecycle & Stale Socket Safety

Before calling `net.Listen("unix", socketPath)`:
1. Validates that the path length does not exceed the macOS kernel `sockaddr_un.sun_path` limit of 103 characters.
2. Checks parent directory and ensures `0700` permissions.
3. Inspects existing file at socket path using `os.Lstat`:
   - Rejects if path is a symlink (`ErrSocketIsSymlink`). Never follows or unlinks symlinks.
   - Rejects if path is a regular file or directory (`ErrSocketNotASocket`). Never removes arbitrary user files.
   - Rejects if file ownership (`uid`) does not match the current process UID (`ErrSocketWrongOwner`).
   - Attempts a dial probe (`net.DialTimeout`) with a 200ms timeout:
     - If connection succeeds, an active daemon is running (`ErrDaemonAlreadyRunning`). Fails fast without interfering.
     - If connection is refused, the socket is confirmed stale and safely removed via `os.Remove`.
4. Creates listener and explicitly applies restrictive `0600` permissions (`os.Chmod`).
5. On SIGINT/SIGTERM, shuts down listener, waits for in-flight requests, and deletes socket file.

---

### 21.5 Automated Verification Results

All tests execute in isolated temporary directories without altering user databases:

```bash
# 1. Format check
gofmt -l .                  # Clean (0 files formatted)

# 2. Static analysis
go vet ./...                # Clean (0 issues)

# 3. Full test suite (12 packages)
go test -count=1 ./...      # PASS (12/12 packages)

# 4. Data race detector (12 packages)
go test -count=1 -race ./...# PASS (0 races across all packages)

# 5. Git whitespace check
git diff --check            # Clean (0 whitespace errors)

# 6. Production binary acceptance
./lifelog status            # Healthy (connected, schema v7)
./lifelog doctor            # 6 passed, 0 failures
./lifelog version           # lifelog version 0.1.0 (darwin/arm64)
./lifelog projects          # Discovered projects active
./lifelog repos             # Repositories active
./lifelog search --limit 3  # FTS5 search index active
```

---

### 21.6 Status of V2 Plan vs Completed Work

- **Completed**: Milestone V2-M1 Go Daemon & Local IPC Foundation (`lifelog daemon`, `internal/ipc`, authenticated UDS, read-only dashboard RPCs, comprehensive concurrency & lifecycle tests).
- **Completed**: Milestone V2-M2 Desktop Shell & Electron Bridge (Scaffold `desktop/` with TypeScript, Main process daemon supervisor, preload API, React dashboard UI, visual verification, zero emojis).

---

## 22. LIFELOG V2-M2 — Electron Desktop Shell & Secure Go Bridge (Implemented)

### 22.1 Desktop Directory Structure

All desktop shell and UI code is encapsulated strictly within `desktop/`:

```text
desktop/
├── .gitignore                   # Ignores node_modules, dist, dist-electron, dist-scripts, logs
├── package.json                 # Electron 33, React 18, Vite 5, Tailwind 3, Lucide
├── package-lock.json            # Deterministic lockfile
├── tsconfig.json                # Strict TS config for renderer
├── tsconfig.main.json           # Strict TS config for Node/Electron main and preload
├── vite.config.ts               # Vite bundler configuration (root: src/renderer, outDir: dist)
├── tailwind.config.js           # Warm off-white & forest green design tokens
├── postcss.config.js            # Tailwind & Autoprefixer PostCSS pipeline
├── index.html                   # Strict Content Security Policy, zero external scripts
├── screenshots/                 # Verified rendered application screenshots
│   ├── screenshot-light-1200x800.png
│   ├── screenshot-dark-1200x800.png
│   ├── screenshot-dark-960x640.png
│   └── screenshot-dark-search-modal.png
├── scripts/
│   ├── capture-screens.ts       # Visual verification capture script (TypeScript)
│   └── capture-screens.js       # Visual verification capture runner (Node/Electron)
├── tests/                       # Automated test suite (9 passing tests)
│   ├── daemonSupervisor.test.ts # Crash tracking, backoff, binary resolution tests
│   ├── ipcClient.test.ts        # JSON-RPC 2.0 framing, timeout, max buffer tests
│   └── uiComponents.test.ts     # Zero-emoji assertion, domain filter, format tests
└── src/
    ├── main/                    # Electron Main process (Node.js)
    │   ├── types.ts             # Typed models matching Go JSON-RPC contract
    │   ├── ipcClient.ts         # JSON-RPC 2.0 client over Unix socket
    │   ├── daemon.ts            # Go daemon process supervisor & lifecycle
    │   ├── bridge.ts            # IPC handler registry & status broadcaster
    │   └── index.ts             # App lifecycle, single-instance lock, window creation
    ├── preload/                 # Preload bridge (Context Isolation)
    │   └── index.ts             # contextBridge exposing window.lifelog API
    └── renderer/src/            # React 18 UI (Vite + Tailwind + Lucide)
        ├── main.tsx             # React entry point
        ├── App.tsx              # App container, router, theme state, keyboard shortcuts
        ├── types.ts             # Frontend domain interfaces
        ├── index.css            # Base styles and custom scrollbars
        ├── components/
        │   ├── Sidebar.tsx      # Persistent navigation, status pill, theme toggle
        │   ├── Header.tsx       # Search trigger (Cmd+K), refresh control
        │   ├── CommandPalette.tsx # Global search overlay, quick-navigation
        │   ├── EmptyState.tsx   # Restrained empty states with Lucide icons
        │   └── ErrorState.tsx   # Error state with retry action
        └── views/
            ├── OverviewView.tsx # Real metrics, active projects, recent activity
            ├── TimelineView.tsx # Multi-domain filter tabs, time window, event stream
            ├── ProjectsView.tsx # Discovered projects grid with Git state
            ├── ResumptionView.tsx # 3-section report (Observed, Derived, Unavailable)
            └── SettingsView.tsx # Engine diagnostics, paths, schema version
```

### 22.2 Dependency & Build Choices

- **Electron 33.4.1**: Current stable LTS Electron runtime.
- **React 18.3.1 + ReactDOM**: Fast, declarative UI rendering without unnecessary heavy component libraries.
- **Vite 5.4.14**: Sub-second dev builds, strict bundling, zero runtime overhead.
- **TypeScript 5.7.3**: Strict type checking (`strict: true`, `noImplicitAny: true`, `noUncheckedIndexedAccess: true`).
- **Tailwind CSS 3.4.17**: Utility-first CSS tailored with curated design tokens. No runtime CSS-in-JS overhead.
- **Lucide React 0.475.0**: Clean, consistent native-feeling icons. Zero emojis anywhere.
- **Vitest 2.1.8**: In-memory unit test runner for main process supervisors, framing, and UI rules.

### 22.3 Electron Process Architecture & Preload Security Boundary

1. **Process Isolation**:
   - `contextIsolation: true`
   - `nodeIntegration: false`
   - `sandbox: true` (Electron default with context isolation)
2. **Content Security Policy (CSP)**:
   - Configured in `desktop/index.html`:
   ```html
   <meta http-equiv="Content-Security-Policy" content="default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self';" />
   ```
   - Zero remote scripts, zero remote fonts, zero CDN links.
3. **Preload API (`desktop/src/preload/index.ts`)**:
   - Exposes strictly typed `window.lifelog`:
     - `getHealth()` -> Promise<HealthResult>
     - `getProjects(options?)` -> Promise<ProjectOverview[]>
     - `getTimeline(options?)` -> Promise<TimelineItem[]>
     - `getResumption(projectPath, options?)` -> Promise<ResumptionReport>
     - `getConnectionStatus()` -> Promise<ConnectionStatus>
     - `onConnectionStatusChange(callback)` -> () => void (cleanup unsubscribe)
   - Zero access to Node.js filesystem, socket, or child process APIs from the renderer.
   - Zero token exposure: Ephemeral authentication token stays strictly inside Electron Main.

### 22.4 Go Daemon Supervision & Lifecycle

1. **Binary Location**:
   - Supervisor (`desktop/src/main/daemon.ts`) searches parent directory hierarchy for existing `lifelog` executable.
   - If missing in development, automatically triggers `go build -o lifelog .` in the project root.
2. **Startup & Authentication**:
   - Generates cryptographically secure 256-bit token (`crypto.randomBytes(32).toString('hex')`).
   - Spawns daemon with `child_process.spawn(binaryPath, ['daemon', '--socket', socketPath])`.
   - Passes token privately via `env: { ...process.env, LIFELOG_IPC_TOKEN: token }`.
   - Performs exponential probe on socket until listener is ready (timeout: 5000ms).
   - Issues `auth` request with token.
   - Validates daemon readiness via `health` RPC check.
3. **Crash Protection & Backoff**:
   - Tracks crash timestamps within a 60-second sliding window.
   - Crash limit: Maximum 3 restarts in 60s.
   - If daemon crashes repeatedly, marks status as `crashed` with actionable diagnostics and stops tight retry loops.
4. **Shutdown**:
   - On application exit (`before-quit`), sends graceful `SIGTERM` to daemon process.
   - Closes IPC socket connection pool.
   - Bounded wait (2000ms) before issuing `SIGKILL` fallback if necessary.

### 22.5 JSON-RPC 2.0 Client Over UDS (`desktop/src/main/ipcClient.ts`)

- Protocol: JSON-RPC 2.0 with newline-delimited framing (`\n`).
- Line Buffer: Bounded to 1 MiB (`MAX_LINE_BUFFER = 1024 * 1024`). Malformed oversized payloads immediately close connection with error.
- Timeouts: 10,000ms request timeout per invocation.
- Request-Response Correlation: Monotonically increasing request IDs with concurrent in-flight map.
- Error Handling: Translates JSON-RPC standard error codes (`-32700`, `-32600`, `-32001`, etc.) into structured JavaScript errors.

### 22.6 Implemented Screens & Real Data Sources

1. **Application Shell (`Sidebar.tsx`, `Header.tsx`)**:
   - Text wordmark `LIFELOG` with sub-label `Local Memory Engine`.
   - Live daemon status pill: Green dot when connected, red dot when disconnected, showing daemon version and PID.
   - Theme toggle (light/dark mode) with SVG icons.
   - Global search input button (`Cmd+K`).
2. **Overview Dashboard (`OverviewView.tsx`)**:
   - Focus metric cards: Discovered Projects (real count), Timeline Events (real count), Storage Engine (`Schema v7`, `SQLite WAL Mode`), IPC Protocol (`v2.0`, `Authenticated Unix Socket`).
   - Discovered Projects list: Real workspace cards with Git active branch and direct "Resume" action.
   - Recent Recorded Activity: Unified timeline items with timestamp, domain icon, and working path.
   - Local-first privacy guarantee card.
3. **Activity Timeline (`TimelineView.tsx`)**:
   - Domain filter tabs: `All`, `Filesystem`, `Git`, `Terminal`.
   - Time window selector: `1 Hour`, `24 Hours`, `7 Days`, `30 Days`.
   - Chronological event cards displaying domain badge, timestamp, title, and working path.
   - Empty state when no activity matches filter; error state with retry button.
4. **Known Projects (`ProjectsView.tsx`)**:
   - Grid of discovered workspaces from `getProjects`.
   - Canonical path, Git branch badge, activity counts breakdown.
   - Quick action: "Synthesize Context" navigating directly to Resumption view.
5. **Project Resumption (`ResumptionView.tsx`)**:
   - Real synthesis report from `getResumption`.
   - Formatted into 3 distinct sections:
     - `1. OBSERVED ACTIVITY`: Filesystem events, Git repository status, terminal command records.
     - `2. DERIVED ACTIVITY SESSIONS`: Chronological sessions clustered by inactivity gap (configurable: 15m, 30m, 1h, 2h).
     - `3. UNAVAILABLE CONTEXT & PRIVACY BOUNDARIES`: Explicit statements confirming zero file contents, zero terminal stdout/stderr, and zero remote URLs collected.
6. **Global Search Modal (`CommandPalette.tsx`)**:
   - Keyboard-driven overlay (`Cmd+K` / `Ctrl+K`, `Escape` to close).
   - Instant view navigation and project quick-jump.
   - Scope notice: "Cross-domain FTS5 search query RPC scheduled for V2 Search Milestone".
7. **Settings & Diagnostics (`SettingsView.tsx`)**:
   - Engine status: PID, daemon version, schema version, database path.
   - SQLite WAL configuration details.
   - Scope clarification: V2 tracking sources (app sessions, browser activity) are not yet implemented and are deferred to future milestones.

### 22.7 Visual Verification

Visual verification was conducted in a live Electron environment connecting to the real Go daemon and local database:
- **Light Theme (1200x800)**: Warm off-white canvas (`#FBFBFA`), deep forest green accents (`#1E4D2B`), charcoal typography, crisp cards.
- **Dark Theme (1200x800)**: Deep slate canvas (`#121413`), emerald accents (`#10B981`), high contrast text (`#F9FAFB`).
- **Compact Window (960x640)**: Fully responsive layout, persistent sidebar, scrollable content area without overflow or clipping.
- **Global Search Overlay**: Centered modal with backdrop blur, keyboard shortcuts, view and project quick-jumping.
- **Zero Emojis Verified**: Confirmed zero emojis across all screens, navigation items, buttons, headers, tooltips, and code files.

### 22.8 Test Execution & Verification Results

```bash
# 1. Desktop TypeScript type check
npm run typecheck --prefix desktop
# Output: Found 0 errors across main, preload, and renderer.

# 2. Desktop Unit Test Suite
npm test --prefix desktop
# Output: 3 test files passed, 9 tests passed in 369ms.
# - daemonSupervisor.test.ts (3 passed)
# - ipcClient.test.ts (3 passed)
# - uiComponents.test.ts (3 passed: zero-emoji check, domain filters, time formats)

# 3. Desktop Production Build
npm run build --prefix desktop
# Output:
# - Vite renderer build: index.html + index.css (13.78 kB) + index.js (190.22 kB)
# - TypeScript main compilation: dist-electron/main and dist-electron/preload generated cleanly.

# 4. Go Formatting Check
gofmt -l .
# Output: Clean (0 files).

# 5. Go Vet
go vet ./...
# Output: Clean (0 issues).

# 6. Go Regression Tests (No Cache)
go test -count=1 ./...
# Output: PASS across all 12 packages (2.7s - 0.4s per package).

# 7. Go Race Detector (No Cache)
go test -count=1 -race ./...
# Output: PASS across all 12 packages with zero data races.

# 8. Git Diff Check
git diff --check
# Output: Clean (0 whitespace errors or conflict markers).
```

### 22.9 Development Launch Instructions

```bash
# 1. Navigate to desktop directory
cd desktop

# 2. Install dependencies (first time only)
npm install

# 3. Start development desktop application
npm run dev
```

The Electron main supervisor will automatically build `lifelog` if needed, start `lifelog daemon`, establish the authenticated socket connection, and launch the native desktop window.

---

## 23. Milestone V2-M3 — Native macOS App Usage Tracking Implementation

### 23.1 Native Helper Architecture and Rationale

To maintain LIFELOG's strict zero-CGO requirement (`CGO_ENABLED=0`) for portable, deterministic builds, native Apple frameworks (`AppKit`, `NSWorkspace`) are not linked directly into the Go binary. Instead, an out-of-process Swift helper (`cmd/app-observer/main.swift`) is compiled into a standalone binary at `bin/lifelog-app-observer`.

**Architecture Decisions:**
- **Zero-CGO Pure Go Core:** Daemon remains pure Go. No CGO compilers or fragile macOS header bindings required to build or test the daemon.
- **Process Isolation:** The helper runs as a supervised child process managed by `internal/apps/ObserverSupervisor`. If the helper terminates unexpectedly, the Go daemon supervisor restarts it safely with exponential backoff (max 3 restarts within 60 seconds).
- **Zero Accessibility Permissions Required:** Uses public `NSWorkspace` notifications (`NSWorkspace.didActivateApplicationNotification`, `NSWorkspace.willSleepNotification`, `NSWorkspace.didWakeNotification`) and `NSWorkspace.shared.frontmostApplication`. Does not require Accessibility (`AXUIElement`) or screen recording permissions.
- **Controlled Process Channel:** The helper communicates via standard streams (`stdout` emitting line-delimited JSON; `stdin` monitored for EOF). When the daemon terminates or closes `stdin`, the helper immediately exits cleanly.

### 23.2 Helper Protocol Specification

The helper communicates using newline-delimited JSON messages with a strict protocol version (`"1.0"`). Incoming lines to Go are bounded at 64 KiB to prevent memory exhaustion attacks.

**Message Format:**
```json
{"version":"1.0","type":"app_activated","timestamp":"2026-10-12T13:16:11.838Z","app_bundle_id":"com.microsoft.VSCode","app_name":"Code"}
{"version":"1.0","type":"system_sleep","timestamp":"2026-10-12T13:20:00.000Z"}
{"version":"1.0","type":"system_wake","timestamp":"2026-10-12T13:25:00.000Z"}
{"version":"1.0","type":"heartbeat","timestamp":"2026-10-12T13:25:30.000Z"}
```

**Event Types:**
- `app_activated`: Emitted whenever frontmost macOS application transitions.
- `system_sleep`: Emitted when macOS prepares to sleep. The supervisor closes the open session immediately to avoid accumulating inactive sleep hours.
- `system_wake`: Emitted when macOS wakes. Activates tracking for the current frontmost application.
- `heartbeat`: Emitted every 30 seconds to provide liveness verification and precise crash recovery bounds.

### 23.3 Database Migration v8

Migration 8 (`app_sessions_table`) is registered in `internal/storage/storage.go` and executed within a transactional migration runner.

**Schema:**
```sql
CREATE TABLE IF NOT EXISTS app_sessions (
    id TEXT PRIMARY KEY,
    app_bundle_id TEXT NOT NULL,
    app_name TEXT NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    duration_ms INTEGER NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_app_sessions_start_time ON app_sessions(start_time);
CREATE INDEX IF NOT EXISTS idx_app_sessions_end_time ON app_sessions(end_time);
CREATE INDEX IF NOT EXISTS idx_app_sessions_bundle_id ON app_sessions(app_bundle_id);
CREATE INDEX IF NOT EXISTS idx_app_sessions_is_active ON app_sessions(is_active);

-- FTS5 full-text search indexing triggers for domain 'app'
CREATE TRIGGER IF NOT EXISTS trg_app_sessions_ai AFTER INSERT ON app_sessions BEGIN
    INSERT INTO search_index (domain, entity_id, title, content, timestamp)
    VALUES ('app', NEW.id, NEW.app_name, NEW.app_bundle_id, NEW.start_time);
END;

CREATE TRIGGER IF NOT EXISTS trg_app_sessions_au AFTER UPDATE ON app_sessions BEGIN
    UPDATE search_index
    SET title = NEW.app_name, content = NEW.app_bundle_id, timestamp = NEW.start_time
    WHERE domain = 'app' AND entity_id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS trg_app_sessions_ad AFTER DELETE ON app_sessions BEGIN
    DELETE FROM search_index WHERE domain = 'app' AND entity_id = OLD.id;
END;
```

### 23.4 Session Lifecycle and Recovery Rules

1. **Session Transitions:** When app B is activated while app A is active, app A's session is updated with `is_active = 0`, `end_time = now`, and `duration_ms = end_time - start_time`. App B's session is created with `is_active = 1`.
2. **Deduplication:** Consecutive activation events for the same bundle identifier are deduplicated in memory.
3. **Crash Recovery & Sleep Handling:** On startup, the daemon executes `CloseOpenAppSessions(ctx, maxDuration)`. Any orphan sessions left active (`is_active = 1`) due to crash, SIGKILL, power loss, or sleep are clamped to a realistic duration (last heartbeat or 30s fallback), preventing unbounded inflated hours.
4. **Duration Invariants:** Durations are strictly calculated as non-negative integers (`duration_ms >= 0`).

### 23.5 Authenticated JSON-RPC 2.0 Protocol Extensions

Four new methods added to the authenticated UDS RPC server:
- `getAppUsage`: Aggregates active foreground usage grouped by application for a given time window (e.g., `since` and `until`). Returns total time, session counts, and percentage share.
- `getAppSessions`: Returns individual paginated application sessions with optional time filters (`since`, `until`, `limit`, `offset`).
- `getTrackingStatus`: Returns operational state of observers (`app_tracking.enabled`, `app_tracking.observer_running`, `app_tracking.active_app`, `app_tracking.bundle_id`, `app_tracking.restart_count`).
- `setAppTracking`: Dynamically pauses or resumes native app tracking (`enabled: true/false`).

### 23.6 Tracking Enable/Disable Behavior

- When app tracking is disabled via `setAppTracking(enabled: false)`, the Go daemon gracefully terminates the Swift helper subprocess, closes the current foreground session, and transitions into a dormant state.
- When re-enabled, the helper is respawned, captures the initial frontmost application, and resumes recording.

### 23.7 UI Implementation: Apps View

Located at `desktop/src/renderer/src/views/AppsView.tsx`:
- **Summary Metrics Cards:** Observed active time, count of unique active applications, total recorded foreground transitions, and observer engine operational status.
- **Top Applications Bar Charts:** Visual usage progress bars sorted by duration, displaying bundle ID, session count, and elapsed time.
- **Recent Sessions Stream:** Chronological feed of foreground session transitions with start timestamps, duration tags, and active status indicator.
- **Time Range Filter:** 24 Hours, 7 Days, and 30 Days aggregations.
- **Pause / Resume Control:** Real-time toggle to disable/enable tracking on the fly.
- **Privacy Boundaries Notice:** Explanatory banner confirming strictly local-first operation without keystroke or content capture.
- **Design System:** Consistent with LIFELOG aesthetic in light mode (warm off-white canvas, forest-green accents) and dark mode (slate background, muted borders, Lucide icons, 0 emojis).

### 23.8 Privacy Boundaries

- Strictly foreground application name and bundle identifier (`CFBundleIdentifier`, `NSRunningApplication.localizedName`).
- Zero window title inspection (`kCGWindowName` or Accessibility attributes).
- Zero URL, tab, or document path inspection.
- Zero keystroke recording or input monitoring.
- Zero screen capture, thumbnail generation, or pixel sampling.
- Zero clipboard inspection.
- Zero telemetry, cloud sync, or external network requests.
- All session records stored exclusively in local SQLite database (`~/Library/Application Support/LifeLog/lifelog.db`).

### 23.9 Performance and Resource Measurements

Measurements performed on Apple Silicon (M4 / macOS Darwin arm64):
- **Idle CPU:** 0.0% CPU (Go daemon and Swift observer are purely event-driven via `NSWorkspace` notifications; zero busy-waiting or polling loops).
- **Go Daemon RSS:** 15.5 MB (15,536 KB).
- **Native Swift Observer RSS:** 10.6 MB (10,672 KB).
- **Total Combined RSS:** ~26.1 MB.
- **SQLite Write Latency:** 86.6 microseconds per transition write (`BenchmarkStorage_AppSessionTransition` @ 14,076 ops/sec, 752 B/op, 20 allocs/op).

### 23.10 Verification Results

All tests and automated checks passing:
1. `gofmt -l .`: Clean (0 unformatted files).
2. `go vet ./...`: Clean (0 warnings).
3. `go test -count=1 ./...`: 13 packages passed.
4. `go test -count=1 -race ./...`: 13 packages passed with zero data races.
5. `git diff --check`: Clean (0 whitespace errors).
6. Desktop TypeScript check (`npm run typecheck`): 0 errors.
7. Desktop Vitest suite (`npm test`): 9 tests passed across 3 test suites.
8. Desktop Production build (`npm run build`): Vite + TypeScript build passed cleanly in 792ms.
9. Emoji Audit: 0 emojis across all desktop UI and M3 code.

---

## 24. LIFELOG V2-M4 — System Activity and Optional Browser-Domain Tracking

### 24.1 Architecture & Native Observer Helper Integration
LIFELOG tracks system lifecycle transitions without Accessibility permissions by extending the native Swift helper (`cmd/app-observer/main.swift`, compiled to `bin/lifelog-app-observer`):
- **Sleep & Wake Notifications:** Subscribes to `NSWorkspace.willSleepNotification` and `NSWorkspace.didWakeNotification` via `NSWorkspace.shared.notificationCenter`.
- **Screen Lock & Unlock Notifications:** Subscribes to `com.apple.screenIsLocked` and `com.apple.screenIsUnlocked` via macOS `DistributedNotificationCenter.default()`.
- **Event Protocol:** Emits newline-delimited JSON payloads over standard output:
  - `{"type":"system_sleep","timestamp":"..."}`
  - `{"type":"system_wake","timestamp":"..."}`
  - `{"type":"system_lock","timestamp":"..."}`
  - `{"type":"system_unlock","timestamp":"..."}`
- **Active Session Finalization:** When receiving `system_sleep` or `system_lock`, the supervisor (`internal/apps/observer.go`) automatically finalizes any open foreground application session (`is_active = 0`, `duration_ms = end_time - start_time`), preventing inflated durations while the machine is asleep or locked. When waking or unlocking, the active foreground application is re-queried and a new session begins.

### 24.2 Database Schema (Migration 9)
Migration 9 (`v9_system_and_data_management`) establishes three tables with FTS5 search integration:
```sql
CREATE TABLE IF NOT EXISTS system_events (
    id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    metadata TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_system_events_time ON system_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_system_events_type ON system_events(event_type);

CREATE TRIGGER IF NOT EXISTS trg_system_events_ai AFTER INSERT ON system_events BEGIN
    INSERT INTO search_index (domain, entity_id, title, content, timestamp)
    VALUES ('system', NEW.id, NEW.event_type, NEW.metadata, NEW.timestamp);
END;

CREATE TRIGGER IF NOT EXISTS trg_system_events_ad AFTER DELETE ON system_events BEGIN
    DELETE FROM search_index WHERE domain = 'system' AND entity_id = OLD.id;
END;

CREATE TABLE IF NOT EXISTS browser_domains (
    id TEXT PRIMARY KEY,
    domain TEXT NOT NULL,
    browser_name TEXT NOT NULL,
    visit_time TIMESTAMP NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_browser_domains_time ON browser_domains(visit_time);
CREATE INDEX IF NOT EXISTS idx_browser_domains_domain ON browser_domains(domain);

CREATE TRIGGER IF NOT EXISTS trg_browser_domains_ai AFTER INSERT ON browser_domains BEGIN
    INSERT INTO search_index (domain, entity_id, title, content, timestamp)
    VALUES ('browser', NEW.id, NEW.domain, NEW.browser_name, NEW.visit_time);
END;

CREATE TRIGGER IF NOT EXISTS trg_browser_domains_ad AFTER DELETE ON browser_domains BEGIN
    DELETE FROM search_index WHERE domain = 'browser' AND entity_id = OLD.id;
END;

CREATE TABLE IF NOT EXISTS system_preferences (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 24.3 Browser-Domain Tracking Boundary & Status
- **Disabled by Default:** Browser tracking is strictly opt-in and stored in `system_preferences` with key `browser_tracking_enabled`.
- **Domain-Only Principle:** Records only top-level domains (e.g., `github.com`).
- **Privacy Boundaries:** Never records full URLs, paths, query strings, headers, cookies, passwords, form inputs, or browser history databases.
- **Window-Title Scraping Rejected:** Window titles were explicitly rejected as an unreliable and invasive mechanism for inferring browsing activity. In the absence of a browser extension or native messaging host, the browser source is truthfully exposed as `status: "disabled"` and `reason: "No extension installed"`.

### 24.4 Daemon IPC Methods for M4
- `getSystemEvents`: Retrieves paginated system events (`since`, `until`, `limit`, `offset`).
- `getSystemActivitySummary`: Aggregates system event counts by type (`sleep`, `wake`, `lock`, `unlock`).
- `getBrowserTrackingStatus`: Returns browser tracking state (`enabled: false`, `reason`).
- `setBrowserTracking`: Updates browser tracking preference (`enabled: true/false`).

---

## 25. LIFELOG V2-M5 — Privacy Controls, Retention, and Data Management

### 25.1 Central Privacy Controls
Configured in `desktop/src/renderer/src/views/SettingsView.tsx`:
- Controls for all 6 tracking sources: Application Usage, System Activity, Browser Domain Tracking, Filesystem Metadata, Git Activity, and Terminal Recording.
- Each source card details:
  - Current operational status (Active, Paused, Disabled).
  - Explicit list of collected fields.
  - Explicit list of non-collected fields.
  - Required macOS permissions (or confirmation that none are needed).
  - Action button to toggle, pause, or resume tracking.

### 25.2 Configurable Retention Policies
- **Retention Presets:** 7 days, 30 days, 90 days, 365 days, or Keep Forever (0 days).
- **Storage:** Persisted in `system_preferences` table under key `retention_days`.
- **Cleanup Routine (`PurgeRetention`):** Executes bounded transactions deleting expired records from `filesystem_events`, `terminal_records`, `app_sessions`, `system_events`, and `browser_domains`. Database triggers automatically cascade deletions to the FTS5 `search_index`.
- **Safety:** Never modifies migrations, database schemas, application binaries, or external directories. Updates `last_retention_cleanup` timestamp.
- **Daemon IPC Methods:**
  - `getRetentionPolicy`: Returns current retention days and last cleanup timestamp.
  - `setRetentionPolicy`: Updates retention days.
  - `runRetentionCleanup`: Triggers an immediate purge and returns deleted record count.

### 25.3 Data Inspection and Storage Measurement
- **Measurement (`getDataStats`):** Measures actual SQLite database file size on disk via `os.Stat` and queries exact record counts:
  - Application sessions count.
  - System events count.
  - Browser domains count.
  - Filesystem events count.
  - Git commits and repositories count.
  - Terminal records count.
  - Resumption checkpoints count.
- **UI Presentation:** Displays live metrics cards with database size formatted in KB/MB and total recorded events in Settings -> Data Management.

### 25.4 Portable User Data Export
- **Format:** Exported as structured JSON conforming to versioned schema `lifelog-export-v1`.
- **Payload:** Includes system metadata, configured retention policy, application sessions, system events, browser domains, filesystem events, terminal records, Git repositories, and checkpoints.
- **Security:** Contains zero credentials, tokens, environment variables, or private database internals.
- **Destination:** Handled via Electron native `dialog.showSaveDialog` and written with safe file permissions (`0600`).
- **Daemon IPC Method:** `exportUserData` retrieves the full sanitized dataset.

### 25.5 Granular Data Deletion Controls
- **Selective Deletion (`deleteActivity`):** Supports clearing records for specific domains (`all`, `apps`, `system`, `browser`, `filesystem`, `terminal`, `git`) with an optional date cutoff (`before`).
- **FTS5 Synchronization:** Automated database triggers remove associated index entries, ensuring no dangling search results remain.
- **Destructive Action Safety:** Requires explicit modal confirmation with warning before executing.

---

## 26. LIFELOG V2-M6 — Packaging, Installation, and Release Readiness

### 26.1 Packaging Architecture & Resource Bundling
LIFELOG packages into a native macOS `.app` bundle using `@electron/packager` orchestrated by `scripts/package-macos.sh`:
- **Go Daemon Binary:** Compiled with `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"` to `bin/lifelog`.
- **Swift Observer Binary:** Compiled with `swiftc -O cmd/app-observer/main.swift -o bin/lifelog-app-observer`.
- **Desktop Bundle:** Packaged with `@electron/packager` targeting `darwin-arm64`.
- **Resource Placement:** Both compiled binaries are embedded into `Contents/Resources/bin/` inside `LifeLog.app`.
- **Runtime Path Resolution (`desktop/src/main/daemon.ts`):** Detects packaged execution via `app.isPackaged` and resolves the daemon binary path from `process.resourcesPath/bin/lifelog` instead of development relative paths.

### 26.2 Distributables Produced
Generated in `desktop/dist-package/`:
1. **Application Bundle:** `LifeLog-darwin-arm64/LifeLog.app`
2. **Apple Disk Image (DMG):** `LifeLog-0.2.0-arm64.dmg` (113 MB standalone installer created via `hdiutil`).
3. **Compressed Archive (ZIP):** `LifeLog-0.2.0-macOS-arm64.zip` (112 MB portable archive).

### 26.3 Electron Security Hardening Audit
- `contextIsolation: true` enforced on main window.
- `nodeIntegration: false` enforced on main window.
- `sandbox: true` enforced on preload script.
- Strict Content Security Policy (CSP) blocking external scripts and objects.
- Minimal typed preload bridge (`window.lifelog`) exposing only validated IPC methods.
- Zero remote navigation allowed.
- Ephemeral cryptographically random handshake token required for all Unix Domain Socket frames.
- Unix Domain Socket created with restricted permissions (`0600`) in `$HOME/Library/Application Support/LifeLog/lifelog.sock`.

### 26.4 Verification Summary
- **Go Unit & Race Tests:** 13 packages passed with zero race conditions (`go test -count=1 -race ./...`).
- **Desktop Typecheck & Tests:** 9/9 Vitest tests passed, 0 TypeScript errors.
- **Packaged App Verification:** Packaged `LifeLog.app` launched, successfully spawned embedded daemon binary, connected to UDS, authenticated, and served requests.
- **Emoji Audit:** 0 emojis across all UI views, components, and documentation.



