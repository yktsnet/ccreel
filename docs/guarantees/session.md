# session

## 一覧（`Discover`・`Scan`・`ScanAll`・`ScanAllCached`）

| 保証 | 対応テスト |
|---|---|
| 一覧はprojects 直下のセッションだけを出し、`<uuid>/subagents/` などの付属物は出さない | `TestDiscoverSkipsAttachments` |
| 一覧の1行はタイトル・最初の発言（スラッシュコマンドは打った形）・最後の cwd・リポ・ブランチ・Issue 番号・発言の数・最初と最後の時刻を持つ | `TestScan` |
| 一覧の発言の数に、`isMeta`・`tool_result`・サブエージェントの発言は入れない | `TestScan` |
| 一覧は新しい順に並ぶ | `TestScanAllOrderAndEmpty` |
| 一覧は発言の無いセッションを出さない | `TestScanAllOrderAndEmpty` |
| 一覧は `claude -p` で動かしたセッション（`entrypoint` が `sdk-cli`）を出さない | `TestScanAllSkipsHeadless` |
| 一覧は同じ UUID が複数のプロジェクトにあるとき、更新の新しい方の1行だけを出す | `TestSameUUIDInTwoProjects` |
| `ScanAllCached` は `ScanAll` と同じ一覧を返す | `TestScanAllCachedMatchesAndRefreshes` |
| `ScanAllCached` は大きさが変わったセッションを読み直す | `TestScanAllCachedMatchesAndRefreshes` |
| `ScanAllCached` は消えたセッションを出さない | `TestScanAllCachedMatchesAndRefreshes` |

## 一覧の行のタイトル・リポ名・Issue 番号

| 保証 | 対応テスト |
|---|---|
| タイトルは `/rename` で付けた名前があれば、その最後のものになる | `TestScanPrefersCustomTitle` |
| タイトルは `/rename` のあとに Claude Code がタイトルを付け直しても、`/rename` で付けた名前のままである | `TestScanPrefersCustomTitle` |
| タイトルは `/rename` で付けた名前が無ければ、Claude Code が付けたタイトルの最後のものになる | `TestScan` |
| リポ名は起動した場所の cwd から取る | `TestRepoComesFromWhereTheSessionStarted` |
| ホームで起動したセッションのリポ名は、最後の cwd から取る | `TestRepoComesFromWhereTheSessionStarted` |
| リポ名はcwd の末尾から取る | `TestRepoNameAndIssue` |
| リポ名はcwd が worktree の中（`.claude/worktrees/…` と `<リポ>.wt/…`）か、worktree を置くディレクトリそのもののときは、元のリポの名前になる | `TestRepoNameAndIssue` |
| Issue 番号は `task/417-…` のようなブランチ名か worktree の名前から取る | `TestRepoNameAndIssue` |
| Issue 番号は数字の直後に `-` と英字が続くときだけその数字になり、`day/2026-10-08` のような日付は番号にならない | `TestRepoNameAndIssue` |
| Issue 番号はセッションの途中で Issue のブランチか worktree にいれば、最後に main へ戻っていても残る | `TestIssueSurvivesReturnToMain` |
| Issue 番号は複数あれば最後に現れたものになる | `TestIssueSurvivesReturnToMain` |

## 語の数え方と検索

| 保証 | 対応テスト |
|---|---|
| `ScanAllTerms` は語ごとに、人の発言と応答の文章のうちその語を含むものの数を数える | `TestScanAllTermsCountsConversationOnly` |
| `ScanAllTerms` はツールの呼び出しと出力・添付・サブエージェント側の応答を数えない | `TestScanAllTermsCountsConversationOnly` |
| 検索の抜粋は当たった語の前後を指定の文字数だけ残し、途中で切ったところに `…` を付ける | `TestSnippet` |
| 検索の抜粋は大文字小文字を区別せずに語を探す | `TestSnippet` |
| 検索はprojects 直下のセッションを対象にし、サブエージェントの記録にしか無い語では当たらない | `TestCandidates` |
| 検索は既定で発言と応答の文章だけを探す | `TestSearch` |
| 検索は `Tools` を付けるとツールの入出力も探す | `TestSearch` |
| 検索は1セッションから出す件数に上限を付けられる | `TestSearch` |

## ターンとセッション ID（`ReadTurns`・`session.Find`）

| 保証 | 対応テスト |
|---|---|
| `ReadTurns` はセッションを、人の発言1つと次の発言の前までの応答（文章・ツールの呼び出し・ツールの出力）からなるターンに分ける | `TestReadTurns` |
| `ReadTurns` はツールの出力に呼んだツールの名前を付ける | `TestReadTurns` |
| ターンの最終応答はターンの中の最後の文章である | `TestReadTurns` |
| `session.Find` はセッション ID を UUID の先頭の数文字で引く | `TestFind` |
| `session.Find` はどのセッションにも当たらない指定をエラーにする | `TestFind` |
| `session.Find` は16進とハイフン以外を含む指定をエラーにする | `TestFind` |
| `session.Find` は複数のセッションに当たる指定をエラーにする | `TestFind` |
| `session.Find` は同じ UUID が複数のプロジェクトにあるとき、更新の新しい方を返す | `TestSameUUIDInTwoProjects` |

## 人の発言に数えるもの

| 保証 | 対応テスト |
|---|---|
| compact のあとに自動で入る要約は人の発言に数えない | `TestPromptTextSkipsCompactSummary` |
| 画像を添えた発言は人の発言に数える | `TestPromptTextShapes` |
| `<pasted_content>` で始まる発言は人の発言に数える | `TestPromptTextShapes` |
| 中断を示す文言（`[Request interrupted by user]`、`for tool use` を含む）は人の発言に数えない | `TestPromptTextShapes` |
| `tool_result` を含む発言は人の発言に数えない | `TestPromptTextShapes` |
| バックグラウンドの作業の通知は人の発言に数えない | `TestPromptTextShapes` |
| コマンドの出力は人の発言に数えない | `TestPromptTextShapes` |
