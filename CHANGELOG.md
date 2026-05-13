# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once it reaches 1.0. Pre-1.0 releases may include breaking changes between minor versions.

## [Unreleased]

## [0.1.4] - 2026-05-13

### Fixed

- `remindb update` on Windows now uses `CREATE_NO_WINDOW` instead of `DETACHED_PROCESS` when spawning the staged installer, so the child PowerShell session can complete its host initialization. The previous flag caused detached pwsh to exit silently before the install ran.

## [0.1.3] - 2026-05-13

### Added

- `remindb update` (Windows) writes a staged PowerShell script to `%TEMP%`, spawns it detached via `CreateProcess`, and exits immediately. The detached child waits for the parent PID to exit before running `install.ps1`, which releases the executable file lock and lets the binary be replaced. Logs land in `%TEMP%\remindb-update.log`.
- `install.ps1` retries the destination copy up to 10 times with 500ms backoff. After exhausting retries it prints an actionable error pointing at the most likely cause (the `remindb-singleton` Windows Service holding the binary open) and the exact commands to fix it.

## [0.1.2] - 2026-05-13

### Fixed

- `install.ps1` no longer depends on `Get-FileHash` for SHA256 verification. The cmdlet was missing in some Windows PowerShell 5.1 sessions (stripped module autoload, ConstrainedLanguage mode). Verification now uses `[System.Security.Cryptography.SHA256]` directly — available in every supported PowerShell environment.
- `remindb update` (Windows) prefers `pwsh.exe` (PowerShell 7+) over the legacy `powershell.exe` when both are present on PATH. Falls back to `powershell.exe` if pwsh is not installed.

### Added

- `install.ps1` rejects PowerShell versions older than 5.1 up front with a clear error rather than failing partway through.

## [0.1.1] - 2026-05-13

### Added

- Windows Service support baked into the binary via `golang.org/x/sys/windows/svc`. New sub-commands: `remindb service install`, `service uninstall`, `service start`, `service stop`, `service status`. The installed service runs windowless under the Service Control Manager, defaults to auto-delayed start, and writes structured `slog` output to `C:\ProgramData\remindb\service.log`.
- `remindb serve --log-file PATH` flag — redirects slog text output to a file. Used by `service install` so the SCM-spawned `serve` (which has no stdio) can still log.
- SCM dispatch loop in `serve` honors `Stop` and `Shutdown` control requests, cancels the shared serve context, and drains rescan + temperature goroutines before reporting `Stopped`.

## [0.1.0] - 2026-05-13

### Added

- First tagged release of `remindb-Local-Hub`. Cross-compiled binaries for `linux`, `darwin`, and `windows` on `amd64` and `arm64`. Published via GoReleaser on `v*` tag push.
- `install.ps1` (Windows) and `install.sh` (Linux / macOS) on the `main` branch root. One-liner install: `irm <raw>/install.ps1 | iex` / `curl <raw>/install.sh | sh`. SHA256-verified against the release `checksums.txt`.
- `remindb update` subcommand re-runs the install script from `main` to upgrade in place.
- FTS5 query safe-quote rewrite in `pkg/store/search.go` (`rewriteQuery`). Hyphenated tokens (`LR-2026-...`, `three-tier`, `docker-compose`), slashed/dotted tokens, mixed-operator queries (`docker-compose AND configuration`, `label:snapshot LR-...`, `"quoted phrase" LR-...`), `NEAR(...)` arguments, prefix wildcards (`three-tier*`), and non-ASCII letters all route safely to FTS5 instead of tripping the bareword column-ref parser ("no such column" errors).
- TCP singleton hub: `remindb serve --listen 127.0.0.1:39291` lets one owning process hold the SQLite database while multiple MCP clients connect through `remindb bridge`, eliminating the WAL contention and duplicate-rescan cost of per-client `serve` processes.
- `remindb bridge --addr ... --db ... --source ... --rescan-interval ... --startup-timeout ...` — thin stdio MCP adapter that forwards JSON-RPC frames to the singleton over loopback TCP.
- Windows path-pin (`compressPrefix(compileRoot)`) so `MemoryTree` and path hashing render correctly under Windows-style absolute paths.
- `go.mod` pinned at `go 1.26.3` so the `setup-go@v6` `go-version-file: go.mod` flow downloads a matched toolchain on CI.

### Changed

- Switched accept-loop exit detection in `cmd/remindb/serve.go` from the deprecated `net.Error.Temporary()` to `errors.Is(err, net.ErrClosed)` — fixes staticcheck SA1019 in v0.1.0.

## Project history

`remindb-Local-Hub` is a hard fork of [`upstream/remindb`](https://github.com/special-place-administrator/remindb-Local-Hub) maintained as an independent project. See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for scope and how contributions are handled.

[Unreleased]: https://github.com/special-place-administrator/remindb-Local-Hub/compare/v0.1.4...HEAD
[0.1.4]: https://github.com/special-place-administrator/remindb-Local-Hub/releases/tag/v0.1.4
[0.1.3]: https://github.com/special-place-administrator/remindb-Local-Hub/releases/tag/v0.1.3
[0.1.2]: https://github.com/special-place-administrator/remindb-Local-Hub/releases/tag/v0.1.2
[0.1.1]: https://github.com/special-place-administrator/remindb-Local-Hub/releases/tag/v0.1.1
[0.1.0]: https://github.com/special-place-administrator/remindb-Local-Hub/releases/tag/v0.1.0
