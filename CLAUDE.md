# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`agent-monitor` (binary: **`mon`**) is a terminal dashboard that shows your background AI-agent sessions live on a second screen. Claude Code fires hooks as sessions start/work/need-you/finish; the panel reads those events and beeps + flashes a card red when a session needs attention.

Personal project tuned for a specific 48×18 second screen (cool-retro-terminal). **The panel UI text is in Portuguese** (labels, help, settings, notices) and the code comments are in Portuguese — match this when editing user-facing strings and comments. macOS-only (uses `afplay` for the alarm).

## Commands

```bash
go build -o mon ./cmd/mon    # build
go test ./...                # run all tests
go test ./... -run TestName  # run a single test (e.g. -run TestDeriveSessions)
go vet ./...                 # vet

./mon                        # open the TUI panel
./mon test                   # inject sample sessions into the log to see it work
./mon install                # wire hooks into ~/.claude/settings.json (writes a backup)
./mon help                   # usage
```

Subcommands (`cmd/mon/main.go` dispatches on `os.Args[1]`): `emit`, `quota`, `install`, `test`, `tui`/`run`, `help`. No args → runs the TUI.

## Architecture

All Go sources live in `cmd/mon/` as a single `package main` (repo root holds only `go.mod`, docs, and assets). The core is **event sourcing**: an append-only log reduced to current state.

**Data flow.** Claude Code hooks pipe a JSON payload on stdin to `mon emit` (`emit.go`), which normalizes it into an `Event` and appends one JSON line to `~/.claude/monitor/events.jsonl` (`event.go: appendEvent`). The TUI (`mon`) tails that file every 150ms, calls `deriveSessions` to reduce the log to a `map[session]→*Session` (last event wins; `KindEnd` deletes), and renders. Any tool that can run a command can push events — `emit` is generic; Claude Code is just one source.

**Key files:**
- `event.go` — the `Event`/`Session`/`Kind` model, the events.jsonl log (append + read + tail), and `deriveSessions` (log → current state).
- `emit.go` — hook payload parsing and `mapHookKind` (Claude hook name → our `Kind`). Contains two important special cases (see below).
- `tui.go` — the Bubble Tea `model`: `Update` (150ms `tickMsg` drives `reload()`), `View`, key handling, alarm logic, quota bar rendering. This is the biggest file.
- `grid.go` — the list/grid/auto body layout and card rendering; scrolling.
- `components.go` — the bubbles spinner and animated quota progress bars (gradient bands by threshold).
- `quota.go` — `mon quota` ingests the statusLine's `rate_limits` (written by an instrumented `~/.claude/statusline.sh`, overwrite not append) into `quota.json`; `readQuota` normalizes it. Deliberately schema-tolerant because the rate_limits layout varies across Claude Code versions.
- `usage.go` — the idle-screen "usage overview" parsed from `~/.claude/stats-cache.json` (same data as `/usage`); cached by mtime.
- `config.go` — `~/.claude/monitor/config.json` preferences, merged over defaults, atomic save.
- `install.go` — idempotently injects `mon emit` hooks into settings.json for the events in `hookEvents`.

**State machine (`Kind`):** `start` → `working` → `attention` (needs you, alarm) / `background` (responded but a shell/subagent still runs) / `done` → `end`. `pr()` in tui.go sets display priority (attention first, then working, background, done). Everything except `working` and `background` ages out after `StaleMinutes`; long silent `working`/`background` tasks never disappear (both mean active work).

### Non-obvious behaviors — preserve these when editing

- **Not every `Notification` is urgency.** `isIdleWaiting` ("waiting for your input") means the session is idle waiting for you (fires after `Stop`) → treated as `done`, no alarm. Only "needs your permission"-style notifications alarm. Handled in both `emit.go` and `deriveSessions`/`curedKind` (the latter "cures" old events already in the log).
- **`PostToolUse` is dropped unless it unblocks an `attention`.** It fires per tool call and would flood the log. `emit.go` only records it (as `working`) when the session's last kind was `attention` — the case where you granted permission and Claude resumed with no `UserPromptSubmit`. `lastKindForSession` reads only the tail ~64KB for this, falling back to a full read only when the session's last event is older than the tail window.
- **Background tasks come from the hook payload.** `Stop`/`SubagentStop` carry `background_tasks[]` (running shells/subagents). At `Stop` with tasks still running, the session becomes `background` (not `done`) and stores the descriptions in `Event.BgTasks`. `SubagentStop` refreshes the count *only* if the session is already idle (`done`/`background`) — otherwise it doesn't interfere with active work; when the last task clears it falls back to `done`. Note: a background *shell* finishing may fire no hook, so a `background` card can linger until the next event in that session (or manual `c`).
- **Empty-view numbers come straight from `stats-cache.json` (the `/usage` cache), which Claude only recomputes when you open `/usage`.** mon reads it faithfully but can't force a recompute, so `usage.go` exposes `LastComputed` and the idle screen shows a "dados até DD mmm · abra /usage" note when the cache is older than today. Numbers stay identical to `/usage`; the note flags staleness rather than inventing data.
- **Alarm fires once per new attention.** `model.rung` tracks which sessions already rang; leaving `attention` clears the flag so it can ring again later.
- **`emit` never returns a fatal error** — a hook must not break the user's session; write failures are swallowed.

## Testing

Plain Go tests, table-driven, no external harness. Tests cover the pure logic — `deriveSessions`/reload, quota parsing, usage stats, grid layout math — not the rendered TUI. When adding behavior to these, add a case rather than a new test file.
