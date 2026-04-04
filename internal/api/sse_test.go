package api

import (
	"strings"
	"testing"
)

func TestSSEParserSingleEvent(t *testing.T) {
	parser := NewSSEParser()

	chunk := "event: message_start\ndata: {\"type\":\"message_start\"}\n\n"
	events := parser.PushChunk([]byte(chunk))

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != "message_start" {
		t.Errorf("Event = %q, want %q", events[0].Event, "message_start")
	}
	if events[0].Data != `{"type":"message_start"}` {
		t.Errorf("Data = %q", events[0].Data)
	}
}

func TestSSEParserMultipleEvents(t *testing.T) {
	parser := NewSSEParser()

	input := "event: message_start\ndata: {\"type\":\"message_start\"}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\"}\n\n"
	events := parser.PushChunk([]byte(input))

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Event != "message_start" {
		t.Errorf("Event[0] = %q", events[0].Event)
	}
	if events[1].Event != "content_block_start" {
		t.Errorf("Event[1] = %q", events[1].Event)
	}
}

func TestSSEParserPartialChunks(t *testing.T) {
	parser := NewSSEParser()

	// Send in two chunks
	events1 := parser.PushChunk([]byte("event: test\ndata: hello"))
	if len(events1) != 0 {
		t.Fatalf("expected 0 events from partial chunk, got %d", len(events1))
	}

	events2 := parser.PushChunk([]byte("\n\n"))
	if len(events2) != 1 {
		t.Fatalf("expected 1 event after completion, got %d", len(events2))
	}
	if events2[0].Event != "test" {
		t.Errorf("Event = %q, want %q", events2[0].Event, "test")
	}
	if events2[0].Data != "hello" {
		t.Errorf("Data = %q, want %q", events2[0].Data, "hello")
	}
}

func TestSSEParserMultiLineData(t *testing.T) {
	parser := NewSSEParser()

	input := "event: result\ndata: line1\ndata: line2\n\n"
	events := parser.PushChunk([]byte(input))

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	want := "line1\nline2"
	if events[0].Data != want {
		t.Errorf("Data = %q, want %q", events[0].Data, want)
	}
}

func TestSSEParserComments(t *testing.T) {
	parser := NewSSEParser()

	input := ": this is a comment\nevent: ping\ndata: pong\n\n"
	events := parser.PushChunk([]byte(input))

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != "ping" {
		t.Errorf("Event = %q", events[0].Event)
	}
}

func TestSSEParserID(t *testing.T) {
	parser := NewSSEParser()

	input := "event: msg\ndata: test\nid: 42\n\n"
	events := parser.PushChunk([]byte(input))

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "42" {
		t.Errorf("ID = %q, want %q", events[0].ID, "42")
	}
}

func TestParseSSEStream(t *testing.T) {
	input := "event: start\ndata: {\"ok\":true}\n\nevent: end\ndata: done\n\n"
	events, err := ParseSSEStream(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSSEStream error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Event != "start" {
		t.Errorf("Event[0] = %q", events[0].Event)
	}
	if events[1].Event != "end" {
		t.Errorf("Event[1] = %q", events[1].Event)
	}
}

func TestSSEParserFinish(t *testing.T) {
	parser := NewSSEParser()
	parser.PushChunk([]byte("event: last\ndata: remaining"))
	events := parser.Finish()

	if len(events) != 1 {
		t.Fatalf("expected 1 event from Finish, got %d", len(events))
	}
}

func TestSSEParserEmptyFinish(t *testing.T) {
	parser := NewSSEParser()
	events := parser.Finish()
	if len(events) != 0 {
		t.Fatalf("expected 0 events from empty Finish, got %d", len(events))
	}
}
