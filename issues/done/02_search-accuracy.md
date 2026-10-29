## PR記録: fix: stop find, list and search from missing or misreading past conversations
issue: 02 (02_search-accuracy.md)
PR: https://github.com/yktsnet/ccreel/pull/29

## 変更内容
Claude が skill から呼ぶ検索が、あるはずの会話を返さない・別の会話を選ぶ原因を直す。
- `find` は `@リポ` といまのセッションを、読む上限（40本）に絞る前に外す。絞る並びは仮の点・mtime・パスで固定し、実行のたびに候補が変わらないようにした
- `find` の本文の点は、生の jsonl の行数ではなく、人の発言と応答の文章のうち語を含むものの数にした。ツールの入出力・注入された CLAUDE.md・過去の `ccreel find` の呼び出しでは当たらない。`Scan` と同じ1回の読み出しで数える（`ScanAllTerms`）
- 候補が1本だけのときは2位を0点とみなし、20点に届かなければ `decided` にしない
- 画像を添えた発言と `<pasted_content>` で始まる発言を人の発言として数え、`[Request interrupted by user]` を数えない
- Issue 番号は、途中で Issue のブランチか worktree にいれば、最後に main へ戻っていても残す（複数あれば最後に現れたもの）
- `list` と `search` も `CCREEL_CONFIG_DIRS` のすべての config dir を探す。`projects` の無い config dir は飛ばす
- skill の `body` の説明と `search` の使い方、文書と保証台帳を合わせた

## 保証
- `@リポ` を上限の前に当てる → `TestRepoIsAppliedBeforeTheReadLimit`
- `Exclude` のセッションを上限の前に外す → `TestExcludeIsAppliedBeforeTheReadLimit`
- 毎回同じ候補と同じ順序 → `TestFindIsDeterministic`
- ツールの入出力では本文に当たらない → `TestBodyIgnoresToolOutput`, `TestScanAllTermsCountsConversationOnly`
- 1本だけでも20点未満は決め切らない → `TestDecided`
- 発言に数える形と数えない形 → `TestPromptTextShapes`
- 途中で現れた Issue 番号を残す（最後に現れたもの） → `TestIssueSurvivesReturnToMain`
- `list` と `search` はすべての config dir を探す → `TestListAndSearchLookInEveryConfigDir`
- 廃止: 1本だけなら点によらず `decided`、本文を jsonl の行数で数えること（`TestDecided`・`TestFindByTitleAndBody` を書き換えた）

## 静的確認結果
`go vet ./...` と `go test ./...` が通る。新しいテストは直す前のコードで落ちることを確かめた。
実際の履歴（2つの config dir）で、`find` の所要時間は直す前の約0.9〜1.6秒から約1.0〜1.7秒。`@リポ` を付けたときだけ、cwd を探す rg が1回増える分（約0.3秒）延びる。
Issue 番号の付くセッションは81本から280本に増え、発言の数は6195から5931になった。

## 確認者の判定
| # | 項目 | 判定 |
|---|---|---|
| 1 | `go vet ./...` と `go test ./...` が通る | 合 |
| 2 | 保証のテストが通る | 合（新しいテストは、直す前のソースではすべて落ちる） |
| 3 | 実際の履歴で `find` の所要時間が大きく延びない | 合 |

保証の各項目も、すべて「合」。初回は `Exclude` と「最後に現れた番号」を固定するテストが無く「否」だった。53353ac でテストを足し、再判定で「合」になった。
範囲の外への波及は無い。応答の text ブロックごとに数える点は、仮定の「メッセージの数」と細部で異なるが、Claude Code は1レコードに1ブロックで書くので、結果は変わらない。

---

