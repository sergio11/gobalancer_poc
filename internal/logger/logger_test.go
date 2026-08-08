package logger

import (
	"testing"
)

func TestLogger_Init(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error", "unknown"}
	for _, level := range levels {
		log := Init(level)
		if log == nil {
			t.Errorf("expected logger to be initialized for level %q", level)
		}
	}
}

func TestLogger_Get(t *testing.T) {
	// Reset global logger
	Log = nil

	log := Get()
	if log == nil {
		t.Errorf("Get() should never return nil")
	}

	// Second call should return cached logger
	log2 := Get()
	if log2 == nil {
		t.Errorf("Get() second call should return a logger")
	}
}
