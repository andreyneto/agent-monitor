# agent-monitor — details

Reference for how `mon` works and everything you can configure. For the intent
and quick start, see the [README](../README.md).

## How it works

Claude Code fires **hooks** on session events. Each hook pipes a small JSON
payload to `mon emit`, which appends a line to
`~/.claude/monitor/events.jsonl`. The panel (`mon`) tails that file and renders
the current state. It works for **every** Claude Code session, in any terminal —
including ones running in the background.

| Claude Code hook   | Panel state          |
|--------------------|----------------------|
| `SessionStart`     | starting             |
| `UserPromptSubmit` | working              |
| `Notification`     | **needs you** (alarm)|
| `Stop`             | done                 |
| `SessionEnd`       | (removed from panel) |

`mon install` wires these hooks into `~/.claude/settings.json` (writing a backup
first). Restart your Claude Code sessions afterwards.

## Keys

| Key | Action |
|-----|--------|
| `?` | help (shortcuts and state) |
| `v` | cycle layout (list → grid → auto) |
| `↑` `↓` | scroll the list / grid |
| `s` | settings screen (editable, saved on exit) |
| `m` | mute / unmute the sound |
| `f` | filter (all ↔ only "needs you") |
| `r` | reload now |
| `c` | clear idle sessions (including ones stuck on "needs you") |
| `q` / `esc` / `ctrl+c` | quit |

On the settings screen: `↑↓` navigate, `←→`/space change a value, `s`/`esc` save
and go back.

## Panel features

- **Auto layout.** With one session it fills the screen as a single card; 2–3 in
  one column; 4–6 in two columns (up to 3 cards per column) until it runs out of
  width, then it falls back to a scrollable list.
- **A color per session,** derived from a hash (FNV-1a) of the session id, so
  each agent keeps a stable, distinct color and is easy to tell apart.
- **Account quota bars.** Two animated rows — current session (5h) and weekly
  (7d) — with a bar, %, and reset time. The color shifts with usage (orange
  `<70%`, amber `70–90%`, red `≥90%`), and slides toward the value with a
  gradient inside the band's color.
- **Usage overview when idle.** With no active sessions, the screen shows your
  overall Claude Code usage (read from `~/.claude/stats-cache.json`, the same
  data as `/usage`): a GitHub-style activity heatmap plus stats — favorite
  model, total tokens, sessions, longest session, active days, longest streak,
  most active day, and current streak. It adapts to the screen size.

## Configuration

Preferences live in `~/.claude/monitor/config.json` (created when you save from
the settings screen). It can be edited by hand:

```json
{
  "sound": true,
  "sound_path": "/System/Library/Sounds/Glass.aiff",
  "stale_minutes": 45,
  "show_quota": true,
  "blink": true,
  "clock_24h": true,
  "layout": "auto",
  "grid_cols": 2,
  "spinner_style": "minidot",
  "only_attention": false
}
```

Everything here is persisted across runs — including the filter (`f`) and the
spinner style.

- `spinner_style` (the "working" indicator) accepts `minidot`, `pulse`, `line`,
  `jump`, or `dot` — all with a stable width so the card borders don't wobble.
- `layout` accepts `list` (the plain list), `grid` (fixed `grid_cols` columns,
  1–6), or `auto` (see Panel features above).

### Environment

| Variable     | Effect                                                        |
|--------------|---------------------------------------------------------------|
| `MON_DIR`    | events folder (default `~/.claude/monitor`)                   |
| `MON_SOUND`  | alarm sound (default `/System/Library/Sounds/Glass.aiff`)     |
| `MON_SILENT` | if set, disables the sound (the terminal bell still fires)    |

`MON_SOUND` / `MON_SILENT` override the config file at boot.

## Account quota (plan limits)

These numbers **aren't stored in a file** — they only arrive live in the
`statusLine` input. So `~/.claude/statusline.sh` is instrumented with one line
that mirrors the input to `mon quota` (in the background, without affecting the
status line output):

```bash
printf '%s' "$input" | /path/to/mon quota >/dev/null 2>&1 &
```

`mon quota` writes just `rate_limits` to `~/.claude/monitor/quota.json`
(overwrite, no bloat). Real schema: each window (`five_hour`, `seven_day`) has
`used_percentage` (0–100) and `resets_at` (Unix epoch). It only shows up for
subscribers, after the first reply.

## Other tools (Cursor, Gemini, …)

`mon emit` is generic: anything that can run a command can push an event. Just
send JSON on stdin:

```bash
echo '{"session":"x","project":"cursor-app","kind":"attention","message":"review"}' | mon emit
```

`kind` accepts: `start`, `working`, `attention`, `done`, `end`.

## Notification classification

Claude fires `Notification` for different things. `mon` tells them apart:

- `"...needs your permission"` → **needs you** (alarm)
- `"...waiting for your input"` → **done / idle** (just waiting, no alarm)

Without this, idle sessions would get stuck showing "needs you".
