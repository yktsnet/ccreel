# 構成

```
cmd/ccreel/main.go          サブコマンドの振り分けと出力（表 / JSON Lines）。上限と切り詰めの表示もここで行う
internal/session/           jsonl の読み出し
  session.go                  Discover（セッションを探す）・Find（UUID の先頭から引く）・Scan（一覧の1行を組み立てる）・ScanAll
  transcript.go               ReadTurns（セッションをターンに分ける）
  search.go                   Candidates（rg で候補を絞る）・Search（新しい順に読んで当たった箇所を返す）
internal/pathmap/           プロジェクト名と jsonl の "cwd" の先頭のホームを、端末の形と `~` の形で相互に変換する
internal/finder/            言葉からセッションを探して順位を付ける（find と、言葉を渡した pick が使う）
internal/hubsync/           push・pull とハブの古いセッションの削除
  hub.go                      ハブ（host:path か手元のディレクトリ）への rsync と、ハブの側で走らせる sh のスクリプト
  hubsync.go                  Push・Pull。運ぶのはセッション本体の jsonl だけ。手元とハブのあいだに、ハブと同じ形の写し（mirror）を持つ
internal/picker/            fzf で選ばせる（Pick）・一覧の1行と更新日の表示（Line・Ago）・resume の前に移るディレクトリを決める（ResumeDir）・画面を消して claude を exec する（Resume）
skills/ccreel/SKILL.md      利用者が Claude Code に入れる skill。どの場面でどのサブコマンドを呼ぶかを書く（本文は英語）
testdata/projects/          架空のセッションで作った fixture。付属物（subagents/）と発言の無いセッションを含む
.github/workflows/ci.yml    go vet と go test
flake.nix                   Nix のパッケージ。fzf・rg・rsync・ssh を PATH の後ろに足し、skill を share/ccreel/skills/ に置く
```

## データの流れ

```
<config dir>/projects/*/*.jsonl
  ├─ Discover → ScanAll(Scan) → []Meta  → list
  ├─ Find → ReadTurns → []Turn          → show / outline
  ├─ Candidates(rg) → Search            → search
  ├─ ParseQuery → collect(rg) → score → decide → find
  └─ pull → Discover（言葉があれば finder の順位）→ ScanAll → fzf → Find → ResumeDir → claude --resume → pick

push: <config dir>/projects → mirror（ContentToHub）→ rsync → <hub>/<config dir 名>/projects
pull: <hub>/<config dir 名>/projects → rsync（直近30日）→ mirror → <config dir>/projects（ContentFromHub）
```

mirror は既定で `$XDG_CACHE_HOME/ccreel/<config dir 名>` に置く。mirror に書いたファイルには元の更新日時を付け直す。
pull で運んだものを push しても、rsync が差分なしと判定するためである。

push はタイマーから毎分呼ばれる前提で作っている。変わったセッションが無く、前回の送信も済んでいれば、ハブにはつながずに終える。
送れなかった変更は mirror の `.push-pending` に記録しておき、次の push で送り直す。ハブと mirror から更新後40日を過ぎたセッションを消すのは1日に1回で、
最後に消した時刻を `.last-prune` に残す。push と pull は mirror の `.lock` で排他し、重なったときは待たずに止める。
pull は、ハブの一覧を更新日時つきで取り、手元の mirror よりハブの方が新しいものだけを rsync する。新しいものが無ければ rsync を起動せず、ssh 1回で終わる。

picker の右のペインには `ccreel preview <id>` が出す Markdown を表示し、bat があれば色を付ける。fzf は全画面で開く。更新日は9日前まで `*0`（今日）・`*1`（昨日）の形で出す。`@リポ`・`#番号` と同じく、この形を入力して一覧を絞り込める。

引数なしの `ccreel` は `pick` と同じ。pull は、ハブが設定されていて `--no-pull` を付けていないときだけ行う。pull に失敗しても、手元のセッションで一覧を出す。

config dir は `--config-dir`、無ければ `CLAUDE_CONFIG_DIR`、それも無ければ `~/.claude` を使う。
`find` と、ID からセッションを引く `show`・`outline`・`resume` は、`CCREEL_CONFIG_DIRS`（コロン区切り）の config dir も探す。

`find` は、Issue 番号の一致（100）、題の一致（語ごとに30）、本文の当たり行数（語ごとに行数の対数で最大20）、
全部の語に当たったとき（15）、find を呼んだ場所と同じリポのとき（10）、新しさ（最大5）を足して点にする。1位が2位の1.5倍以上で、かつ20点以上離れていれば
`decided` にする。番号はツールの出力の数字にも当たり、候補が数百本になることがある。全部を読むと遅いので、rg の当たりで仮の順位を付け、上位40本に絞って読む。

**ターン**は、人の発言1つと、次の発言の前までの応答をまとめた単位である。`show --turns` と `search` の `turn` は、ターンの番号を指す。
`show` と `search` は既定でツールの入出力を出さない（`--tools` で出す）。上限で止めたときは、その旨を stderr に書き、stdout の JSON Lines には混ぜない。

段取りは `PLAN.md` にある。
