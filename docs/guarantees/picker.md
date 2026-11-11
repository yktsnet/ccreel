# picker

## `Line`

| 保証 | 対応テスト |
|---|---|
| `Line` はfzf の1行を「UUID・タイトル・`@リポ #番号`・更新日・24時間以内の経過時間」のタブ区切りで作る | `TestLine` |
| `Line` はタイトルより後ろの列を薄い色で出す | `TestLine` |
| `Line` はタイトルが無ければ、最初の発言を1行にしてタイトルの代わりに使う | `TestLine` |

## `Ago` と `Elapsed`

| 保証 | 対応テスト |
|---|---|
| `Ago` は更新からの経過時間を24時間で割って切り捨てた値を日数とし、9日（216〜240時間前）までは `*` に続けて日数を返す | `TestAgo` |
| `Ago` は9日より前を週（`2w`）で返す | `TestAgo` |
| `Ago` は日付をまたいでも、24時間以内なら `*0` を返す | `TestAgo` |
| `Elapsed` は24時間以内（`*0`）のものにだけ経過時間を返し、24時間を過ぎたものは空にする | `TestElapsed` |
| `Elapsed` は1分未満を `now`、60分未満を分（`12m`）、それ以上を時間（`23h`）で返す | `TestElapsed` |

## `ResumeDir` と `selectedIDs`

| 保証 | 対応テスト |
|---|---|
| `ResumeDir` は記録された cwd が手元にあればそこを返す | `TestResumeDir` |
| `ResumeDir` は記録された cwd が手元に無ければ、手元にある一番近い祖先を返す | `TestResumeDir` |
| `ResumeDir` は祖先がホームそのもの、またはホームより上になるとき、および cwd が空のときは、ホームを返す | `TestResumeDir` |
| `selectedIDs` はfzf で複数の行を選んだとき、選んだ順にすべての行の UUID を返す | `TestSelectedIDs` |
