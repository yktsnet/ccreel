# ドメイン

ccreel が扱うものの名前と境界。

- **セッション**：`projects/<プロジェクト名>/<uuid>.jsonl` の1本。追記しかされず、1本は1台の端末でしか書かれない。
  UUID がセッションの同一性で、プロジェクト名は置き場にすぎない
- **付属物**：`<uuid>/` 配下の `subagents/`・`tool-results/`・`workflows/`。セッションと一緒に運ぶ。
  一覧には出さない
- **config dir**：`~/.claude`・`~/.claude-work` のように `CLAUDE_CONFIG_DIR` で切り替わる置き場。
  セッションは config dir をまたいで混ぜない
- **プロジェクト名**：起動した cwd の `/` を `-` に置き換えた名前。端末ごとにホームのパスが違う
  （`-Users-<user>` と `-home-<user>`）ので、VPS では正規の形にそろえて持つ（形は未確定。`PLAN.md`）
- **ハブ**：各端末が push し pull する VPS 上の置き場。正規化したプロジェクト名の下に、UUID ごとに1本だけ持つ
- **処理の流れ**：`push → ハブ → pull（パスの書き換え） → 読み出しの中核 → fzf の画面 / JSON 出力`
  - push: 手元の config dir からハブへ。ファイル単位で新しい方を残す
  - pull: ハブから手元の config dir へ。プロジェクト名と jsonl 内の `"cwd"` を手元のパスへ書き換え、
    手元に無い cwd は存在する祖先へ寄せる
  - 読み出しの中核: jsonl を読み、一覧・検索・本文・目次を返す。画面と JSON はここだけを通す
