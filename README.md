[🇯🇵 日本語](README.md) | [🇬🇧 English](README.en.md)

# ccreel

[![CI](https://github.com/yktsnet/ccreel/actions/workflows/ci.yml/badge.svg)](https://github.com/yktsnet/ccreel/actions/workflows/ci.yml)

Claude Code の過去の会話を全部のリポから1つの一覧で探し、Enter ですぐ resume する CLI。
Claude Code 自身も1回の呼び出しで正しい会話を引ける。
ssh でつながる置き場（ハブ）を足せば、ほかの端末の会話も同じ一覧に並ぶ。

![demo: 全部のリポの会話から @app #417 で絞り込み、Enter で resume する](demo/demo.gif)

## Why

### 一覧から選んですぐ resume する

`ccreel` と打つと、全部のリポ・全部の config dir のセッションが1行に1本ずつ新しい順に並ぶ。fzf は全画面で開き、右のペインには選んでいる会話の中身を出す。
打った文字がそのまま絞り込みになる。`@app` でリポ、`#417` で Issue 番号、`*0` で直近24時間のものに絞れ、組み合わせてもよい。worktree で作業した会話は元のリポにまとまる。
Enter で、その会話で最後に作業していたディレクトリへ移って resume する。Tab で複数を選ぶと、2本目からは tmux の新しいウィンドウで開く。

Claude Code の `/resume` は1本に2行を使い、会話の中身はプレビューに切り替えないと見えない。既定ではいまのディレクトリのセッションだけを出す。
会話を読むための道具（[claude-history](https://github.com/raine/claude-history) など）では、選ぶと読む画面が開き、resume は別のキーで行う。

### Claude Code に過去の会話を探させる

Claude Code に「前に話したあの件」を探させたとき、候補の順位だけが返ってくると、最初に当たった1本を深く読み、続きのセッションを見落とすことがある。
履歴を探す既存の skill や MCP サーバー（claude-history・[claude-historian-mcp](https://github.com/Vvkmnn/claude-historian-mcp) など）は、点数つきの順位を返し、どれを読むかを Claude Code に委ねる。

同梱の skill を入れると、Claude Code は `ccreel find` を1回呼ぶ。ccreel が候補に点を付け、1位が2位を十分に引き離していれば、その1本で決まりという意味の `decided: true` を返す。
Claude Code はその1本を読み、決まらなければ読む前に候補を見せて聞く。

```
> #417 の続きをやりたい

⏺ ccreel find 417 --json
  → decided: true  1c0e662e  @app #417「Lock invoices after month close」  issue #417 · body:417×6
⏺ ccreel outline 1c0e662e --json → ccreel show 1c0e662e --last 3 --json
  （直前の3ターンを読んで、続きから答える）
```

人が打つ `ccreel pick <言葉>` も `find` と同じ順位で並ぶ。点の付け方と判定の条件は [docs/search.md](docs/search.md) にある。

## 複数の端末で使う

macOS で始めた会話はそのままでは Linux の端末の `claude --resume` に出てこない。
ccreel では各端末がハブへ push し、`ccreel` を開くときに pull する。運んだ会話も手元の会話と同じ一覧と検索に載る。ハブは ssh と rsync が使えれば何でもよく、ccreel を入れる必要はない。
ハブを常時動く Linux サーバーに置き、そこで Claude Code を Remote Control で動かすと、スマホの Claude アプリから全部の端末の会話を探せる。会話を書いた端末が起動していなくてもよい。

```mermaid
flowchart TD
  subgraph A["端末 A（macOS）"]
    ca(["claude"])
  end
  subgraph S["ハブのサーバー（VPS など）"]
    hub[("ハブ")]
    rc(["claude（Remote Control）"])
  end
  subgraph B["端末 B（Linux）"]
    fb{{"ccreel (fzf)"}} ==> rb(["claude --resume"])
  end
  phone(["📱 Claude アプリ"])
  ca -.->|push| hub -.->|pull| fb
  phone -.-> rc -->|find| hub
```

ホームのパスが違う端末どうしで同じセッションとして扱う仕組みは、[docs/sync.md](docs/sync.md) にある。

## Quick Start

### 1台で使う

手元に `claude`・`fzf`・`rg` が要る（Nix で入れれば一緒に入る）。

```sh
nix profile install github:yktsnet/ccreel
# または
go install github.com/yktsnet/ccreel/cmd/ccreel@latest    # Go 1.24 以上
```

Claude Code から使うには、同梱の skill を入れる（Nix で入れた場合は `share/ccreel/skills/ccreel` にある）。

```sh
cp -r skills/ccreel ~/.claude/skills/
```

あとは `ccreel` と打つ。

### 複数の端末で使う

各端末には `rsync`・`ssh` も要る。ハブには ssh の鍵認証でログインでき、`rsync`・`sh`・`find`・`stat` があれば足りる。
各端末でハブの宛先を決め、push を毎分呼ぶ。変わったものが無い回はハブにつながずに終わる。

```sh
export CCREEL_HUB=<hub-host>:ccreel-hub     # ssh の先。手元のディレクトリも指定できる
ccreel push                                 # 初回は手元の履歴を全部送る
```

```cron
* * * * * CCREEL_HUB=<hub-host>:ccreel-hub ccreel push
```

ハブを置いたサーバーにスマホからつなぐなら、サーバーにも ccreel と skill を入れ、宛先には手元のディレクトリを渡して push だけを毎分呼ぶ。
サーバーは pull しない。代わりに `find`・`search`・`show` などの読むだけのコマンドが、ハブを直に読む（[docs/sync.md](docs/sync.md#ハブを置いたサーバーで読む)）。

```sh
export CCREEL_HUB=$HOME/ccreel-hub          # ハブ自身なので手元のディレクトリを渡す
claude --remote-control                     # スマホの Claude アプリからつなぐ
```

```cron
* * * * * CCREEL_HUB=$HOME/ccreel-hub ccreel push
```

## Usage

```sh
ccreel                          # pull してから fzf を開く（= ccreel pick）
ccreel pick 締め処理            # 言葉に当たったセッションだけを、find と同じ順位で並べる
ccreel resume 1c0e662e          # ID の先頭で引いて resume する

ccreel find 417 @app --json     # 順位付きの候補と、決め切れたかどうか
ccreel list --since 1d --json   # 直近24時間の作業の一覧
ccreel outline 1c0e662e --json  # 1ターン1行の目次
ccreel show 1c0e662e --last 3 --json
ccreel search "締め処理" --json
```

fzf の操作・JSON の項目・環境変数・フラグは [docs/usage.md](docs/usage.md) にある。

`list --json` と `show --json` を読めば、ほかの道具からもセッションを扱える。たとえば [codeview](https://github.com/yktsnet/codeview#extensions) には、セッションの一覧と本文を、コードや Issue と同じサイトのページにする拡張の例がある。

## Design Decisions

- **1本に決め切れたかを CLI が点差で判定する**: Claude Code に判断させず、同じ履歴に同じ依頼をすれば、同じ候補と同じ判定が返る
- **索引 DB を持たず、rg で候補を絞ってから読む**: 手元の履歴（約1.4GB）で `find` が1秒かからない
- **ハブは置き場だけにし、一覧・検索・resume は手元の config dir にある会話で行う**: 検索が速く、オフラインでも動き、Claude Code が何度呼んでもハブまで行かない

点の付け方は [docs/search.md](docs/search.md)、ほかの判断と技術の選定理由は [docs/design-decisions.md](docs/design-decisions.md) にある。

## Scope

macOS と Linux（WSL を含む）で、2台の端末と2つの config dir で使っている。ハブを置いた Linux サーバーは、スマホからつなぐ先も兼ねている。

1台で使うとき:

- 一覧には `claude -p` で動かしたセッションと、発言が1回だけのセッションを出さない（発言が1回のものは `--all` で出る）
- 一覧の `#番号` はブランチ名か worktree の名前に入れた番号から取る
- MCP サーバー・索引 DB・LLM によるタイトルの生成は持たない

複数の端末で使うとき:

- 同じセッションを2台で同時に書くと、ハブでは更新の新しい方が残る
- 手元へ pull するのは直近30日まで。それより古いセッションはハブにだけある
- ハブにしか無いセッションは、ハブを置いたサーバーで読めるが、resume はできない
- 付属物（サブエージェントの記録・退避したツール出力）は運ばない

テストで裏付けた振る舞いは [docs/guarantees.md](docs/guarantees.md) にある。そこに載っていない振る舞いは、予告なく変わることがある。

## License

[MIT](LICENSE)
