# 構成

```
cmd/ccreel/main.go          サブコマンドの振り分けと出力（表 / JSON Lines）。上限と切り詰めの表示もここで行う
internal/session/           jsonl の読み出し
  session.go                  Discover（セッションを探す）・Find（UUID の先頭から引く）・Scan（一覧の1行を組み立てる）・ScanAll
  transcript.go               ReadTurns（セッションをターンに分ける）
  search.go                   Candidates（rg で候補を絞る）・Search（新しい順に読んで当たった箇所を返す）
internal/pathmap/           プロジェクト名と jsonl の "cwd" の先頭のホームを、端末の形と `~` の形で相互に変換する
internal/hubsync/           push・pull とハブの古いセッションの削除
  hub.go                      ハブ（host:path か手元のディレクトリ）への rsync と、ハブの側で走らせる sh のスクリプト
  hubsync.go                  Push・Pull。手元とハブのあいだに、ハブと同じ形の写し（mirror）を持つ
testdata/projects/          架空のセッションで作った fixture。付属物（subagents/）と発言の無いセッションを含む
.github/workflows/ci.yml    go vet と go test
```

## データの流れ

```
<config dir>/projects/*/*.jsonl
  ├─ Discover → ScanAll(Scan) → []Meta  → list
  ├─ Find → ReadTurns → []Turn          → show / outline
  └─ Candidates(rg) → Search            → search

push: <config dir>/projects → mirror（ContentToHub）→ rsync → <hub>/<config dir 名>/projects
pull: <hub>/<config dir 名>/projects → rsync（直近30日）→ mirror → <config dir>/projects（ContentFromHub）
```

mirror は既定で `$XDG_CACHE_HOME/ccreel/<config dir 名>` に置く。mirror に書いたファイルには元の更新日時を付け直す。
pull で運んだものを push しても、rsync が差分なしと判定するためである。

config dir は `--config-dir`、無ければ `CLAUDE_CONFIG_DIR`、それも無ければ `~/.claude` を使う。

**ターン**は、人の発言1つと、次の発言の前までの応答をまとめた単位である。`show --turns` と `search` の `turn` は、ターンの番号を指す。
`show` と `search` は既定でツールの入出力を出さない（`--tools` で出す）。上限で止めたときは、その旨を stderr に書き、stdout の JSON Lines には混ぜない。

fzf の画面と resume はまだ無い。段取りは `PLAN.md` にある。
