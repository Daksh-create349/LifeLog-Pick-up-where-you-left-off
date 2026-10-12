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






