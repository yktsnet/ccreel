# Guarantee Ledger

## Guarantees

### 1. `cmd/ccreel/main_test.go` — cmd/ccreel (list / show / resume)

- `show --turns` は `3` で1ターン、`2-4` で範囲を選び、`--last N` はその中の最後の N ターンに絞る。数でない指定や逆順の範囲はエラーにする
- 1つのメッセージを切り詰めたときは、末尾に `… [truncated N chars]` を付けて、落とした文字数を示す
- `show <id> --final --json --last 1` のように、フラグはセッション ID の後ろに置いても効く
- `show` は、出力の合計文字数が `--max-total` を超えたら、最初のターンだけは出して止め、残りのターン数と次のターン番号を stderr に書く。stdout の JSON Lines には混ぜない
- `pick` と `resume` で `--` より後ろに書いた引数は、そのまま claude に渡す
- resume で claude に渡す環境では、`CLAUDE_CONFIG_DIR` を選んだ config dir の1つだけにする
- `list` は、`CLAUDE_CODE_SESSION_ID` のセッション（呼んでいる Claude Code 自身）を外す。`--include-current` で戻す
- `list --repo` はリポ名の完全一致で絞る
- `list --issue` は `12` と `#12` のどちらでも、その Issue 番号のセッションだけに絞る。JSON に `issue` を出す
- `list` は、発言が1回のセッションを既定で外し、`--all` で出す

| 保証（要約） | 対応テスト |
|---|---|
| `--turns` と `--last` の選び方 | `TestSelectTurns` |
| 切り詰めの表示 | `TestTruncate` |
| ID の後ろのフラグ | `TestShowFlagsAfterID` |
| `--max-total` で止めて stderr に書く | `TestShowMaxTotal` |
| `--` の後ろを claude へ | `TestSplitPassthrough` |
| `CLAUDE_CONFIG_DIR` を1つにする | `TestClaudeEnvReplacesConfigDir` |
| いまのセッションを外す | `TestListSkipsCurrentSession` |
| `--repo` は完全一致 | `TestListRepoIsExact` |
| `--issue` で絞る | `TestListIssue` |
| 発言が1回のセッションを外す | `TestListHidesSinglePromptSessions` |

*（この節には複数のサブコマンドが混在するので、行ごとに主語を明示している）*

### 2. `internal/finder/finder_test.go` — internal/finder (ParseQuery / Find)

- `ParseQuery` は `@名前` をリポの指定、`#12` と `12` を Issue 番号として読み、番号は本文の語としても残す
- `Find` は、題と本文に語が当たったセッションを返し、当たった理由（`title:語`、`body:語×数`）を添える。本文の数は、人の発言と応答の文章のうち語を含むものの数である。ツールの呼び出しと出力にしか無い語では本文に当たらない
- `Find` は、ブランチか worktree の名前の Issue 番号が一致するセッションを返し、理由の先頭に `issue #番号` を置く
- `Find` は、`@リポ` だけを渡すと、そのリポのセッションだけを返す
- `Find` は、`@リポ` を読む本数の上限（40本）に絞る前に当てる。他のリポに、仮の点がそのリポのセッションより高いセッションが上限より多くあっても、そのリポのセッションを返す
- `Find` は、`Exclude` に渡したセッションも、読む本数の上限に絞る前に外す。外したセッションは読む本数に数えない
- `Find` は、同じ履歴と同じ言葉に対して、毎回同じ候補を同じ順で返す
- `Find` は、題に語があるセッションを、本文で語をくり返しただけのセッションより上に置く
- `Find` は、Issue の作業をしたセッション（ブランチ・worktree の番号が一致）を、本文で番号をくり返しただけのセッションより上に置く
- `Find` は、`Exclude` に渡したセッション（いま話しているもの）を返さない
- `Find` は、語も番号もリポも無ければ `ErrEmpty` を返す
- 1位が2位の1.5倍以上で、かつ20点以上離れているときに限り、`decided` を真にする。候補が1本だけのときは2位を0点とみなす

