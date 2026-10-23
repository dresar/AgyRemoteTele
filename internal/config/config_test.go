package config

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestParseChatIDs(t *testing.T) {
	tests := []struct {
		input    string
		expected []int64
	}{
		{"", nil},
		{"12345", []int64{12345}},
		{"123, 456 ,789", []int64{123, 456, 789}},
		{"invalid, 100", []int64{100}},
	}

	for _, tt := range tests {
		actual := parseChatIDs(tt.input)
		if !reflect.DeepEqual(actual, tt.expected) {
			t.Errorf("parseChatIDs(%q) = %v, expected %v", tt.input, actual, tt.expected)
		}
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	origToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	origProfile := os.Getenv("DEFAULT_PROFILE")
	defer func() {
		_ = os.Setenv("TELEGRAM_BOT_TOKEN", origToken)
		_ = os.Setenv("DEFAULT_PROFILE", origProfile)
	}()

	_ = os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
	_ = os.Unsetenv("DEFAULT_PROFILE")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.TelegramBotToken != "test-token" {
		t.Errorf("expected token test-token, got %s", cfg.TelegramBotToken)
	}

	if cfg.DefaultProfile != "agy01" {
		t.Errorf("expected default profile agy01, got %s", cfg.DefaultProfile)
	}

	if cfg.ExecutionTimeout != 30*time.Minute {
		t.Errorf("expected 30m timeout, got %v", cfg.ExecutionTimeout)
	}
}
