# remindb for Claude Code

Drops [remindb Local Hub](https://github.com/special-place-administrator/remindb-Local-Hub) into Claude Code as an MCP server. The agent picks up the full `Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

Claude Code loads `.claude-plugin/plugin.json` as the plugin manifest and merges `.mcp.json` into its effective MCP server list. When Claude Code starts with the plugin enabled, it spawns `remindb bridge` over stdio. The bridge connects to one singleton local `remindb serve --listen` process so several agents can share the same `.db` safely.

Tools are namespaced, so `MemoryFetch` shows up as `remindb__MemoryFetch` in the tool list.

## Installation

### 1. Install the remindb binary

It needs to be on `$PATH`. Until this fork publishes releases, build it from source:

```bash
git clone https://github.com/special-place-administrator/remindb-Local-Hub.git
cd remindb-Local-Hub
go test ./...
go build -o ~/.local/bin/remindb ./cmd/remindb
```

On Windows:

```powershell
git clone https://github.com/special-place-administrator/remindb-Local-Hub.git
cd remindb-Local-Hub
go test ./...
go build -o .\remindb.exe .\cmd\remindb
$installDir = "$env:LOCALAPPDATA\Programs\remindb\bin"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Copy-Item .\remindb.exe "$installDir\remindb.exe" -Force
```

Verify:

```bash
remindb --version
```

### 2. Compile a source directory

remindb needs a SQLite file built from a source tree before the agent can read from it.

A natural source for Claude Code is its own per-project memory at `~/.claude/projects/<project>/memory/` — those markdown files Claude has been quietly accumulating about each repo it works in. Indexing them across all projects lets Claude query its own persistent memory through remindb instead of grepping the dot folder.

`~/.claude/projects/<project>/` sits next to a few other artifacts that don't belong in long-term memory: session-log `.jsonl` files, plus `subagents/` and `tool-results/` subtrees under each session UUID directory. Drop a `.remindb.ignore` at `~/.claude/projects/` to filter them out, so the only thing the compiler ingests is `memory/*.md` per project:

```bash
mkdir -p ~/.cache/remindb
printf '%s\n' \
    '# Compile only per-project memory/ markdown; skip the surrounding telemetry.' \
    '' \
    '# Session logs (large, low value).' \
    '*.jsonl' \
    '# Per-session subagent traces (any depth).' \
    'subagents/' \
    '# Per-session tool outputs (any depth).' \
    'tool-results/' \
    > ~/.claude/projects/.remindb.ignore
remindb compile ~/.claude/projects --db ~/.cache/remindb/claude.db
```

The same `.remindb.ignore` is honored by `serve`'s background rescan and the `MemoryCompile` tool — set it once, all paths agree. If Claude Code adds a new sibling-of-`memory/` artifact in some future release, append it to the file and recompile. Or point at any other workspace you want the agent to see — a docs tree, a notes repo, a project directory. Re-run `compile` whenever you want a fresh baseline; `serve` keeps the DB current after that.

### 3. Point remindb at your workspace

`remindb bridge` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. The bundled `.mcp.json` declares both as `${VAR}` passthroughs into the spawned subprocess, so export them in the shell **before launching Claude Code with the plugin enabled** — otherwise the first activation falls back to a stray `memory.db` in cwd:

```bash
export REMINDB_DB=$HOME/.cache/remindb/claude.db
export REMINDB_SOURCE=$HOME/.claude/projects
export REMINDB_RESCAN_INTERVAL=60s
```

Stick them in `~/.bashrc` / `~/.zshrc` / your fish equivalent to make the mapping permanent, or scope them to a single session if you want to switch workspaces between runs. Undefined `${VAR}` references resolve to empty strings, which is what triggers the cwd fallback.

### 4. Install the plugin

Pick one:

**Local checkout**:

```bash
claude --plugin-dir ./plugins/claude-code
```

There is no Local Hub marketplace package yet. Do not install `radimsem/remindb` from the marketplace if you need `remindb bridge`; that installs the upstream stdio-only server config.

Either way, confirm the server is connected:

```
/mcp
```

You should see `remindb` listed with the full `Memory*` tool suite.

A same-named server in user-scope `~/.claude.json` *replaces* the plugin's bundled entry per Claude Code's MCP precedence rules (it does not merge), so don't try to inject env there.

#### Seed remaining context

Step 2 compiled `~/.claude/projects/` — Claude's cross-project memory. The current project's `CLAUDE.md` and in-repo docs (`README.md`, design notes) live in the repo, not under that path. Ask Claude in your first session to fold them in. Use absolute paths — `MemoryCompile` doesn't expand `~`:

```
remindb__MemoryCompile(path="/home/you/code/my-project/CLAUDE.md", message="seed: project rules")
remindb__MemoryCompile(path="/home/you/code/my-project/README.md", message="seed: project overview")
```

Re-run whenever a file changes.

## Configuration

The plugin itself has no runtime options. `remindb bridge` resolves its DB and source paths from `REMINDB_DB` and `REMINDB_SOURCE` at launch, then starts or connects to the singleton local server.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