| 保証（要約） | 対応テスト |
|---|---|
| `@`・`#`・語の読み分け | `TestParseQuery` |
| 題と本文で探して理由を返す | `TestFindByTitleAndBody` |
| Issue 番号で探す | `TestFindByIssue` |
| `@リポ` だけで絞る | `TestFindByRepoOnly` |
| `@リポ` を上限の前に当てる | `TestRepoIsAppliedBeforeTheReadLimit` |
| `Exclude` のセッションを上限の前に外す | `TestExcludeIsAppliedBeforeTheReadLimit` |
| 毎回同じ候補と同じ順序 | `TestFindIsDeterministic` |
| ツールの入出力では本文に当たらない | `TestBodyIgnoresToolOutput` |
| 題の一致が本文のくり返しより上 | `TestTitleOutranksBody`, `TestRanking` |
| Issue の一致が本文の番号より上 | `TestRanking` |
| いまのセッションを外す | `TestExcludeCurrent` |
| 空の問い合わせ | `TestEmptyQuery` |
| 決め切れるかの判定 | `TestDecided` |

*（この節には `ParseQuery`・`Find`・判定の3つが混在するので、行ごとに主語を明示している）*

### 3. `internal/hubsync/hubsync_test.go` — internal/hubsync (Push / Pull / ParseHub)

- `Push` はセッション本体の jsonl だけをハブへ送り、`<uuid>/` の付属物と `memory/` は送らない。ハブではプロジェクト名と `"cwd"` のホームを `~` に置き換える
- `Pull` は、別の端末が送ったセッションを手元の形（自分のホーム）に戻して config dir に書き、元の更新日時を保つ
- `Pull` は、手元の写しよりハブの方が新しいものが無ければ、ハブから何も取らない
- `Pull` したセッションを `Push` し直しても、ハブの中身も更新日時も変わらない
- `Pull` は、手元の方が新しいセッションを上書きしない。手元で追記したセッションは、`Push` すると相手の端末に届く
- `Push` は、ハブと mirror（ハブと同じ形にした手元の写し）のうち、最後の更新から `KeepDays`（既定40日）を過ぎたセッションを消す。`KeepDays` を過ぎた手元のセッションは送らない
- `Pull` は、ハブでの更新から `PullDays`（既定30日）以内のセッションだけを取る
- `Push` は、変わったセッションが無く、前回の送信も済んでいれば、ハブにつながない
- `Push` は、送れなかった変更を、次の `Push` で変わったものが無くても送り直す
- `Push` は、ハブと mirror の古いものを消す処理を1日に1回だけ行う
- `Force` を付けた `Push` は、変わったものが無くても送る
- 同じ mirror を別の `Push` か `Pull` が使っているあいだは、待たずに `ErrBusy` を返す
- `Push` と `Pull` は、ssh の先のハブに `~/.ssh/config` の多重化を使わずにつなぎ、応答の止まった接続と転送を時間で切る
- `ParseHub` は、最初の `/` より前に `:` があれば `host:path`（ssh の先）、それ以外は手元のディレクトリとして読み、空の指定はエラーにする

| 保証（要約） | 対応テスト |
|---|---|
| 本体だけを `~` の形で送る | `TestPushThenPullAcrossMachines` |
| 手元の形に戻し、日時を保つ | `TestPushThenPullAcrossMachines` |
| 新しいものが無ければ取らない | `TestPushThenPullAcrossMachines` |
| `Push` し直してもハブが変わらない | `TestPushThenPullAcrossMachines` |
| 新しい側が勝つ | `TestNewerSideWins` |
| 40日で消し、古い手元のものは送らない | `TestPullWindowAndPrune` |
| 30日以内のものだけ取る | `TestPullWindowAndPrune` |
| 変化が無ければつながない | `TestPushOnlyWhenChanged` |
| 送れなかった変更の送り直し | `TestPushOnlyWhenChanged` |
| 削除は1日1回 | `TestPushOnlyWhenChanged` |
| `Force` | `TestPushOnlyWhenChanged` |
| 重なったら `ErrBusy` | `TestLockedMirror` |
| 止まった接続を待ち続けない | `TestRemoteHubDoesNotWaitForever` |
| ハブの指定の読み方 | `TestParseHub` |

*（この節には `Push`・`Pull`・`ParseHub` が混在するので、行ごとに主語を明示している）*

