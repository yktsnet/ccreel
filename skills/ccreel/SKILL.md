---
name: ccreel
description: Look up past Claude Code sessions with the ccreel CLI. Use when the user asks to continue earlier work ("pick up where the last session left off", "read the previous session"), asks whether something was already discussed or decided ("didn't we decide this before?"), asks for the conversation about an issue or topic, or asks what was worked on today or recently.
---

# ccreel

`ccreel` reads Claude Code's own session files. Use it instead of opening
`~/.claude*/projects/**/*.jsonl` directly: the raw files are large and mostly tool output.

- The session you are running in is always left out, so "the newest session" is the previous one.
- `<id>` accepts any unique prefix of the session UUID; the first 8 characters are enough.
  `show`, `outline` and `resume` find the session in any config dir listed in `CCREEL_CONFIG_DIRS`.
- Always pass `--json`. When output is cut short, a line starting with `ccreel:` on stderr says
  what was dropped and which flag widens it.

## Finding a past conversation: always start with `find`

`find` does the searching for you in one call: it looks in every config dir, matches the issue
number against branch and worktree names, the words against session titles and text, and ranks
the sessions. Do not chain `list` and `search` by hand to do the same.

```sh
ccreel find <words...> --json
```

Give it the parts of the request that identify the conversation, not the whole sentence:

| In the request | Pass |
|---|---|
| an issue or PR number ("951", "#417") | `951` (matched as `#issue` and as text) |
| a repo ("in invoice_issue") | `@invoice_issue` (keeps only that repo) |
| a topic ("the WireGuard discussion") | `wireguard` |
| a topic in a sentence ("the work to run workflows in parallel") | 2 or 3 key words: `並列 ワークフロー` |

Then follow the result:

1. **`decided: true`** — read the first candidate: `ccreel outline <id> --json`, then
   `ccreel show <id> --turns <a>-<b> --json` for the turns you need. Mention the second candidate
   in one line if its reasons show it is about the same topic (a follow-up session, for example).
2. **`decided: false` with candidates** — do not read them all. Show the top 3 or 4 to the user
   (date, `@repo`, `#issue`, title, and why it matched) and ask which one they mean, with the
   AskUserQuestion tool if you have it. If the request asked for the latest one, say which is newest.
3. **No candidates** — retry once with different words (the user's language and English, an
   identifier, a shorter word). If there is still nothing, say so; do not guess.

Each candidate has `reasons`: `issue #N` (on that issue's branch or worktree), `title:<word>`,
`body:<word>×<lines>`, `all terms`, `same repo` (the repo you were called from).

## Continue the previous session in this repo

```sh
ccreel list --repo "$(basename "$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")")" --limit 3 --json
ccreel outline <id> --json
ccreel show <id> --last 3 --json
```

`--git-common-dir` gives the main checkout even inside a worktree.

## Summarize today's or recent work

```sh
ccreel list --since 1d --limit 0 --json
```

`list` hides sessions with a single prompt; add `--all` when a one-off question matters too.
Each line's `title` and `first_prompt` are usually enough to group the work. Run `outline` only
for sessions whose topic is unclear from those two fields. Use `--since 7d` for the week.

## Fields

| Command | Fields |
|---|---|
| `find` | `decided`, `candidates[]` (`id`, `config_dir`, `repo`, `issue`, `title`, `updated`, `score`, `reasons`, …) |
| `list` | `id`, `repo`, `cwd`, `branch`, `issue`, `title`, `first_prompt`, `prompts`, `started`, `updated` |
| `outline` | `turn`, `time`, `prompt`, `reply`, `tools` |
| `show` | `turn`, `time`, `prompt`, `messages[]` (`kind`: `text` / `tool_use` / `tool_result`, `tool`, `text`) |

## Keeping output small

- `show` caps each message at 2000 characters (`--max-chars`) and the whole output at 40000
  (`--max-total`). Narrow with `--turns` or `--last` rather than raising the caps.
- `show --final` keeps only the last reply of each turn.
- Tool calls and tool output are left out unless you pass `--tools`.
- `search <phrase>` returns the matching lines with context when you need to locate a quote inside
  a session you already chose.

When you report what you found, name the session (first 8 characters of `id`) and its date so the
user can open it with `ccreel resume <id>`.
