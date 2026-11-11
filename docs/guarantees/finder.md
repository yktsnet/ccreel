# finder

## `ParseQuery`

| 保証 | 対応テスト |
|---|---|
| `ParseQuery` は `@名前` をリポの指定として読む | `TestParseQuery` |
| `ParseQuery` は `#12` と `12` を Issue 番号として読む | `TestParseQuery` |
| `ParseQuery` はIssue 番号を本文の語としても残す | `TestParseQuery` |

## `Find` が返す候補

| 保証 | 対応テスト |
|---|---|
| `Find` はタイトル（`/rename` で付けた名前を含む）に語が当たったセッションを返す | `TestFindByTitleAndBody`, `TestFindByCustomTitle` |
| `Find` は本文に語が当たったセッションを返す | `TestFindByTitleAndBody` |
| `Find` は当たった理由（`title:語`、`body:語×数`）を候補に添える | `TestFindByTitleAndBody`, `TestFindByCustomTitle` |
| `Find` が理由に添える本文の数は、人の発言と応答の文章のうち語を含むものの数である | `TestFindByTitleAndBody` |
| `Find` はツールの呼び出しと出力にしか無い語では本文に当てない | `TestBodyIgnoresToolOutput` |
| `Find` はブランチか worktree の名前の Issue 番号が一致するセッションを返す | `TestFindByIssue` |
| `Find` はIssue 番号で当たった候補の理由の先頭に `issue #番号` を置く | `TestFindByIssue` |
| `Find` は `@リポ` だけを渡すと、そのリポのセッションだけを返す | `TestFindByRepoOnly` |
| `Find` は `Exclude` に渡したセッション（いま話しているもの）を返さない | `TestExcludeCurrent` |
| `Find` は語も番号もリポも無ければ `ErrEmpty` を返す | `TestEmptyQuery` |

## `Find` が読む本数の上限

| 保証 | 対応テスト |
|---|---|
| `Find` は `@リポ` を読む本数の上限（40本）に絞る前に当てるので、他のリポに仮の点がより高いセッションが上限より多くあっても、そのリポのセッションを返す | `TestRepoIsAppliedBeforeTheReadLimit` |
| `Find` は `Exclude` に渡したセッションを、読む本数の上限に絞る前に外す | `TestExcludeIsAppliedBeforeTheReadLimit` |
| `Find` は `Exclude` で外したセッションを読む本数に数えない | `TestExcludeIsAppliedBeforeTheReadLimit` |

## `Find` の順位と `decided`

| 保証 | 対応テスト |
|---|---|
| `Find` は同じ履歴と同じ言葉に対して、毎回同じ候補を同じ順で返す | `TestFindIsDeterministic` |
| `Find` はタイトルに語があるセッションを、本文で語をくり返しただけのセッションより上に置く | `TestTitleOutranksBody`, `TestRanking` |
| `Find` はIssue の作業をしたセッション（ブランチ・worktree の番号が一致）を、本文で番号をくり返しただけのセッションより上に置く | `TestRanking` |
| `decided` は1位が2位の1.5倍以上で、かつ20点以上離れているときに限り真になる | `TestDecided` |
| `decided` は候補が1本だけのときは2位を0点とみなす | `TestDecided` |
