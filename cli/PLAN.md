# CLI Plan: `remove-comments`

A Go CLI tool that removes all comments from every source file in a directory tree,
respecting `.gitignore` rules. The companion to the `nvim-remove-comments` Neovim plugin.

---

## Goals

- Walk an entire project directory and strip comments from every supported source file
- Respect `.gitignore` at every level (root, nested subdirectories, `.git/info/exclude`)
- Default to dry-run: show a diff preview, require `--write` to modify files
- Process files in parallel for speed on large repos
- Zero false positives: use Tree-sitter AST parsing, not regex
- Single static binary, zero runtime dependency for end users

---

## Non-Goals

- Selective comment preservation (e.g., keep `TODO`, keep license headers) — not in v1
- Watching files for changes
- Modifying `plugin/` or `lua/` — the Neovim plugin is untouched

---

## Technology Choices

| Concern | Choice | Reason |
|---|---|---|
| Language | Go | Single static binary, native concurrency, zero runtime for users |
| Tree-sitter binding | `smacker/go-tree-sitter` | Bundles many language grammars as CGO sub-packages, no separate grammar fetch step |
| gitignore parsing | `boyter/gocodewalker` | Handles nested `.gitignore`, `.git/info/exclude`, proven in production tooling |
| CLI framework | `spf13/cobra` | Industry standard, clean subcommand/flag model |
| Colored output | `fatih/color` | Lightweight, respects `NO_COLOR` env var |
| Diff output | stdlib `strings` comparison | No external diff library needed; line-by-line comparison is sufficient |

---

## Directory Layout

```
cli/
├── main.go
├── go.mod
├── go.sum
├── cmd/
│   └── root.go
└── internal/
    ├── languages/
    │   └── languages.go
    ├── walker/
    │   └── walker.go
    ├── parser/
    │   └── parser.go
    ├── remover/
    │   └── remover.go
    ├── diff/
    │   └── diff.go
    └── output/
        └── output.go
```

### File Responsibilities

**`main.go`**
Entry point. Calls `cmd.Execute()`. Nothing else lives here.

**`cmd/root.go`**
Cobra command definition. Declares all flags. Wires together walker → parser → remover → output.
Contains the top-level `run()` function that orchestrates the pipeline.

**`internal/languages/languages.go`**
A static lookup table: file extension → `LangConfig{ Name, Query }`.
This mirrors `lua/nvim-remove-comments/config.lua` exactly.
Any new language added here must also be added to the Lua config, and vice versa.
Also provides `Names()` (all language names, sorted), `ValidName`, `ExtensionsFor`,
and `LangFilter` for multi-language filtering with validation.

**`internal/generated/generated.go`**
Generated-file detection. Two strategies:
filename patterns (`*.g.dart`, `*.pb.go`, `*_pb2.py`, `*.gen.ts`, `*.min.js`, ...)
and content markers (`code generated` / `do not edit` in the first 512 bytes).
Disabled with `--no-skip-generated`.

**`internal/config/config.go`**
Loads `.remove-comments.yaml` (discovered by searching upward from the target
path, or passed explicitly via `--config`). Fields: `exclude`, `include`,
`langs`, `skip-generated`, `max-file-size`. CLI flags override config values.

**`internal/walker/walker.go`**
Accepts a root path and an `Options` struct (`Langs`, `MaxFileSize`,
`Exclude`, `Include`). Uses `gocodewalker` to walk the directory tree,
respecting all `.gitignore` files found at each level. Filters results to
only files whose extension is in the `languages` table, whose language passes
the `Langs` filter (empty = all), matching at least one `Include` glob (when
set), and not matching any `Exclude` glob. Glob matching uses `doublestar`
(supports `**`) against basename, full path, and path relative to the walk
root. Returns a slice of `FileEntry`.

**`internal/parser/parser.go`**
Accepts a `FileEntry` and the raw file bytes. Initializes the correct Tree-sitter
language parser. Runs the S-expression query to find all `@comment` nodes.
Returns a `[]CommentRange` describing every comment's position.

