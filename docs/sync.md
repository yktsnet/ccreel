# Sync

セッションをハブ経由で別の端末へ運ぶ仕組みである。図は [README](../README.md) の「端末をまたぐ」にある。

## ハブでの持ち方

ハブには `<hub>/<config dir 名>/projects/<プロジェクト名>/<uuid>.jsonl` の形で置く。
プロジェクト名と jsonl の `"cwd"` の先頭のホームは `~` に置き換える（`-home-alice-src-app` → `~-src-app`）。どの端末から push しても同じセッションは同じバイト列になり、pull したものを push し直しても rsync は差分なしと判定する。

運ぶのはセッション本体の jsonl だけで、`<uuid>/` 配下の付属物（サブエージェントの記録・退避したツール出力）は運ばない。

## push

mirror は、ハブと同じ形にした手元の写しで、既定では `$XDG_CACHE_HOME/ccreel/<config dir 名>` に置く。
push は config dir と mirror を比べる。変わったセッションが無く、前回の送信も済んでいれば、ハブにつながずに数ミリ秒で終わる。送れなかった変更は mirror の `.push-pending` に残し、次の push で送り直す。

毎分呼ぶ前提なので、cron・systemd のタイマー・launchd などに登録する。タイマーからはシェルの環境変数が見えないので、ハブの宛先も一緒に渡す。

```cron
* * * * * CCREEL_HUB=<hub-host>:ccreel-hub ccreel push
```

push は config dir を1つずつ送る。config dir が複数あるなら、`--config-dir` を付けてそれぞれ呼ぶ。

## pull

pull は、ハブの一覧を更新日時つきで取り、mirror より新しいものだけを rsync する。新しいものが無ければ ssh 1回で終わる。
`ccreel`（pick）は、`CCREEL_CONFIG_DIRS` に並べた config dir ごとに pull してから fzf を開く。pull に失敗しても、手元のセッションで一覧を出す。

開くときの待ちをなくすには、push と同じタイマーで pull も呼び、`ccreel --no-pull` で開く。別の端末の最新は、タイマーの間隔の分だけ遅れて届く。

```cron
* * * * * CCREEL_HUB=<hub-host>:ccreel-hub ccreel push; CCREEL_HUB=<hub-host>:ccreel-hub ccreel pull
```

## ssh の接続

ssh は `~/.ssh/config` の多重化（`ControlMaster`）を使わず、毎回つなぎ直す。`ConnectTimeout` は新しくつなぐときにしか効かないので、固まった多重化の接続を通すと pull がいつまでも返らない。
つないだあとに応答が止まったら、ssh の `ServerAliveInterval` と rsync の `--timeout` で切る。初回の大きな転送のように、時間がかかっても進んでいるものは切らない。

## 保持期間

- pull は、ハブでの更新から30日以内のセッションだけを取る（Claude Code の保持期間に合わせた）。全部を取ると、手元で消えたセッションが pull のたびに戻ってくる
- ハブと mirror からは、更新から40日を過ぎたセッションを消す。消すのは1日に1回で、最後に消した時刻を mirror の `.last-prune` に残す

どちらも `--pull-days`・`--keep-days` で変えられる。

## 排他

push と pull は mirror の `.lock` で排他し、重なったときは待たずに止まる。初回の大きな push の最中に `ccreel` を開いても、待たされない。

## resume するディレクトリ

resume は、記録された cwd へ移ってから claude を起動する。cwd が手元に無いとき（別の端末の worktree など）は、手元にある一番近い祖先のディレクトリで開く。jsonl の中身は書き換えない。