### 4. `internal/pathmap/pathmap_test.go` — internal/pathmap (Mapper)

- プロジェクト名の先頭のホーム（`-home-alice`・`-Users-alice`）を `~` に置き換え、逆の変換で元に戻す。ホームそのものは `~` にする
- ホームの名前で始まるだけの別のディレクトリ（`-home-alice2-…`）と、ホームの外のプロジェクトは変えない
- jsonl の `"cwd"` の先頭のホームを `~` に置き換え、逆の変換で元に戻す。`"cwd"` 以外のパスと、ホームの名前で始まるだけの別のパスは変えない
- macOS で送ったものを Linux で戻すと、Linux のホームの形になる。戻したものを送り直すと、ハブの中身と同じバイト列になる

| 保証（要約） | 対応テスト |
|---|---|
| プロジェクト名の往復 | `TestProjectRoundTrip` |
| 別のディレクトリに触れない | `TestProjectRoundTrip` |
| `"cwd"` の往復 | `TestContentRoundTrip` |
| 端末をまたいだ往復と、送り直しで同じバイト列 | `TestProjectRoundTrip`, `TestContentRoundTrip` |

### 5. `internal/picker/picker_test.go` — internal/picker (Line / Ago / ResumeDir / selectedIDs)

- `Line` は fzf の1行を「UUID・題・`@リポ #番号`・更新日・24時間以内の経過時間」のタブ区切りで作り、題より後ろは薄い色で出す。題が無ければ最初の発言を1行にして題の代わりに使う
- `Ago` は、更新からの経過時間を24時間で割って切り捨てた値を日数とし、9日（216〜240時間前）までは `*` に続けて日数を、それより前は週（`2w`）を返す。日付をまたいでも、24時間以内なら `*0` になる
- `Elapsed` は、24時間以内（`*0`）のものにだけ経過時間を返す。1分未満は `now`、60分未満は分（`12m`）、それ以上は時間（`23h`）で、24時間を過ぎたものは空にする
- `ResumeDir` は、記録された cwd があればそこを、無ければ手元にある一番近い祖先を返す。祖先がホームそのもの、またはホームより上になるとき、および cwd が空のときは、ホームを返す
- `selectedIDs` は、fzf で複数の行を選んだとき、選んだ順にすべての行の UUID を返す

| 保証（要約） | 対応テスト |
|---|---|
| fzf の1行の形 | `TestLine` |
| 経過時間を24時間ごとに数えた日数 | `TestAgo` |
| 24時間以内のものにだけ付く経過時間 | `TestElapsed` |
| resume で入るディレクトリ | `TestResumeDir` |
| fzf で選んだ複数の行の ID を、選んだ順に取る | `TestSelectedIDs` |

*（この節には `Line`・`Ago`・`ResumeDir`・`selectedIDs` が混在するので、行ごとに主語を明示している）*

### 6. `internal/session/search_test.go` — internal/session (search)

- 検索の抜粋は、当たった語の前後を指定の文字数だけ残し、途中で切ったところに `…` を付ける。大文字小文字は区別しない
- 検索の対象は projects 直下のセッションで、サブエージェントの記録にしか無い語では当たらない
- 検索は、既定で発言と応答の文章だけを探す。`Tools` を付けるとツールの入出力も探す。1セッションからの件数と全体の件数に上限を付けられる

| 保証（要約） | 対応テスト |
|---|---|
| 抜粋の形 | `TestSnippet` |
| サブエージェントの記録は探さない | `TestCandidates` |
| 探す範囲と件数の上限 | `TestSearch` |

### 7. `internal/session/session_test.go` — internal/session (Discover / Scan / ScanAllTerms)

