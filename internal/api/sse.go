package api

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"
)

// SSEEvent represents a single server-sent event.
type SSEEvent struct {
	Event string
	Data  string
	ID    string
	Retry *int
}

// SSEParser incrementally parses SSE streams.
type SSEParser struct {
	buffer    bytes.Buffer
	eventName string
	dataLines []string
	id        string
	retry     *int
}

// NewSSEParser creates a new SSE parser.
func NewSSEParser() *SSEParser {
	return &SSEParser{}
}

// PushChunk processes a chunk of SSE data and returns complete events.
func (p *SSEParser) PushChunk(chunk []byte) []SSEEvent {
	p.buffer.Write(chunk)
	var events []SSEEvent

	for {
		// Look for event delimiter (double newline)
		data := p.buffer.Bytes()
		idx := bytes.Index(data, []byte("\n\n"))
		if idx == -1 {
			break
		}

		raw := data[:idx]
		p.buffer.Next(idx + 2) // skip past delimiter

		scanner := bufio.NewScanner(bytes.NewReader(raw))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, ":") {
				continue // comment
			}
			if strings.HasPrefix(line, "event: ") {
				p.eventName = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				p.dataLines = append(p.dataLines, strings.TrimPrefix(line, "data: "))
			} else if strings.HasPrefix(line, "id: ") {
				p.id = strings.TrimPrefix(line, "id: ")
			} else if strings.HasPrefix(line, "retry: ") {
				v, err := strconv.Atoi(strings.TrimPrefix(line, "retry: "))
				if err == nil {
					p.retry = &v
				}
			}
		}

		if len(p.dataLines) > 0 {
			events = append(events, SSEEvent{
				Event: p.eventName,
				Data:  strings.Join(p.dataLines, "\n"),
				ID:    p.id,
				Retry: p.retry,
			})
		}

		// Reset for next event
		p.eventName = ""
		p.dataLines = nil
		p.id = ""
		p.retry = nil
	}

	return events
}

// Finish flushes any remaining data in the buffer.
func (p *SSEParser) Finish() []SSEEvent {
	if p.buffer.Len() == 0 {
		return nil
	}
	remaining := p.buffer.String()
	p.buffer.Reset()
	if strings.TrimSpace(remaining) == "" {
		return nil
	}
	return []SSEEvent{{
		Event: p.eventName,
		Data:  strings.TrimPrefix(remaining, "data: "),
		ID:    p.id,
		Retry: p.retry,
	}}
}

// ParseSSEStream reads an entire SSE stream from a reader.
func ParseSSEStream(r io.Reader) ([]SSEEvent, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	parser := NewSSEParser()
	events := parser.PushChunk(data)
	remaining := parser.Finish()
	return append(events, remaining...), nil
}
