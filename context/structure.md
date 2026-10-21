# 構成

```
cmd/ccreel/main.go          サブコマンドの振り分けと出力（表 / JSON Lines）。上限と切り詰めの表示もここで行う
internal/session/           jsonl の読み出し
  session.go                  Discover（セッションを探す）・Find（UUID の先頭から引く）・Scan（一覧の1行を組み立てる）・ScanAll
  transcript.go               ReadTurns（セッションをターンに分ける）
  search.go                   Candidates（rg で候補を絞る）・Search（新しい順に読んで当たった箇所を返す）
testdata/projects/          架空のセッションで作った fixture。付属物（subagents/）と発言の無いセッションを含む
.github/workflows/ci.yml    go vet と go test
```

## データの流れ

```
<config dir>/projects/*/*.jsonl
  ├─ Discover → ScanAll(Scan) → []Meta  → list
  ├─ Find → ReadTurns → []Turn          → show / outline
  └─ Candidates(rg) → Search            → search
```

config dir は `--config-dir`、無ければ `CLAUDE_CONFIG_DIR`、それも無ければ `~/.claude` を使う。

**ターン**は、人の発言1つと、次の発言の前までの応答をまとめた単位である。`show --turns` と `search` の `turn` は、ターンの番号を指す。
`show` と `search` は既定でツールの入出力を出さない（`--tools` で出す）。上限で止めたときは、その旨を stderr に書き、stdout の JSON Lines には混ぜない。

同期（push・pull）・パスの書き換え・fzf の画面はまだ無い。段取りは `PLAN.md`。
