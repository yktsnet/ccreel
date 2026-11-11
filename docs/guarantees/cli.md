# cli

## `show`・`outline`・`preview`

| 保証 | 対応テスト |
|---|---|
| `show --turns 3` は3番目のターンだけを選ぶ | `TestSelectTurns` |
| `show --turns 2-4` は2〜4番目のターンを選ぶ | `TestSelectTurns` |
| `show --last N` は選んだターンのうち最後の N ターンに絞る | `TestSelectTurns` |
| `show --turns` は数でない指定と逆順の範囲をエラーにする | `TestSelectTurns` |
| 切り詰めたメッセージは、末尾に `… [truncated N chars]` を付けて、落とした文字数を示す | `TestTruncate` |
| `show` は `show <id> --final --json --last 1` のようにセッション ID の後ろに置いたフラグも、前に置いたときと同じに扱う | `TestShowFlagsAfterID` |
| `show` は出力の合計文字数が `--max-total` を超えたら、最初のターンだけは出して止める | `TestShowMaxTotal` |
| `show` は `--max-total` で止めたとき、残りのターン数と次のターン番号を stderr に書く | `TestShowMaxTotal` |
| `outline --json` の項目は `turn`・`time`・`prompt`・`reply`・`tools` である | `TestOutlineAndSearchJSONFields` |
| `preview` はタイトル・`@リポ`・`#番号`・ブランチを先頭にまとめる | `TestPreviewIsNewestFirstMarkdown` |
| `preview` は先頭の下に新しいターンから順に、発言と最終応答を Markdown で並べる | `TestPreviewIsNewestFirstMarkdown` |

## `list` と `search`

| 保証 | 対応テスト |
|---|---|
| `list` は `CLAUDE_CODE_SESSION_ID` のセッション（呼んでいる Claude Code 自身）を外す | `TestListSkipsCurrentSession` |
| `list --include-current` は `CLAUDE_CODE_SESSION_ID` のセッションも出す | `TestListSkipsCurrentSession` |
| `list --repo` はリポ名の完全一致で絞る | `TestListRepoIsExact` |
| `list --issue` は `12` と `#12` のどちらでも、その Issue 番号のセッションだけに絞る | `TestListIssue` |
| `list --json` は各行に `issue` を出す | `TestListIssue` |
| `list` は発言が1回のセッションを既定で外す | `TestListHidesSinglePromptSessions` |
| `list --all` は発言が1回のセッションも出す | `TestListHidesSinglePromptSessions` |
| `search --json` の項目は `id`・`repo`・`title`・`updated`・`turn`・`kind`・`snippet` である | `TestOutlineAndSearchJSONFields` |

## `pick` と `resume`

| 保証 | 対応テスト |
|---|---|
| `pick` と `resume` は `--` より後ろに書いた引数をそのまま claude に渡す | `TestSplitPassthrough` |
| `resume` はclaude に渡す環境の `CLAUDE_CONFIG_DIR` を、選んだ config dir の1つだけにする | `TestClaudeEnvReplacesConfigDir` |
| `pick <言葉>` は `find` と同じ順位で fzf に行を渡す | `TestPickWithWordsUsesFindOrder` |
| `pick` はfzf で複数を選ぶと、tmux の中では2本目から `tmux new-window` で開き、1本目はいまのプロセスで開く | `TestPickOpensExtraSelectionsInTmux` |
| `pick` が `tmux new-window` で開くセッションは、そのセッションの config dir を `CLAUDE_CONFIG_DIR` にして開く | `TestPickOpensExtraSelectionsInTmux` |

## 複数の config dir（`CCREEL_CONFIG_DIRS`）を読むコマンド

| 保証 | 対応テスト |
|---|---|
| `resume` は `CCREEL_CONFIG_DIRS` のどの config dir にあるセッションでも ID から引く | `TestResumeUsesTheSessionsConfigDir` |
| `resume` はセッションに記録された cwd へ移って claude を起動する | `TestResumeUsesTheSessionsConfigDir` |
| `resume` はセッションのある config dir を `CLAUDE_CONFIG_DIR` にして claude を起動する | `TestResumeUsesTheSessionsConfigDir` |
| `show` と `outline` は `CCREEL_CONFIG_DIRS` の別の config dir にあるセッションも ID から引く | `TestShowFindsSessionInOtherConfigDir` |
| `list` と `search` は `CCREEL_CONFIG_DIRS` のすべての config dir を探す | `TestListAndSearchLookInEveryConfigDir` |
| `list` と `search` はまだ `projects` の無い config dir が混ざっていても失敗しない | `TestListAndSearchLookInEveryConfigDir` |
| `find` は複数の config dir のセッションを合わせて順位を付ける | `TestFindRanksAcrossConfigDirs` |
| `find` は候補ごとにその config dir を返す | `TestFindRanksAcrossConfigDirs` |
| `pick` は `CCREEL_CONFIG_DIRS` のすべての config dir のセッションを fzf に渡す | `TestPickListsEveryConfigDir` |

## ハブ（`CCREEL_HUB`）とのやり取り

| 保証 | 対応テスト |
|---|---|
| `push` はハブへ送らなかった回に stdout にも stderr にも何も出さない | `TestPushIsQuietWhenNothingIsSent` |
| `CCREEL_HUB` が手元のディレクトリなら、`list`・`search`・`show`・`find` はハブにしか無いセッションも読む | `TestReadCommandsLookInLocalHub` |
| ハブから読んだセッションの cwd は、先頭の `~` を手元のホームに戻して出す | `TestReadCommandsLookInLocalHub` |
| `resume` はハブにしか無いセッションを開かない | `TestReadCommandsLookInLocalHub` |
| 手元の config dir とハブの両方にあるセッションは、`list`・`search` で1本として扱う | `TestLocalCopyWinsOverHub` |
| 手元の config dir とハブの両方にあるセッションを、`show` は失敗せずに出す | `TestLocalCopyWinsOverHub` |
| 手元の config dir とハブの両方にあるセッションについて、`find` は手元の config dir の候補だけを返す | `TestLocalCopyWinsOverHub` |
| `CCREEL_HUB` が ssh の先（`host:path`）なら、読み出しのコマンドはハブを読まない | `TestRemoteHubIsNotRead` |