**`internal/remover/remover.go`**
Accepts the file's lines and a `[]CommentRange`. Applies the removal logic.
Returns the cleaned lines. Pure function — no I/O, no side effects.

**`internal/diff/diff.go`**
Accepts original lines and cleaned lines. Produces a `[]DiffLine` (tagged as
removed, kept, or context). Used by the output layer to render the preview.

**`internal/output/output.go`**
Renders results to stdout. Handles both dry-run (diff preview) and write (summary)
modes. All terminal formatting lives here.

---

## Data Flow

```
root path (CLI arg)
    │
    ▼
walker.Walk(root)
    │  respects .gitignore at every dir level
    │  filters by known file extension
    ▼
[]FileEntry{ path, lang, extension }
    │
    ▼  (worker pool, N goroutines)
    │
    ├─▶ read file bytes from disk
    │
    ├─▶ parser.Parse(bytes, langConfig)
    │       └─▶ returns []CommentRange
    │
    ├─▶ remover.Remove(lines, []CommentRange)
    │       └─▶ returns cleanedLines
    │
    └─▶ diff.Compute(originalLines, cleanedLines)
            └─▶ returns []DiffLine
    │
    ▼
collect []FileResult (sorted by path for deterministic output)
    │
    ▼
output.Render([]FileResult, flags)
    │
    ├─▶ dry-run: print colored diff per file + summary
    └─▶ --write: overwrite files on disk + print summary
```

---

## Core Types

### `FileEntry`
Represents a file discovered by the walker that the tool can process.

Fields:
- `Path` — absolute path to the file
- `Lang` — language name (matches key in `languages.go`)
- `Extension` — file extension

### `LangConfig`
Represents the Tree-sitter configuration for one language.

Fields:
- `Name` — language name string
- `Query` — Tree-sitter S-expression query string (e.g., `(comment) @comment`)
- `Parser` — the Tree-sitter language parser function pointer

### `CommentRange`
Represents a single comment node found by Tree-sitter.

Fields:
- `StartRow`, `StartCol` — zero-indexed start position
- `EndRow`, `EndCol` — zero-indexed end position
- `IsFullLine` — true when the comment occupies the entire line (col 0 to end)
- `IsMultiLine` — true when `StartRow != EndRow`

### `DiffLine`
Represents one line in a diff preview.

Fields:
- `LineNo` — original line number (1-indexed for display)
- `Content` — the line text
- `Type` — enum: `Removed`, `Kept`, `Context`

### `FileResult`
The complete result of processing one file.

Fields:
- `Entry` — the original `FileEntry`
- `OriginalLines` — lines before removal
- `CleanedLines` — lines after removal
- `Diff` — `[]DiffLine`
- `CommentsRemoved` — count of comments removed
- `Changed` — whether any comments were found

---

## Removal Logic (inside `remover.go`)

This ports the exact logic from `lua/nvim-remove-comments/core.lua`:

1. Iterate over all `CommentRange` entries.
2. For a **single-line, full-line comment** (`IsFullLine == true`): mark the row index
   for deletion in a set.
3. For a **single-line, inline comment** (`IsFullLine == false`, `StartRow == EndRow`):
   splice the line — keep everything before `StartCol`, discard from `StartCol` to
   `EndCol`, keep everything after `EndCol`. Trim trailing whitespace on the spliced line.
4. For a **multi-line comment** (`IsMultiLine == true`): mark every row from `StartRow`
   to `EndRow` inclusive for deletion.
5. Collect all marked rows into a slice. Sort descending (largest row first). Delete
   bottom-up. This prevents row-index shifting during batch deletion — same technique
   as the Lua plugin.
6. After deletion, strip any sequences of more than one consecutive blank line left
   behind (collapse to at most one blank line between blocks).

The result is the cleaned `[]string` of lines.

---

## Walker Behavior

