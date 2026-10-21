# ccreel

Claude Code のセッション履歴を端末間で共有し、fzf で選んで resume する Go の CLI。
同じ中身を JSON でも出し、Claude Code が対話の中で過去の会話を調べられるようにする。
完成条件と段取りは `PLAN.md`、設計判断は `JUDGE.md` にある。

フェーズ: **MVP期**。

@context/conventions.md
@context/structure.md
@context/domain.md

## コマンド

```bash
go build ./...
go vet ./...
go test ./...
go run ./cmd/ccreel list --config-dir testdata    # fixture に対して動かす
```

## 検証手段

`go vet ./...` と `go test ./...`。CI も同じ2つを回す。

`internal/hubsync` のテストは、手元の一時ディレクトリをハブにして rsync を実際に走らせる。ssh は使わない。

読み出しの変更は `testdata/` の fixture で確かめる。fixture は架空のセッションで作り、
実際の履歴（`~/.claude*/projects` の中身）を写さない。実際の履歴に対して動かすのは、
所要時間や件数を見るときだけにし、出力をリポに残さない。

## 実値を置かない

ハブの宛先・端末名・config dir の一覧は、設定ファイルか環境変数で外から渡す。
ホスト名・IP・ユーザー名をコード・fixture・文書に書かない。
