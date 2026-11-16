package anthropic

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// F1 verification: the stored history of chat run 8 (object tool results, the
// shape that poisoned runs 9 and 10) encodes to a valid wire request: every
// tool_result content is a string or a list of blocks.
func TestEncodeRequest_StoredChatRun8History(t *testing.T) {
	raw, err := os.ReadFile("testdata/chat_run8_messages.json")
	if err != nil {
		t.Fatal(err)
	}
	var msgs []llm.Message
	if err := json.Unmarshal(raw, &msgs); err != nil {
		t.Fatal(err)
	}
	body, err := encodeRequest(llm.Request{Model: "claude-x", Messages: msgs}, true, wire{maxTokens: 4096})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Messages []struct {
			Content []struct {
				Type    string `json:"type"`
				Content any    `json:"content"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	results := 0
	for _, m := range got.Messages {
		for _, b := range m.Content {
			if b.Type != "tool_result" {
				continue
			}
			results++
			switch b.Content.(type) {
			case string, []any:
			default:
				t.Fatalf("tool_result content is %T, want string or blocks", b.Content)
			}
		}
	}
	if results != 2 {
		t.Fatalf("tool_result blocks = %d, want 2", results)
	}
}
