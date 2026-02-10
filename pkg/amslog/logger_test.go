package amslog

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	config := Config{
		ServiceName:    "kb-agent",
		ServiceVersion: "1.0.0",
		Environment:    "dev",
	}

	logger := NewLogger(config)
	if logger == nil {
		t.Fatal("Expected logger to be created")
	}
	defer logger.Close()
}

func TestLoggerInfo(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		ServiceName:    "kb-agent",
		ServiceVersion: "1.0.0",
		Environment:    "dev",
		Output:         &buf,
	}

	logger := NewLogger(config)
	defer logger.Close()

	logger.Info(Event{
		Name:    "TEST_EVENT",
		Message: "Test message",
	})

	output := buf.String()
	if !strings.Contains(output, "TEST_EVENT") {
		t.Errorf("Expected log to contain TEST_EVENT, got: %s", output)
	}

	// Verificar que es JSON válido
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Errorf("Expected valid JSON, got error: %v", err)
	}

	// Verificar campos obligatorios
	requiredFields := []string{
		"@timestamp",
		"log.level",
		"service.name",
		"service.version",
		"labels.environment",
		"event.name",
		"event.outcome",
		"message",
	}

	for _, field := range requiredFields {
		if _, ok := logEntry[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

func TestLoggerWithContext(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		ServiceName:    "kb-agent",
		ServiceVersion: "1.0.0",
		Environment:    "dev",
		Output:         &buf,
	}

	logger := NewLogger(config)
	defer logger.Close()

	ctx := WithTraceID(context.Background(), "trace-123")
	ctx = WithRequestID(ctx, "req-456")

	logger.InfoContext(ctx, Event{
		Name:    "TEST_EVENT",
		Message: "Test with context",
	})

	output := buf.String()
	var logEntry map[string]interface{}
	json.Unmarshal([]byte(output), &logEntry)

	if logEntry["trace.id"] != "trace-123" {
		t.Errorf("Expected trace.id to be trace-123, got: %v", logEntry["trace.id"])
	}

	if logEntry["request.id"] != "req-456" {
		t.Errorf("Expected request.id to be req-456, got: %v", logEntry["request.id"])
	}
}

func TestSanitization(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		ServiceName:        "kb-agent",
		ServiceVersion:     "1.0.0",
		Environment:        "dev",
		Output:             &buf,
		EnableSanitization: true,
	}

	logger := NewLogger(config)
	defer logger.Close()

	logger.Info(Event{
		Name:    "TEST_SANITIZATION",
		Message: "Test sanitization",
		Fields: map[string]interface{}{
			"username": "john",
			"password": "secret123",
			"token":    "abc123",
		},
	})

	output := buf.String()
	if strings.Contains(output, "secret123") {
		t.Error("Password should be sanitized")
	}
	if strings.Contains(output, "abc123") {
		t.Error("Token should be sanitized")
	}
	if !strings.Contains(output, "***REDACTED***") {
		t.Error("Expected ***REDACTED*** in output")
	}
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		ServiceName:    "kb-agent",
		ServiceVersion: "1.0.0",
		Environment:    "dev",
		Output:         &buf,
		MinLevel:       LevelInfo,
	}

	logger := NewLogger(config)
	defer logger.Close()

	// DEBUG no debería aparecer
	logger.Debug(Event{
		Name:    "DEBUG_EVENT",
		Message: "Debug message",
	})

	if strings.Contains(buf.String(), "DEBUG_EVENT") {
		t.Error("DEBUG log should be filtered out")
	}

	// INFO sí debería aparecer
	logger.Info(Event{
		Name:    "INFO_EVENT",
		Message: "Info message",
	})

	if !strings.Contains(buf.String(), "INFO_EVENT") {
		t.Error("INFO log should appear")
	}
}

func TestAsyncLogger(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		ServiceName:    "kb-agent",
		ServiceVersion: "1.0.0",
		Environment:    "dev",
		Output:         &buf,
		Async:          true,
		BufferSize:     100,
	}

	logger := NewLogger(config)

	for i := 0; i < 10; i++ {
		logger.Info(Event{
			Name:    "ASYNC_TEST",
			Message: "Async log",
		})
	}

	logger.Close() // Flush logs

	output := buf.String()
	count := strings.Count(output, "ASYNC_TEST")
	if count != 10 {
		t.Errorf("Expected 10 logs, got %d", count)
	}
}

func BenchmarkSyncLogger(b *testing.B) {
	var buf bytes.Buffer
	config := Config{
		ServiceName:    "kb-agent",
		ServiceVersion: "1.0.0",
		Environment:    "dev",
		Output:         &buf,
	}

	logger := NewLogger(config)
	defer logger.Close()

	event := Event{
		Name:    "BENCHMARK_EVENT",
		Message: "Benchmark message",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info(event)
	}
}

func BenchmarkAsyncLogger(b *testing.B) {
	var buf bytes.Buffer
	config := Config{
		ServiceName:    "kb-agent",
		ServiceVersion: "1.0.0",
		Environment:    "dev",
		Output:         &buf,
		Async:          true,
		BufferSize:     10000,
	}

	logger := NewLogger(config)
	defer logger.Close()

	event := Event{
		Name:    "BENCHMARK_EVENT",
		Message: "Benchmark message",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info(event)
	}
}