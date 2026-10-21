# 構成

```
cmd/ccreel/main.go          サブコマンドの振り分けと出力（表 / JSON Lines）
internal/session/           jsonl の読み出し
  session.go                  Discover（セッションを探す）・Scan（1本から一覧の1行を組み立てる）・ScanAll（並列に読んで新しい順に並べる）
  session_test.go
testdata/projects/          架空のセッションで作った fixture。付属物（subagents/）と発言の無いセッションを含む
.github/workflows/ci.yml    go vet と go test
```

## データの流れ

```
<config dir>/projects/*/*.jsonl → Discover → ScanAll(Scan) → []Meta → list の表 / JSON Lines
```

config dir は `--config-dir`、無ければ `CLAUDE_CONFIG_DIR`、それも無ければ `~/.claude` を使う。
同期（push・pull）・パスの書き換え・fzf の画面・`search`・`show`・`outline` はまだ無い。段取りは `PLAN.md`。