- 一覧に出すのは projects 直下のセッションだけで、`<uuid>/subagents/` などの付属物は出さない
- 一覧の1行は、題（最後に付け直されたもの）・最初の発言（スラッシュコマンドは打った形）・最後の cwd・リポ・ブランチ・Issue 番号・発言の数・最初と最後の時刻を持つ。発言の数に、`isMeta`・`tool_result`・サブエージェントの発言は入れない
- Issue 番号は、セッションの途中で Issue のブランチか worktree にいれば、最後に main へ戻っていても残す。複数あれば最後に現れたものにする
- `ScanAllTerms` は、語ごとに、人の発言と応答の文章のうちその語を含むものの数を数える。ツールの呼び出しと出力・添付・サブエージェント側の応答は数えない
- 一覧は新しい順に並び、発言の無いセッションと、`claude -p` で動かしたセッション（`entrypoint` が `sdk-cli`）は出さない
- 同じ UUID が複数のプロジェクトにあるときは、更新の新しい方の1行だけを出す
- リポ名は起動した場所の cwd から取る。ホームで起動したセッションは最後の cwd から取る
- リポ名は cwd の末尾から取る。cwd が worktree の中（`.claude/worktrees/…` と `<リポ>.wt/…`）か、worktree を置くディレクトリそのもののときは、元のリポの名前にする。Issue 番号は、`task/417-…` のようなブランチ名か worktree の名前から取る。数字の直後に `-` と英字が続くときだけ、その数字を番号にし、`day/2026-10-08` のような日付は番号にしない

| 保証（要約） | 対応テスト |
|---|---|
| 付属物を出さない | `TestDiscoverSkipsAttachments` |
| 一覧の1行の中身 | `TestScan` |
| 新しい順・発言の無いものを出さない | `TestScanAllOrderAndEmpty` |
| リポ名と Issue 番号の取り方 | `TestRepoNameAndIssue` |
| `claude -p` のセッションを出さない | `TestScanAllSkipsHeadless` |
| 同じ UUID は1行にまとめる | `TestSameUUIDInTwoProjects` |
| リポは起動した場所から | `TestRepoComesFromWhereTheSessionStarted` |
| 途中で現れた Issue 番号を残す | `TestIssueSurvivesReturnToMain` |
| 語の当たりは会話の文章だけで数える | `TestScanAllTermsCountsConversationOnly` |

### 8. `internal/session/transcript_test.go` — internal/session (ReadTurns / Find)

- セッションを、人の発言1つと次の発言の前までの応答（文章・ツールの呼び出し・ツールの出力）に分ける。ツールの出力には呼んだツールの名前を付け、ターンの最終応答はその中の最後の文章にする
- セッション ID は UUID の先頭の数文字で引ける。当たらない指定、16進とハイフン以外を含む指定、複数のセッションに当たる指定はエラーにする。同じ UUID が複数のプロジェクトにあるときは、更新の新しい方を返す
- compact のあとに自動で入る要約は、人の発言として数えない
- 画像を添えた発言と、`<pasted_content>` で始まる発言は、人の発言として数える。中断を示す文言（`[Request interrupted by user]`、`for tool use` を含む）・`tool_result` を含むもの・バックグラウンドの作業の通知・コマンドの出力は数えない

| 保証（要約） | 対応テスト |
|---|---|
| ターンへの分け方 | `TestReadTurns` |
| ID の引き方 | `TestFind`, `TestSameUUIDInTwoProjects` |
| compact の要約を数えない | `TestPromptTextSkipsCompactSummary` |
| 発言に数える形と数えない形 | `TestPromptTextShapes` |

### 9. `cmd/ccreel/cli_test.go` — cmd/ccreel (resume / show / outline / list / search / find / pick / push / preview)

