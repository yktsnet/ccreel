# Usage

## コマンド

| コマンド | 動き |
|---|---|
| `pick [言葉]` | pull してから fzf で選び、resume する。引数なしの `ccreel` と同じ |
| `resume <id>` | 記録された cwd へ移って resume する |
| `list` | セッションを新しい順に並べる |
| `find <言葉>` | `#番号`・`@リポ`・語で順位を付け、1本に決め切れたかを返す |
| `outline <id>` | 1ターン1行の目次 |
| `show <id>` | ターンの本文 |
| `search <語>` | 語が出てくる箇所と前後の文脈 |
| `preview <id>` | picker の右のペインに出す Markdown |
| `push` / `pull` | ハブとのあいだでセッションを運ぶ |

`<id>` は UUID の先頭の数文字で足りる。`pick` と `resume` で `--` の後ろに書いた引数は、そのまま claude に渡す（`ccreel resume 1c0e662e -- --permission-mode auto`）。
各コマンドのフラグは `ccreel <command> -h` で出る。

## fzf の画面

| 操作 | 動き |
|---|---|
| Enter | 選んだセッションを、記録された cwd へ移って resume する |
| Tab | 複数を選ぶ。1本目はいまのペインで、2本目からは tmux の新しいウィンドウで開く（tmux の外では1本目だけ） |
| `@app` `#417` | リポ・Issue 番号で絞る |
| `*0` `*1` | 今日・昨日に更新したものに絞る（9日前まで `*9` の形で出る）。`12` と打っても日付には当たらない |
| Ctrl-/ | プレビューの表示を切り替える |

一覧には、`claude -p` で動かしたセッションと、発言が1回だけのセッションを出さない（後者は `--all` で出す）。

## JSON の出力

`--json` を付けると JSON Lines を出す。skill などが項目名に依存するので、変えるときは同梱の skill もあわせて直す。

| コマンド | 項目 |
|---|---|
| `find` | `decided`、`candidates[]`（`id`・`config_dir`・`repo`・`issue`・`title`・`updated`・`score`・`reasons` など） |
| `list` | `id`・`repo`・`cwd`・`branch`・`issue`・`title`・`first_prompt`・`prompts`・`started`・`updated` |
| `outline` | `turn`・`time`・`prompt`・`reply`・`tools` |
| `show` | `turn`・`time`・`prompt`・`messages[]`（`kind` は `text`・`tool_use`・`tool_result`） |
| `search` | `id`・`repo`・`title`・`updated`・`turn`・`kind`・`snippet` |

**ターン**は、人の発言1つと、次の発言の前までの応答をまとめた単位である。

`show` は1つのメッセージを2000文字（`--max-chars`）、全体を40000文字（`--max-total`）で止める。`show` と `search` は、既定でツールの入出力を出さない（`--tools` で出す）。
上限で止めたときは、その旨を stderr に書き、stdout の JSON Lines には混ぜない。

## 環境変数

| 変数 | 意味 |
|---|---|
| `CCREEL_HUB` | ハブの rsync の宛先。`host:path` なら ssh の先、それ以外は手元のディレクトリ。`--hub` でも渡せる |
| `CCREEL_CONFIG_DIRS` | まとめて扱う config dir（コロン区切り）。`pick`・`find`・`show`・`outline`・`resume` が全部を探す |
| `CLAUDE_CONFIG_DIR` | 既定の config dir。無ければ `~/.claude`。`--config-dir` が優先する |
| `CCREEL_FZF_OPTS` | fzf に足す引数 |
| `CCREEL_CLAUDE` | `claude` の代わりに起動するコマンド |
| `XDG_CACHE_HOME` | mirror の置き場の基点 |

resume のときは、セッションのある config dir を `CLAUDE_CONFIG_DIR` にして claude を起動する。

## push / pull のフラグ

| フラグ | 既定 | 意味 |
|---|---|---|
| `--pull-days` | 30 | ハブでの更新からこの日数以内のセッションだけを pull する |
| `--keep-days` | 40 | 更新からこの日数を過ぎたセッションを、ハブと mirror から消す（1日に1回） |
| `--mirror` | `$XDG_CACHE_HOME/ccreel/<config dir 名>` | ハブと同じ形にした手元の写しの置き場 |
| `--force`（push） | | 変わったものが無くても送る |
| `--no-pull`（pick） | | ハブから取らずに開く |
