# pathmap

| 保証 | 対応テスト |
|---|---|
| プロジェクト名の先頭のホーム（`-home-alice`・`-Users-alice`）は `~` に置き換わり、逆の変換で元に戻る | `TestProjectRoundTrip` |
| ホームそのもののプロジェクト名は `~` になる | `TestProjectRoundTrip` |
| ホームの名前で始まるだけの別のディレクトリ（`-home-alice2-…`）のプロジェクト名は変わらない | `TestProjectRoundTrip` |
| ホームの外のプロジェクト名は変わらない | `TestProjectRoundTrip` |
| jsonl の `"cwd"` の先頭のホームは `~` に置き換わり、逆の変換で元に戻る | `TestContentRoundTrip` |
| jsonl の `"cwd"` 以外にあるパスは変わらない | `TestContentRoundTrip` |
| ホームの名前で始まるだけの別のパスの `"cwd"` は変わらない | `TestContentRoundTrip` |
| macOS で送ったものを Linux で戻すと、Linux のホームの形になる | `TestProjectRoundTrip`, `TestContentRoundTrip` |
| 戻したものを送り直すと、ハブの中身と同じバイト列になる | `TestContentRoundTrip` |
