---
name: ccreel
description: Look up past Claude Code sessions with the ccreel CLI. Use when the user asks to continue earlier work ("pick up where the last session left off", "read the previous session"), asks whether something was already discussed or decided ("didn't we decide this before?"), or asks what was worked on today or recently.
---

# ccreel

`ccreel` reads Claude Code's own session files and prints them as JSON Lines. Use it instead of
opening `~/.claude*/projects/**/*.jsonl` directly: the raw files are large and mostly tool output.

- It reads the config dir in `CLAUDE_CONFIG_DIR` (or `~/.claude`), the same one this session uses.
- The session you are running in is left out of `list`, `search` and `pick` (it is identified by
  `CLAUDE_CODE_SESSION_ID`), so "the newest session" is the previous one.
- `<id>` accepts any unique prefix of the session UUID; the first 8 characters are enough.
- Always pass `--json`. When output is cut short, a line starting with `ccreel:` on stderr says
  what was dropped and which flag widens it.

## Continue the previous session in this repo

```sh
repo=$(basename "$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")")
ccreel list --repo "$repo" --limit 3 --json
ccreel outline <id> --json          # one line per turn: what was asked, what was answered
ccreel show <id> --last 3 --json    # the last turns in full, without tool calls
```

Read the outline first to see where the session ended, then `show` only the turns you need.
`--repo` is an exact name. `--git-common-dir` gives the main checkout even inside a worktree, and
sessions started in a worktree are listed under the repo they belong to.

## Find the sessions for an issue

```sh
ccreel list --issue 417 --json
```

`issue` comes from the branch or worktree name (`task/417-...`, `.claude/worktrees/417-...`).
Sessions on `main` or on branches without a leading number have no `issue`.

## Check whether a topic was discussed or decided

```sh
ccreel search "<phrase>" --since 30d --limit 20 --json
ccreel show <id> --turns <n-1>-<n+1> --json
```

`search` is a case-insensitive substring match on prompts and replies, not a semantic search.
Try two or three wordings (both the user's language and English, the identifiers involved)
before concluding that nothing was found. Each hit has `id`, `turn` and a `snippet`; open the
surrounding turns with `show --turns` before quoting a decision.

## Summarize today's or recent work

```sh
ccreel list --since 1d --limit 0 --json
```

Each line's `title` and `first_prompt` are usually enough to group the work. Run `outline` only
for sessions whose topic is unclear from those two fields. Use `--since 7d` for the week.

## Fields

| Command | Fields |
|---|---|
| `list` | `id`, `repo`, `cwd`, `branch`, `issue`, `title`, `first_prompt`, `prompts`, `started`, `updated` |
| `outline` | `turn`, `time`, `prompt`, `reply`, `tools` |
| `show` | `turn`, `time`, `prompt`, `messages[]` (`kind`: `text` / `tool_use` / `tool_result`, `tool`, `text`) |
| `search` | `id`, `repo`, `title`, `updated`, `turn`, `kind`, `snippet` |

## Keeping output small

- `show` caps each message at 2000 characters (`--max-chars`) and the whole output at 40000
  (`--max-total`). Narrow with `--turns` or `--last` rather than raising the caps.
- `show --final` keeps only the last reply of each turn.
- Tool calls and tool output are left out unless you pass `--tools`; add it only when the answer
  is in a command's output.

When you report what you found, name the session (first 8 characters of `id`) and its date so the
user can open it with `ccreel resume <id>`.
