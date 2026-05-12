# ReminDB-Local-Hub for OpenCode

Drops [ReminDB-Local-Hub](https://github.com/special-place-administrator/remindb-Local-Hub) into OpenCode as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

OpenCode configures MCP servers in `opencode.json` under the top-level `mcp` object rather than via the plugin API. This folder ships:

- `opencode.json` — a ready-to-merge MCP entry that spawns `remindb bridge` over stdio.
- `plugin.ts` — a minimal OpenCode plugin stub so the bundle can be distributed as an npm package for users who prefer that path.

## Installation

### 1. Install the ReminDB-Local-Hub binary

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

### 2. Compile your workspace

remindb needs a SQLite file built from a source tree before the agent can read from it. The source is whatever workspace you want OpenCode to remember — a code repo, a docs tree, a notes directory.

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/code/my-project --db ~/.cache/remindb/my-project.db
```

Drop a `.remindb.ignore` at the workspace root if you need to exclude noise (build outputs, vendored deps, generated files). The same file is honored by `serve`'s background rescan and the `MemoryCompile` tool.

### 3. Configure opencode.json

`remindb bridge` reads `REMINDB_DB` and `REMINDB_SOURCE` for its `--db` and `--source` flags, then starts or connects to one singleton local `remindb serve --listen` process. The cleanest place to set them for OpenCode is the `environment` object on the `mcp.remindb` entry — OpenCode passes it straight to the spawned subprocess without touching your shell env. The full config looks like this:

```json
{
    "$schema": "https://opencode.ai/config.json",
    "mcp": {
        "remindb": {
            "type": "local",
            "command": ["remindb", "bridge"],
            "environment": {
                "REMINDB_DB": "{env:HOME}/.cache/remindb/my-project.db",
                "REMINDB_SOURCE": "{env:HOME}/code/my-project",
                "REMINDB_RESCAN_INTERVAL": "60s"
            },
            "enabled": true
        }
    }
}
```

Pick one install path:

**Project-level** (recommended — one workspace per repo):

```bash
curl -fsSL https://raw.githubusercontent.com/special-place-administrator/remindb-Local-Hub/main/plugins/opencode/opencode.json \
    -o opencode.json
```

**Global** (applies to every OpenCode session):

```bash
mkdir -p ~/.config/opencode
curl -fsSL https://raw.githubusercontent.com/special-place-administrator/remindb-Local-Hub/main/plugins/opencode/opencode.json \
    -o ~/.config/opencode/opencode.json
```

The bundled file ships only the bare MCP entry — open it after curling and add the `environment` block from the snippet above. Or skip the curl and write the full snippet by hand into either path.

Heads up: OpenCode only expands `{env:VARIABLE_NAME}` in config values — shell-style `$HOME` or `${HOME}` is treated as a literal string and won't work. Swap the paths for a different workspace (e.g., `{env:HOME}/notes` + `{env:HOME}/.cache/remindb/notes.db`) whenever you want OpenCode to read a different tree. Per-project is recommended so each workspace carries its own DB and source paths — OpenCode reads `opencode.json` on session start, so launching a fresh session from the new directory is enough to swap configs.

**Optional — npm-distributed plugin stub.** ReminDB-Local-Hub does not publish an npm package yet. Do not use the upstream npm package if you need `remindb bridge`.

**Prefer a shell-inherited env?** Point the two values at your own env vars via the same substitution:

```json
"environment": {
    "REMINDB_DB": "{env:REMINDB_DB}",
    "REMINDB_SOURCE": "{env:REMINDB_SOURCE}"
}
```

Then export the pair in `~/.bashrc` / `~/.zshrc` / your fish equivalent and restart OpenCode from that shell.

Confirm the server is connected:

```bash
opencode mcp list
```

You should see `remindb` listed with the full `Memory*` tool suite.

#### Seed remaining context

OpenCode doesn't keep a `memory/` folder — its persistent context is a stack of `AGENTS.md` files loaded from three places: the global `~/.config/opencode/AGENTS.md`, project-root and ancestor `AGENTS.md` files traversed upward from your cwd, and a Claude Code fallback at `~/.claude/CLAUDE.md` (unless disabled). Only `AGENTS.md` files at or below the workspace root land in `REMINDB_SOURCE` automatically — ancestors above it and the global file live outside.

Ask OpenCode in your first session to fold them in. Use absolute paths — `MemoryCompile` doesn't expand `~`:

```
remindb__MemoryCompile(path="/home/you/.config/opencode/AGENTS.md", message="seed: global memory")
remindb__MemoryCompile(path="/home/you/code/parent/AGENTS.md", message="seed: ancestor memory")
remindb__MemoryCompile(path="/home/you/.claude/CLAUDE.md", message="seed: claude-code fallback")
```

Re-run whenever a file changes.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](../../README.md#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as ReminDB-Local-Hub.
