# hubsync

## `Push` が送るもの

| 保証 | 対応テスト |
|---|---|
| `Push` はセッション本体の jsonl をハブへ送る | `TestPushThenPullAcrossMachines` |
| `Push` は `<uuid>/` の付属物と `memory/` を送らない | `TestPushThenPullAcrossMachines` |
| `Push` はハブではプロジェクト名と `"cwd"` のホームを `~` に置き換える | `TestPushThenPullAcrossMachines` |
| `Pull` したセッションを `Push` し直しても、ハブの中身も更新日時も変わらない | `TestPushThenPullAcrossMachines` |
| `Push` は最後の更新から `KeepDays` を過ぎた手元のセッションを送らない | `TestPullWindowAndPrune` |

## `Pull` が取るもの

| 保証 | 対応テスト |
|---|---|
| `Pull` は別の端末が送ったセッションを手元の形（自分のホーム）に戻して config dir に書く | `TestPushThenPullAcrossMachines` |
| `Pull` は書いたセッションの元の更新日時を保つ | `TestPushThenPullAcrossMachines` |
| `Pull` は手元の写しよりハブの方が新しいものが無ければ、ハブから何も取らない | `TestPushThenPullAcrossMachines` |
| `Pull` は手元の方が新しいセッションを上書きしない | `TestNewerSideWins` |
| 手元で追記したセッションは、`Push` すると相手の端末の `Pull` で届く | `TestNewerSideWins` |
| `Pull` はハブでの更新から `PullDays`（既定30日）以内のセッションだけを取る | `TestPullWindowAndPrune` |

## `Push` がハブにつなぐ回と古いものの削除

| 保証 | 対応テスト |
|---|---|
| `Push` はハブにある、最後の更新から `KeepDays`（既定40日）を過ぎたセッションを消す | `TestPullWindowAndPrune` |
| `Push` は変わったセッションが無く、前回の送信も済んでいれば、ハブにつながない | `TestPushOnlyWhenChanged` |
| `Push` は送れなかった変更を、次の `Push` で変わったものが無くても送り直す | `TestPushOnlyWhenChanged` |
| `Push` は古いものを消す処理を1日に1回だけ実行する | `TestPushOnlyWhenChanged` |
| `Force` を付けた `Push` は変わったものが無くても送る | `TestPushOnlyWhenChanged` |

## `Push` と `Pull` のつなぎ方

| 保証 | 対応テスト |
|---|---|
| `Push` と `Pull` は同じ mirror を別の `Push` か `Pull` が使っているあいだは、待たずに `ErrBusy` を返す | `TestLockedMirror` |
| `Push` と `Pull` はssh の先のハブに `~/.ssh/config` の多重化を使わずにつなぐ | `TestRemoteHubDoesNotWaitForever` |
| `Push` と `Pull` は応答の止まった接続と転送を時間で打ち切る | `TestRemoteHubDoesNotWaitForever` |

## `ParseHub`

| 保証 | 対応テスト |
|---|---|
| `ParseHub` は最初の `/` より前に `:` があれば `host:path`（ssh の先）として読む | `TestParseHub` |
| `ParseHub` は最初の `/` より前に `:` が無ければ手元のディレクトリとして読む | `TestParseHub` |
| `ParseHub` は空の指定をエラーにする | `TestParseHub` |
