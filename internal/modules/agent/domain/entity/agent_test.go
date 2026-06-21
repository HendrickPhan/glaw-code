package entity

import (
	"testing"
)

func TestIsValidAgentType(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"general-purpose", true},
		{"Explore", true},
		{"Plan", true},
		{"Verification", true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsValidAgentType(tt.input); got != tt.want {
			if got {
				t.Errorf("IsValidAgentType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	}
}

func TestAgentTypeAllowedTools(t *testing.T) {
	at := AgentType("Explore")
	tools := at.AllowedTools()
	if len(tools) == 0 {
		t.Error("Explore agent should have allowed tools")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello world", 20, "hello world"},
		{"hello world", 8, "hello..."},
		{"hello", 5, "hello"},
		{"hello", 4, "h..."},
	}
	for _, tt := range tests {
		got := Truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}