- Starts from the path given as the CLI argument (defaults to `.`)
- Uses `gocodewalker` which natively handles:
  - Root `.gitignore`
  - Nested `.gitignore` files in subdirectories
  - `.git/info/exclude`
  - Always skips `.git/` itself
- Additionally skips: binary files, files larger than 10 MB (configurable via flag),
  and symlinks to directories (to avoid cycles)
- Only yields files whose extension maps to a language in `languages.go`
- Skips files it cannot read (logs a warning, continues)

---

## Language Support

Mirrors the Neovim plugin's `config.lua` exactly. Supported languages at launch:

| Language | Extensions | Comment Node Types |
|---|---|---|
| JavaScript | `.js`, `.mjs`, `.cjs` | `(comment)` |
| JSX | `.jsx` | `(comment)` |
| TypeScript | `.ts` | `(comment)` |
| TSX | `.tsx` | `(comment)` |
| Lua | `.lua` | `(comment)` |
| Python | `.py` | `(comment)` |
| Go | `.go` | `(comment)` |
| Java | `.java` | `(line_comment)`, `(block_comment)` |
| C | `.c`, `.h` | `(line_comment)`, `(block_comment)` |
| C++ | `.cpp`, `.cc`, `.cxx`, `.hpp` | `(line_comment)`, `(block_comment)` |
| Rust | `.rs` | `(line_comment)` |
| HTML | `.html`, `.htm` | `(comment)` |
| CSS | `.css` | `(comment)` |
| YAML | `.yaml`, `.yml` | `(comment)` |
| TOML | `.toml` | `(comment)` |
| Bash/Shell | `.sh`, `.bash` | `(comment)` |
| Dart | `.dart` | `(comment)`, `(documentation_comment)` |

---

## CLI Interface

**Binary name:** `remove-comments`

**Usage:**
```
remove-comments [path] [flags]
```

**Arguments:**

| Argument | Default | Description |
|---|---|---|
| `path` | `.` | Root directory to scan recursively |

**Flags:**

| Flag | Short | Default | Description |
|---|---|---|---|
| `--write` | `-w` | `false` | Write changes to disk. Without this flag, the tool only previews. |
| `--quiet` | `-q` | `false` | Suppress per-file diff output. Print only the final summary line. |
| `--lang` | | `""` | Process only files of the specified languages, comma-separated (e.g., `--lang go,java`). |
| `--include` | | | Glob patterns; when set, only matching files are processed (e.g., `src/**`). |
| `--no-skip-generated` | | `false` | Do not skip generated files. |
| `--config` | | | Path to a config file (default: search for `.remove-comments.yaml`). |
| `--no-config` | | `false` | Do not load a config file. |
| `--list-langs` | | `false` | List supported languages and exit. |
| `--jobs` | `-j` | `NumCPU` | Number of parallel workers. |
| `--max-file-size` | | `10485760` | Skip files larger than this byte size (default 10 MB). |
| `--version` | `-v` | | Print version string and exit. |

**Exit codes:**

| Code | Meaning |
|---|---|
| `0` | Success (dry-run: changes previewed; write: changes applied) |
| `1` | Fatal error (path not found, permission denied, etc.) |
| `2` | No supported files found in the given path |

---

## Output Format

### Dry-run mode (default)

```
src/main.go
  - 12 | // initialize the database connection
  - 34 | /* legacy block comment
  - 35 |    continued here */
  3 comments removed  (dry-run)

src/utils.ts
  - 7  | // helper function
  1 comment removed  (dry-run)

──────────────────────────────────────
2 files · 4 comments removed
Run with --write to apply changes.
```

- Removed lines are shown in red with a `-` prefix and the original line number
- Files with zero comments are not printed
- A horizontal rule separates per-file output from the summary

### Write mode (`--write`)

```
src/main.go     3 comments removed
src/utils.ts    1 comment removed

──────────────────────────────────────
2 files · 4 comments removed
```

- No diff shown, just a per-file count and the summary
- Files are listed in alphabetical order (sorted by path)

### Quiet mode (`--quiet`)

```
2 files · 4 comments removed
```