- `resume` は、`CCREEL_CONFIG_DIRS` のどの config dir にあるセッションでも ID から引き、記録された cwd へ移って、そのセッションのある config dir を `CLAUDE_CONFIG_DIR` にして claude を起動する
- `show` と `outline` は、`CCREEL_CONFIG_DIRS` の別の config dir にあるセッションも ID から引く
- `list` と `search` は、`CCREEL_CONFIG_DIRS` のすべての config dir を探す。まだ `projects` の無い config dir が混ざっていても失敗しない
- `find` は、複数の config dir のセッションを合わせて順位を付け、候補ごとに `config_dir` を返す
- `pick <言葉>` は、`find` と同じ順位で fzf に行を渡す
- `pick` は、`CCREEL_CONFIG_DIRS` のすべての config dir のセッションを fzf に渡す
- `pick` で複数を選ぶと、tmux の中では2本目から `tmux new-window` で、そのセッションの config dir を `CLAUDE_CONFIG_DIR` にして開き、1本目はいまのプロセスで開く
- `outline --json` の項目は `turn`・`time`・`prompt`・`reply`・`tools`、`search --json` の項目は `id`・`repo`・`title`・`updated`・`turn`・`kind`・`snippet` である
- `push` は、送らなかった回に stdout にも stderr にも何も出さない
- `preview` は、題・`@リポ`・`#番号`・ブランチを先頭にまとめ、その下に新しいターンから順に、発言と最終応答を Markdown で並べる
- `CCREEL_HUB` が手元のディレクトリなら、`list`・`search`・`show`・`find` はハブにしか無いセッションも読み、cwd の先頭の `~` を手元のホームに戻す。`resume` はハブにしか無いセッションを開かない
- 手元の config dir とハブの両方にあるセッションは、`list`・`search` で1本として扱い、`show` は失敗せず、`find` は手元の config dir の候補だけを返す
- `CCREEL_HUB` が ssh の先（`host:path`）なら、ハブを読まない

| 保証（要約） | 対応テスト |
|---|---|
| 別の config dir のセッションを、その config dir で resume | `TestResumeUsesTheSessionsConfigDir` |
| 別の config dir のセッションを show・outline | `TestShowFindsSessionInOtherConfigDir` |
| `list` と `search` はすべての config dir を探す | `TestListAndSearchLookInEveryConfigDir` |
| 複数の config dir を合わせた順位 | `TestFindRanksAcrossConfigDirs` |
| `pick <言葉>` の並びは `find` の順位 | `TestPickWithWordsUsesFindOrder` |
| `pick` はすべての config dir を出す | `TestPickListsEveryConfigDir` |
| 複数を選ぶと tmux のウィンドウで開く | `TestPickOpensExtraSelectionsInTmux` |
| `outline` と `search` の JSON の項目 | `TestOutlineAndSearchJSONFields` |
| 送らなかった回は何も出さない | `TestPushIsQuietWhenNothingIsSent` |
| `preview` の形と並び | `TestPreviewIsNewestFirstMarkdown` |
| 手元のハブも読み、resume はしない | `TestReadCommandsLookInLocalHub` |
| 手元とハブの両方にあれば手元の1本 | `TestLocalCopyWinsOverHub` |
| ssh の先のハブは読まない | `TestRemoteHubIsNotRead` |

*（この節には複数のサブコマンドが混在するので、行ごとに主語を明示している）*

## Gaps

以下は保証すべきと思われるが、対応するテストが無い。

- picker の絞り込みで、`*0`・`*1` が24時間以内・24〜48時間前の行にだけ当たること（`*1` が10〜19日前の行に当たらない）。`12` のように数字だけを打っても、更新日の列に広く当たらないこと。経過時間の列（`12m` など）が `--nth` で絞り込みから外れていて、`*1` や `12` が `*0` の行に当たらないこと。fzf を対話で動かす必要があり、自動のテストでは確かめられない。tmux の中で手で確かめた（経過時間の列は `fzf --filter` で確かめた）
- picker で選んだあと、claude を起動する前に端末の画面を消し、fzf の表示と claude の表示が重ならないこと。実際の端末でしか確かめられない。WSL と macOS の tmux の中で手で確かめた
- picker で Tab を押して選んだ複数のセッションが、実際の tmux で別々のウィンドウに開くこと。tmux と fzf を対話で動かす必要がある。macOS の tmux の中で、claude の代わりのスクリプトを使って確かめた
- ハブが BSD の `stat` しか持たないときにも、`Pull` が更新日時つきの一覧を取れること。手元にも CI にも BSD の環境が無く、通るかを確かめられない。GNU の `stat` の環境でしか通していない

## About

対象は `ccreel` の CLI のサブコマンドと出力、ハブに置くデータの形、push と pull の約束、同梱の skill が頼る `find` の順位である。fzf の画面の見た目や、rg が無い環境でも仮の順位に使う当たり行数が変わらないことなど、内部の作りは対象外である。**ここに載っていない振る舞いは約束ではなく、予告なく変わりうる**。地位は design-decisions.md 相当のドキュメントと同格である。
