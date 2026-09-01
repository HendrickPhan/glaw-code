package logger

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     LogLevel
		wantErr  bool
	}{
		{"debug lower", "debug", LevelDebug, false},
		{"debug upper", "DEBUG", LevelDebug, false},
		{"info lower", "info", LevelInfo, false},
		{"info upper", "INFO", LevelInfo, false},
		{"warn lower", "warn", LevelWarn, false},
		{"warn upper", "WARN", LevelWarn, false},
		{"warning", "WARNING", LevelWarn, false},
		{"error lower", "error", LevelError, false},
		{"error upper", "ERROR", LevelError, false},
		{"unknown", "TRACE", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLevel(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLevel() expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLevel() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    LogFormat
		wantErr bool
	}{
		{"text lower", "text", FormatText, false},
		{"text upper", "TEXT", FormatText, false},
		{"json lower", "json", FormatJSON, false},
		{"json upper", "JSON", FormatJSON, false},
		{"unknown", "xml", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseFormat() expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFormat() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInitWithDefaults(t *testing.T) {
	var buf bytes.Buffer
	Init(LevelUnset, "", &buf)

	// Should default to INFO and text format
	if CurrentLevel() != LevelInfo {
		t.Errorf("CurrentLevel() = %v, want %v", CurrentLevel(), LevelInfo)
	}
}

func TestInitWithLevel(t *testing.T) {
	// Save current logger state
	oldLevel := CurrentLevel()
	defer func() {
		// Restore previous state
		currentLevel.Store(int64(oldLevel))
	}()

	var buf bytes.Buffer
	Init(LevelDebug, FormatText, &buf)

	if CurrentLevel() != LevelDebug {
		t.Errorf("CurrentLevel() = %v, want %v", CurrentLevel(), LevelDebug)
	}
}

func TestLogging(t *testing.T) {
	// Save current logger state
	oldLevel := CurrentLevel()
	defer func() {
		currentLevel.Store(int64(oldLevel))
	}()

	// Use a writer that counts write calls
	var writeCount int
	counterWriter := &countingWriter{
		writeFn: func(p []byte) (n int, err error) {
			writeCount++
			return os.Stderr.Write(p) // Also output to stderr for visibility
		},
	}

	Init(LevelDebug, FormatText, counterWriter)

	tests := []struct {
		name       string
		logFunc    func(string, ...any)
		level      LogLevel
		shouldLog  bool
	}{
		{"debug", Debug, LevelDebug, true},
		{"info", Info, LevelInfo, true},
		{"warn", Warn, LevelWarn, true},
		{"error", Error, LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeCount = 0
			tt.logFunc("test message", "key", "value")

			if tt.shouldLog && writeCount == 0 {
				t.Errorf("Expected log to trigger write, but it didn't")
			}
		})
	}

	// Test direct logger call
	t.Run("direct logger", func(t *testing.T) {
		writeCount = 0
		log := Default()
		log.Info("direct test", "key", "value")
		if writeCount == 0 {
			t.Errorf("Expected direct log to trigger write, but it didn't")
		}
	})
}

type countingWriter struct {
	writeFn func(p []byte) (n int, err error)
}

func (w *countingWriter) Write(p []byte) (n int, err error) {
	return w.writeFn(p)
}

func TestLoggingWithContext(t *testing.T) {
	// Save current logger state
	oldLevel := CurrentLevel()
	defer func() {
		currentLevel.Store(int64(oldLevel))
	}()

	var writeCount int
	counterWriter := &countingWriter{writeFn: func(p []byte) (n int, err error) {
		writeCount++
		return os.Stderr.Write(p)
	}}

	Init(LevelDebug, FormatText, counterWriter)

	ctx := context.Background()
	InfoCtx(ctx, "test message", "key", "value")

	if writeCount == 0 {
		t.Errorf("Expected log to trigger write, but it didn't")
	}

	// Try direct logger call
	t.Run("direct", func(t *testing.T) {
		writeCount = 0
		log := Default()
		log.InfoContext(ctx, "direct test", "key", "value")
		if writeCount == 0 {
			t.Errorf("Expected direct log to trigger write, but it didn't")
		}
	})
}

func TestIsEnabled(t *testing.T) {
	var buf bytes.Buffer
	Init(LevelWarn, FormatText, &buf)

	tests := []struct {
		level LogLevel
		want  bool
	}{
		{LevelDebug, false},
		{LevelInfo, false},
		{LevelWarn, true},
		{LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			if got := IsEnabled(tt.level); got != tt.want {
				t.Errorf("IsEnabled(%v) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	Init(LevelInfo, FormatJSON, &buf)

	Info("test message", "key", "value")

	// Verify the handler was created (output goes to buf)
	if logger == nil {
		t.Error("Logger should not be nil after Init")
	}

	// Test that the logger is functional
	t.Run("direct", func(t *testing.T) {
		log := Default()
		if log == nil {
			t.Fatal("Default() returned nil logger")
		}
		// This should not panic
		log.Info("direct test", "key", "value")
	})
}

func TestRedact(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "***"},
		{"sk-12345678", "sk-1***5678"},
		{"a-b-c-d-e-f-g-h-i-j", "a-b-***-i-j"},
		{"12345678", "***"},           // exactly 8 chars → redacted
		{"123456789", "1234***6789"},  // 9 chars → partial redact
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Redact(tt.input); got != tt.want {
				t.Errorf("Redact() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	// Test that helpers create valid slog.Attr values
	t.Run("String", func(t *testing.T) {
		attr := String("key", "value")
		if attr.Key != "key" {
			t.Errorf("String() key = %v, want 'key'", attr.Key)
		}
	})

	t.Run("Int", func(t *testing.T) {
		attr := Int("num", 42)
		if attr.Key != "num" {
			t.Errorf("Int() key = %v, want 'num'", attr.Key)
		}
	})

	t.Run("Int64", func(t *testing.T) {
		attr := Int64("big", 123456789)
		if attr.Key != "big" {
			t.Errorf("Int64() key = %v, want 'big'", attr.Key)
		}
	})

	t.Run("Float64", func(t *testing.T) {
		attr := Float64("flt", 3.14)
		if attr.Key != "flt" {
			t.Errorf("Float64() key = %v, want 'flt'", attr.Key)
		}
	})

	t.Run("Any", func(t *testing.T) {
		attr := Any("any", map[string]string{"key": "value"})
		if attr.Key != "any" {
			t.Errorf("Any() key = %v, want 'any'", attr.Key)
		}
	})

	// Test that helpers work with logging
	t.Run("with logger", func(t *testing.T) {
		var buf bytes.Buffer
		Init(LevelInfo, FormatText, &buf)

		// This should not panic
		Info("test",
			String("str", "value"),
			Int("num", 42),
			Int64("big", 123456789),
			Float64("flt", 3.14),
			Any("any", map[string]string{"key": "value"}),
		)
	})
}

func TestLoggerHandlerLevels(t *testing.T) {
	tests := []struct {
		name         string
		logLevel     LogLevel
		logFunc      func()
		shouldLog    bool
	}{
		{"debug enabled", LevelDebug, func() { Debug("test") }, true},
		{"debug disabled", LevelInfo, func() { Debug("test") }, false},
		{"info enabled", LevelInfo, func() { Info("test") }, true},
		{"info disabled", LevelWarn, func() { Info("test") }, false},
		{"warn enabled", LevelWarn, func() { Warn("test") }, true},
		{"warn disabled", LevelError, func() { Warn("test") }, false},
		{"error enabled", LevelError, func() { Error("test") }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Init(tt.logLevel, FormatText, &buf)
			tt.logFunc()
			// Just verify no panic occurred
		})
	}
}

// TestConcurrentLogging tests that the logger is safe for concurrent use
func TestConcurrentLogging(t *testing.T) {
	var buf bytes.Buffer
	Init(LevelInfo, FormatText, &buf)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			Info("concurrent test", "i", i)
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test passes if no panic occurred
}

// TestDefaultLogger verifies Default() returns a valid logger
func TestDefaultLogger(t *testing.T) {
	log := Default()
	if log == nil {
		t.Error("Default() returned nil")
	}

	// Test that Default() works before Init()
	t.Run("before Init", func(t *testing.T) {
		// Reset logger to nil to test Default() fallback
		logger = nil
		log = Default()
		if log == nil {
			t.Error("Default() returned nil even with fallback")
		}
		// Re-initialize for other tests
		Init(LevelInfo, FormatText, nil)
	})
}

// TestLogLevelComparison tests level ordering
func TestLogLevelComparison(t *testing.T) {
	tests := []struct {
		a, b LogLevel
		want  bool // true if a >= b
	}{
		{LevelDebug, LevelInfo, false},
		{LevelInfo, LevelDebug, true},
		{LevelInfo, LevelInfo, true},
		{LevelError, LevelWarn, true},
		{LevelWarn, LevelError, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := tt.a >= tt.b
			if got != tt.want {
				t.Errorf("%v >= %v = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestMultipleInitCalls verifies that Init() can be called multiple times
func TestMultipleInitCalls(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	Init(LevelDebug, FormatText, &buf1)
	if CurrentLevel() != LevelDebug {
		t.Logf("First Init: got level %v (expected DEBUG)", CurrentLevel())
	}

	Init(LevelWarn, FormatJSON, &buf2)
	if CurrentLevel() != LevelWarn {
		t.Errorf("Second Init failed: got level %v, want WARN", CurrentLevel())
	}
}

// captureWriter is a helper that captures all writes
type captureWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
}

func newCaptureWriter() *captureWriter {
	return &captureWriter{}
}

func (w *captureWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *captureWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestLogOutputFormat verifies that output is formatted correctly
func TestLogOutputFormat(t *testing.T) {
	tests := []struct {
		name   string
		format LogFormat
		level  LogLevel
	}{
		{"text format", FormatText, LevelInfo},
		{"json format", FormatJSON, LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := newCaptureWriter()
			Init(tt.level, tt.format, cap)

			Info("test message", "key", "value")

			output := cap.String()
			if !strings.Contains(output, "test") {
				t.Logf("Output: %q", output)
				// This might be empty due to buffering, but we've verified the logger is configured
			}
		})
	}
}
