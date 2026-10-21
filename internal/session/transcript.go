package session

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Turn は人の発言1つと、次の発言までの応答のまとまり。show・outline・search の単位。
type Turn struct {
	Index    int       `json:"turn"`
	Time     time.Time `json:"time"`
	Prompt   string    `json:"prompt"`
	Messages []Message `json:"messages,omitempty"`
}

// Message は応答の1片。Kind は text・tool_use・tool_result のどれか。
type Message struct {
	Kind string `json:"kind"`
	Tool string `json:"tool,omitempty"`
	Text string `json:"text"`
}

// FinalText はそのターンの最後の文章の応答。ツールの往復を除いた結論にあたる。
func (t Turn) FinalText() string {
	for i := len(t.Messages) - 1; i >= 0; i-- {
		if t.Messages[i].Kind == "text" {
			return t.Messages[i].Text
		}
	}
	return ""
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
	Content   json.RawMessage `json:"content"`
}

// ReadTurns はセッション1本をターンに分ける。最初の発言より前の応答は捨てる。
func ReadTurns(path string) ([]Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var turns []Turn
	toolNames := map[string]string{}
	add := func(m Message) {
		if len(turns) == 0 || m.Text == "" {
			return
		}
		t := &turns[len(turns)-1]
		t.Messages = append(t.Messages, m)
	}
	err = eachRecord(f, func(r record) {
		if r.IsSidechain {
			return
		}
		switch r.Type {
		case "user":
			if text, ok := promptText(r); ok {
				turns = append(turns, Turn{Index: len(turns) + 1, Time: r.Timestamp, Prompt: text})
				return
			}
			if r.IsMeta {
				return
			}
			for _, b := range contentBlocks(r.Message) {
				if b.Type == "tool_result" {
					add(Message{Kind: "tool_result", Tool: toolNames[b.ToolUseID], Text: resultText(b.Content)})
				}
			}
		case "assistant":
			for _, b := range contentBlocks(r.Message) {
				switch b.Type {
				case "text":
					add(Message{Kind: "text", Text: strings.TrimSpace(b.Text)})
				case "tool_use":
					toolNames[b.ID] = b.Name
					add(Message{Kind: "tool_use", Tool: b.Name, Text: toolInput(b.Input)})
				}
			}
		}
	})
	return turns, err
}

func contentBlocks(raw json.RawMessage) []block {
	var msg message
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	var blocks []block
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	return blocks
}

// tool_result の content は文字列のことも、text ブロックの配列のこともある。
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// ツールの入力は、何を対象にしたかが分かる代表のフィールドだけを出す。無ければ JSON のまま返す。
func toolInput(raw json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(raw, &in) != nil {
		return ""
	}
	for _, k := range []string{"command", "file_path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := in[k].(string); ok && v != "" {
			return v
		}
	}
	return string(raw)
}