Only the summary line. Works in both dry-run and write modes.

### No changes found

```
No comments found. Nothing to do.
```

---

## Concurrency Model

- The walker produces `FileEntry` values on a buffered channel
- A fixed worker pool (default: `runtime.NumCPU()` goroutines) reads from the channel
- Each worker independently: reads the file, parses, removes, computes diff
- Results are sent to a results channel and collected into a `[]FileResult`
- After all workers finish, results are sorted by `Entry.Path` for deterministic output
- A `sync.WaitGroup` coordinates worker shutdown
- Errors from individual files do not abort other workers; they are collected and
  printed at the end as warnings

---

## Error Handling

- Path does not exist → fatal error, exit code 1, clear message
- File unreadable (permission denied) → warning printed, file skipped, processing continues
- Tree-sitter has no parser for a language → should never happen (walker filters by known
  extensions), but if it does: skip with warning
- Tree-sitter parse error (malformed file) → skip the file with a warning, do not write
  partial results
- Write failure (disk full, permission denied on write) → fatal error for that file,
  print error, continue with remaining files, exit code 1 at the end

---

## Testing Strategy

All tests live alongside their source file (`walker_test.go` next to `walker.go`).
Table-driven tests for all functions with multiple input/output cases.
No external test frameworks.

**`walker_test.go`**
- Creates a temporary directory tree with a mix of supported and unsupported file types
- Writes a `.gitignore` that excludes some files
- Asserts that the walker returns only the expected set of files

**`parser_test.go`**
- For each supported language, provides a small source file string containing known
  comments
- Asserts that `Parse` returns the exact expected `[]CommentRange` (correct rows, cols,
  `IsFullLine`, `IsMultiLine` values)

**`remover_test.go`**
- Table-driven: input lines + `[]CommentRange` → expected output lines
- Covers: full-line single comment, inline comment, multi-line block comment,
  mixed inline and full-line in one file, consecutive blank lines after removal,
  empty file, file with no comments

**`diff_test.go`**
- Verifies `DiffLine` output for known before/after line sets

**`output_test.go`**
- Captures stdout and asserts the formatted output matches expected strings for
  dry-run mode, write mode, and quiet mode

**Integration test** (in `cmd/` or a separate `testdata/` directory):
- A real directory tree with several files in different languages, each containing
  known comments, plus a `.gitignore`
- Run the CLI in dry-run mode, assert stdout matches expected diff output
- Run the CLI in write mode, assert files on disk match expected cleaned content

---

## Build & Release

- `go build ./...` produces the binary
- `go install github.com/KashifKhn/nvim-remove-comments/cli@latest` for users with Go installed
- Cross-compilation targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
  `windows/amd64`
- No Makefile for v1 — plain `go build` is sufficient

---

## Implementation Order

1. `go.mod` initialization and dependency pinning
2. `internal/languages/languages.go` — language table, no logic
3. `internal/walker/walker.go` + `walker_test.go`
4. `internal/parser/parser.go` + `parser_test.go`
5. `internal/remover/remover.go` + `remover_test.go`
6. `internal/diff/diff.go` + `diff_test.go`
7. `internal/output/output.go` + `output_test.go`
8. `cmd/root.go` — Cobra wiring, worker pool, pipeline
9. `main.go` — entry point
10. Integration test
11. Update root `AGENTS.md` if any conventions deviate from the plan

---

## Out of Scope for v1

- ~~Config file support (`.removecommentsrc`)~~ — done: `.remove-comments.yaml`
- ~~`--ignore` flag for additional ignore patterns beyond `.gitignore`~~ — done: `--exclude`
- ~~Selective processing~~ — done: `--include`, multi-lang `--lang`
- ~~Generated-file skipping~~ — done: `internal/generated`
- Preserving specific comment patterns (license headers, `TODO`, `FIXME`)
- `--stdin` / `--stdout` single-file pipe mode
- Shell completion (`cobra` has this built-in, can be added trivially later)
- Homebrew formula / package manager distribution