## find・list・search が会話を取りこぼす・取り違えるのを直す
id: 02
branch-slug: search-accuracy
status: close
type: fix
対象: internal/session/session.go, internal/session/session_test.go, internal/session/search.go, internal/session/transcript_test.go, internal/finder/finder.go, internal/finder/finder_test.go, cmd/ccreel/main.go, cmd/ccreel/cli_test.go, skills/ccreel/SKILL.md, docs/search.md, docs/usage.md, docs/guarantees.md, context/structure.md, context/conventions.md
内容: Claude が skill から呼ぶ検索が、あるはずの会話を返さない・別の会話を選ぶ原因を直す。(1) `find @リポ 語` は全リポから仮の順位で40本に絞ってからリポで絞るので、対象が落ちる。同点の並びが map の順で決まり、実行のたびに結果が変わる。(2) 画像を添えた発言と `<pasted_content>` で始まる発言を人の発言として数えず、`[Request interrupted by user]` を数えている。(3) Issue 番号を最後のブランチと cwd から取るので、PR を出して worktree を消したセッションは番号を失う。(4) `find` の本文の点が生の jsonl の行を数えるので、ツールの出力・CLAUDE.md の注入・過去の `ccreel find` の呼び出しにも当たる。(5) 候補が1本だと点が低くても `decided` になる。(6) `list` と `search` が `CCREEL_CONFIG_DIRS` を見ず、`find` の候補の config dir と食い違う
対象外: `ScanAll`（jsonl の時刻）と `Find`（mtime）で、同じ UUID の重複から選ぶ基準が違うこと。`search` に1本のセッションへ絞る引数を足すこと。`ParseQuery` が番号を1つしか持たないこと
仮定: `find` の本文の数は、発言と応答の文章（ツールの入出力を除く。`search` の既定と同じ範囲）のうち語を含むメッセージの数にする。rg の生の行数は、読む40本を選ぶ仮の順位にだけ使う
確認: `go vet ./...` と `go test ./...` が通る。下の保証のテストが通る。実際の履歴に対して `find` の所要時間が直す前（約1〜1.5秒）から大きく延びない

---

### 保証
- 新たに宣言する保証:
  - `Find` は、`@リポ` と `Exclude` を、読む上限（40本）に絞る前に当てる。そのリポのセッションが他のリポの候補より仮の点で下でも返す
  - `Find` は、同じ履歴と同じ言葉に対して、毎回同じ候補を同じ順で返す
  - `Find` の本文の当たりは、発言と応答の文章だけで数え、ツールの入出力では数えない。理由は `body:語×メッセージ数`
  - 候補が1本だけでも、点が20に届かなければ `decided` を真にしない
  - 画像を添えた発言と、`<pasted_content>` で始まる発言は、人の発言として数える。`[Request interrupted by user]`（`for tool use` を含む）は数えない
  - Issue 番号は、セッションの途中で Issue のブランチか worktree にいたら、最後に main へ戻っていても残す（最後に現れた番号）
  - `list` と `search` は、`CCREEL_CONFIG_DIRS` のすべての config dir を探す
- 維持する保証:
  - `@`・`#`・語の読み分け、題の一致が本文のくり返しより上、Issue の一致が本文の番号より上
  - `list --issue`・`list --repo`・発言が1回のセッションを外すこと
  - `isMeta`・`tool_result`・コマンドの出力・compact の要約を発言に数えないこと
- 廃止する保証:
  - 候補が1本だけなら点によらず `decided` を真にすること
  - 本文の当たりを jsonl の行数で数えること（`body:語×行数`）

### internal/finder/finder.go
- `collect` に `Options` を渡す。`@リポ` があれば、`"cwd"` にそのリポの名前の区切り（`/<リポ>/`・`/<リポ>"`・`/<リポ>.`）が現れるセッションを rg で拾い、それ以外を候補から外してから40本に絞る。`Exclude` のファイルも先に外す
- 40本に絞る並びは、仮の点・mtime の新しい順・パスの順で決める
- 本文の当たり数は、読んだ候補を `session.ScanAllTerms` で読み、発言と文章の応答のうち語を含むものを数える。`Scan` と同じ1回の読み出しで数え、ファイルを読み直さない
- `decided` は、2位が無いときは2位の点を0とみなして同じ条件で判定する

### internal/session
- `promptText`: text 以外のブロックは、`tool_result` があれば発言でないとし、画像などは読み飛ばして text だけをつなぐ。`[Request interrupted by user` で始まるものを落とす。`commandText` は `<pasted_content` で始まるものをそのまま返す
- `Scan`: レコードごとにブランチと cwd から番号を出し、空でない最後のものを `Issue` にする
- `ScanAllTerms(paths, terms)` を足し、`Meta.Hits`（JSON には出さない）に語ごとの当たり数を入れる。`ScanAll` は語なしの `ScanAllTerms`
- `Candidates`: `projects` の無い config dir では、rg を呼ばずに空を返す

### cmd/ccreel/main.go
- `list` と `search` を `configDirs` の全部で探す。usage の `CCREEL_CONFIG_DIRS` の説明を直す
- `list` と `pick` で同じ「全部の config dir の Discover」を `discoverAll` にまとめる

### 文書
- `skills/ccreel/SKILL.md`: `body:<word>×<lines>` をメッセージ数に直す。`search` を「選んだセッションの中を探す」ではなく、全セッションから箇所とターン番号を引くものとして書き直す
- `docs/search.md`・`docs/usage.md`・`context/structure.md`・`context/conventions.md`・`docs/guarantees.md` を上の保証に合わせる
