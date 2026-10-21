# ccreel

Claude Code のセッション履歴を複数の端末で VPS 経由で共有し、fzf で選んで resume する CLI。
同じ中身を JSON でも出し、Claude Code が対話の中で過去の会話を調べて使えるようにする。

## 何のために作るか

- **種別**: Type B 利用保証型（導入して日々の resume と履歴検索に使わせるため）
- **使う人**: まず自分（macOS と WSL の2台）。小さな OSS として公開する
- **解く問題**: 履歴は端末ごとの config dir にしか無く、別の端末で始めた会話を resume できない。
  いまは端末どうしの rsync と claude-history で凌いでいるが、相手の端末がスリープしていると履歴が欠ける。
  過去の会話を Claude Code から調べる手段（ctx・`claude-history agent`）も別の道具に分かれている

## 完成条件（DoD）

これが全部満たされたら MVP は終わり。ここに無いものは作らない。

- [ ] README が Type B の下限を満たす（Installation・Usage・設定の参照・対応範囲と非対応）
- [ ] 2台の端末と2つの config dir で、`c` → history を ccreel に差し替えて1日使える。
      片方の端末で始めたセッションを、もう片方で選んで resume できる
- [ ] 同梱の skill を入れた Claude Code が、ctx-history-search で頼んでいた3つの用途
      （トピック検索・直前の続きを読む・今日の作業の振り返り）を ccreel だけでこなし、ctx を外せる
- [ ] 同じ3つの用途で ctx と ccreel にかかる時間を測った比較表が、README の本文に出ている
- [ ] 架空のセッションで撮った GIF を見れば、第三者が30秒で「別の端末の会話を選んで続けられる」と分かる

## やらないこと

| 項目 | 理由 |
|---|---|
| MCP サーバ | CLI と skill で足りるか先に確かめる。要るとなったら足す |
| 索引 DB（SQLite など） | `rg -l` で絞ってから読む方式で間に合うか先に確かめる |
| LLM による題の生成 | `ai-title` があり、無ければ最初の発言で足りる |
| 3台以上の端末・Windows ネイティブ | 自分の環境で確かめられない |
| goreleaser・Homebrew tap | 配布は当面 clone と Nix flake。利用者が現れたら足す |
| 端末どうしの直接同期 | VPS をハブにする |

## 概念

概念は `context/domain.md` にある。

## 構成

```
cmd/ccreel/        エントリポイントとサブコマンドの振り分け
internal/session/  jsonl の読み出し（一覧の行・本文・目次・検索の該当箇所）
internal/hubsync/  push・pull・ハブの古いセッションの削除（rsync と ssh を外部コマンドとして呼ぶ）
internal/pathmap/  プロジェクト名と cwd の書き換え
internal/picker/   fzf の起動とプレビュー
skills/ccreel/     同梱の Claude Code skill
testdata/          匿名化した fixture（実際の履歴は入れない）
```

## データ設計

- ハブの置き場: `<hub-root>/<config dir 名>/projects/<~ に置き換えたプロジェクト名>/<uuid>.jsonl`。付属物は置かない
- 設定: ハブの宛先・端末名・config dir の一覧を、設定ファイルか環境変数で外から渡す。実値はリポに入れない
- JSON 出力: `list`・`search`・`show`・`outline` の出力形を固定する。ID は UUID の先頭の数文字でも引ける

## 使い方

```
ccreel                      # pull してから fzf を開く。Enter で cwd へ移って claude --resume
ccreel push                 # 手元の新しいセッションをハブへ送る。初回は手元の履歴を全部送る
ccreel pull
ccreel list --since 1d --repo ccreel --json
ccreel search "fzf preview" --json --limit 20
ccreel show 1c0e662e --last 3 --no-tools --json
ccreel outline 1c0e662e --json
```

## 段取り

| # | 作業 | 前提 | 完了の目安 |
|---|---|---|---|
| 0 | ctx の所要時間を測る（3つの用途それぞれ、取り込みの時間を含めて） | なし | ctx を撤去する前に、手順と数字が `bench/` に残っている |
| 1 | 足場（repo-standardize）と private リポ | なし | 決定的チェックが通り、`yktsnet/ccreel` に push 済み |
| 2 | 匿名化した fixture | 1 | 実データの形を写した fixture で `go test` が回る |
| 3 | 読み出しの中核と `list`・`show`・`outline` の JSON | 2 | fixture に対する出力がテストで固定される |
| 4 | `search`（`rg -l` → 該当箇所と前後の文脈） | 3 | 件数と文字数の上限、切り詰めの表示がある |
| 5 | パスの書き換え | 2 | 端末の形と `~` の形の相互変換（プロジェクト名と cwd）がテストで固定される |
| 6 | push・pull・ハブの古いセッションの削除 | 5 | 2台と2つの config dir で push と pull を往復させ、同じセッションが重複せず1本になる。ハブから40日より古いものが消える |
| 7 | fzf の画面と resume | 3, 6 | 別の端末で始めたセッションを選んで続けられる |
| 8 | 同梱の skill | 4 | DoD の3つの用途を skill 経由でこなせる |
| 9 | dotfiles 側の配線（flake input・`c` の差し替え・push のきっかけ・ctx と ctx-history-search の撤去） | 7, 8 | 1日使える |
| 10 | デモ GIF と README（0 と同じ手順で ccreel を測り、比較表を載せる） | 0, 7, 8 | DoD の残りが満たされる |

## 未確定の項目

- push のきっかけ（セッション終了のフック・タイマー・`c` の起動時）
- 一覧の1行に出す「リポ」を、cwd の末尾から取るか、git のルートから取るか
- `memory/` などセッション以外のディレクトリを運ぶかどうか（いまは運ばない想定）
