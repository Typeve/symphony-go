package commandline

import (
	"reflect"
	"testing"
)

func TestSplitUsesDefaultForEmptyCommand(t *testing.T) {
	name, args, err := Split("   ", "codex")
	if err != nil {
		t.Fatalf("Split returned error: %v", err)
	}
	if name != "codex" {
		t.Fatalf("name = %q, want codex", name)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want empty", args)
	}
}

func TestSplitHandlesArgsAndQuotes(t *testing.T) {
	name, args, err := Split(`claude-code --provider deepseek --name "review bot" 'literal value'`, "claude")
	if err != nil {
		t.Fatalf("Split returned error: %v", err)
	}
	if name != "claude-code" {
		t.Fatalf("name = %q, want claude-code", name)
	}
	want := []string{"--provider", "deepseek", "--name", "review bot", "literal value"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestSplitRejectsUnterminatedQuote(t *testing.T) {
	_, _, err := Split(`codex "unfinished`, "codex")
	if err == nil {
		t.Fatal("Split returned nil error, want unterminated quote error")
	}
}
