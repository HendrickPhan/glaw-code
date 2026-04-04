package agent

import (
	"context"
	"testing"
	"time"
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
		t.Run(tt.input, func(t *testing.T) {
			if got := IsValidAgentType(tt.input); got != tt.want {
				t.Errorf("IsValidAgentType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAgentTypeAllowedTools(t *testing.T) {
	tests := []struct {
		agentType AgentType
		wantBash  bool
		wantRead  bool
		wantWrite bool
	}{
		{TypeExplore, false, true, false},
		{TypePlan, false, true, false},
		{TypeVerification, true, true, false},
		{TypeGeneral, true, true, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.agentType), func(t *testing.T) {
			tools := tt.agentType.AllowedTools()
			has := func(name string) bool {
				for _, n := range tools {
					if n == name {
						return true
					}
				}
				return false
			}
			if got := has("bash"); got != tt.wantBash {
				t.Errorf("AllowedTools() has bash = %v, want %v", got, tt.wantBash)
			}
			if got := has("read_file"); got != tt.wantRead {
				t.Errorf("AllowedTools() has read_file = %v, want %v", got, tt.wantRead)
			}
			if got := has("write_file"); got != tt.wantWrite {
				t.Errorf("AllowedTools() has write_file = %v, want %v", got, tt.wantWrite)
			}
		})
	}
}

func TestManagerSpawn(t *testing.T) {
	mgr := NewManager(nil)

	a, err := mgr.Spawn(context.Background(), "list files in project", "Explore")
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected non-empty agent ID")
	}
	if a.Type != TypeExplore {
		t.Fatalf("expected type Explore, got %q", a.Type)
	}
	if a.Status != StatusPending && a.Status != StatusRunning {
		t.Fatalf("expected pending or running status, got %q", a.Status)
	}

	// Wait for the agent to finish.
	result := a.Wait()
	if result.Error != nil {
		t.Fatalf("agent finished with error: %v", result.Error)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestManagerSpawnInvalidType(t *testing.T) {
	mgr := NewManager(nil)
	_, err := mgr.Spawn(context.Background(), "test", "invalid-type")
	if err == nil {
		t.Fatal("expected error for invalid agent type")
	}
}

func TestManagerList(t *testing.T) {
	mgr := NewManager(nil)

	_, _ = mgr.Spawn(context.Background(), "task 1", "Explore")
	_, _ = mgr.Spawn(context.Background(), "task 2", "Plan")

	// Give agents a moment to register.
	time.Sleep(50 * time.Millisecond)

	list := mgr.List()
	if len(list) < 2 {
		t.Fatalf("expected at least 2 agents, got %d", len(list))
	}
}

func TestManagerGet(t *testing.T) {
	mgr := NewManager(nil)

	a, _ := mgr.Spawn(context.Background(), "test task", "Verification")

	found, err := mgr.Get(a.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found.ID != a.ID {
		t.Fatalf("expected ID %q, got %q", a.ID, found.ID)
	}

	_, err = mgr.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestManagerCancel(t *testing.T) {
	mgr := NewManager(nil)

	a, _ := mgr.Spawn(context.Background(), "cancel me", "general-purpose")

	err := mgr.Cancel(a.ID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	// Wait for the agent to finish after cancellation.
	result := a.Wait()
	if result == nil {
		t.Fatal("expected a result after cancellation")
	}
}

func TestManagerWait(t *testing.T) {
	mgr := NewManager(nil)

	a, _ := mgr.Spawn(context.Background(), "wait for me", "Explore")

	result, err := mgr.Wait(a.ID)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output from Wait()")
	}
}

func TestManagerShutdown(t *testing.T) {
	mgr := NewManager(nil)

	_, _ = mgr.Spawn(context.Background(), "task a", "Explore")
	_, _ = mgr.Spawn(context.Background(), "task b", "Plan")
	_, _ = mgr.Spawn(context.Background(), "task c", "Verification")

	mgr.Shutdown()

	// After shutdown, all agents should be in a terminal state.
	for _, s := range mgr.List() {
		if s.Status != StatusCompleted && s.Status != StatusCancelled {
			t.Errorf("agent %q has non-terminal status %q after shutdown", s.ID, s.Status)
		}
	}
}

func TestManagerBringToFront(t *testing.T) {
	mgr := NewManager(nil)

	a, _ := mgr.Spawn(context.Background(), "background task", "general-purpose")

	result, err := mgr.BringToFront(a.ID)
	if err != nil {
		t.Fatalf("BringToFront() error = %v", err)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output from BringToFront()")
	}
}

func TestManagerSendToBackground(t *testing.T) {
	mgr := NewManager(nil)

	a, _ := mgr.Spawn(context.Background(), "bg task", "Explore")

	// Agent may already be completed by the time we send it to background,
	// but the call should not panic.
	_ = mgr.SendToBackground(a.ID)
}

func TestAgentStatusSnapshot(t *testing.T) {
	mgr := NewManager(nil)

	a, _ := mgr.Spawn(context.Background(), "snapshot test", "Explore")

	snap := a.StatusSnapshot()
	if snap.ID != a.ID {
		t.Fatalf("expected ID %q, got %q", a.ID, snap.ID)
	}
	if snap.Type != "Explore" {
		t.Fatalf("expected type Explore, got %q", snap.Type)
	}
	if snap.StartTime.IsZero() {
		t.Fatal("expected non-zero start time")
	}

	// Wait for completion, then check EndTime.
	a.Wait()
	snap = a.StatusSnapshot()
	if snap.EndTime == nil {
		t.Fatal("expected non-nil end time after completion")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"abc", 3, "abc"},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}
