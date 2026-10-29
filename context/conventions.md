# 規約

- 依存は標準ライブラリだけにする。fzf・rsync・rg は外部コマンドとして呼び、Go のライブラリで置き換えない
- jsonl の形式は公開されていないので、知らない `type` や欠けたフィールドで失敗させない。
  読めない行は飛ばし、読めた分で組み立てる
- decode の前に行の中身で振り分ける。API の生データなどの行は数 MB になり、全行を decode すると一覧が遅くなる
- `list` などのサブコマンドは、人向けの表と `--json`（JSON Lines）の2つの出力形を持つ。
  JSON のフィールド名は公開面として扱い、変えるときは skill と README も直す
- 人が打った発言は `internal/session` の `promptText` の判定に揃える。`isMeta`・`isSidechain`・`tool_result`・コマンドの出力・中断を示す文言（`[Request interrupted by user]`）は数えない。画像を添えた発言と、貼り付け（`<pasted_content>`）で始まる発言は数える
- コメントは日本語で WHY だけを書く。実行時のメッセージ（エラー・ヘルプ）は英語で書く
