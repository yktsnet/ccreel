# VHS demo: ccreel
# 生成: リポルートで `vhs demo/demo.tape`（go・fzf・bat が PATH に要る）
# 示したいこと: 全部のリポの会話が1つの一覧に並び、`@app #417` と打つと絞り込まれ、Enter でその会話を resume する
# 素材: demo/gensessions が書く架空のセッション。claude の代わりに demo/claude を起動する
# 尺規範: 10秒前後

Output demo/demo.gif

Set FontSize 18
Set Width 1600
Set Height 700
Set Theme "Catppuccin Mocha"
Set TypingSpeed 80ms

Hide
Type "export DEMO=$(mktemp -d) PS1='$ '"
Enter
Type "go build -o $DEMO/ccreel ./cmd/ccreel && go run ./demo/gensessions $DEMO/cfg"
Enter
Type "export PATH=$DEMO:$PATH CLAUDE_CONFIG_DIR=$DEMO/cfg CCREEL_CONFIG_DIRS= CCREEL_HUB= CCREEL_CLAUDE=$PWD/demo/claude"
Enter
Sleep 8s
Type "clear"
Enter
Show

Set TypingSpeed 20ms
Type "# every repo's sessions in one list; narrow with @repo #issue, Enter resumes"
Enter
Sleep 800ms
Set TypingSpeed 80ms
Type "ccreel"
Enter
Sleep 2s
Type "@app"
Sleep 1.5s
Type " #417"
Sleep 2.5s
Enter
Sleep 2s
