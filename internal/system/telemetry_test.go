package system

import (
	"testing"
	"time"
)

func TestCollectMetrics(t *testing.T) {
	metrics, err := CollectMetrics("/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.MemTotalMB == 0 {
		t.Error("expected non-zero memory total")
	}
	if metrics.DiskTotalGB == 0 {
		t.Error("expected non-zero disk total")
	}
	if metrics.Goroutines <= 0 {
		t.Error("expected positive goroutine count")
	}
}

func TestFormatUptime(t *testing.T) {
	d1 := 50 * time.Second
	if s := FormatUptime(d1); s != "0m 50s" {
		t.Errorf("expected 0m 50s, got %s", s)
	}

	d2 := 2*time.Hour + 15*time.Minute + 30*time.Second
	if s := FormatUptime(d2); s != "2h 15m 30s" {
		t.Errorf("expected 2h 15m 30s, got %s", s)
	}

	d3 := 49*time.Hour + 3*time.Minute
	if s := FormatUptime(d3); s != "2d 1h 3m" {
		t.Errorf("expected 2d 1h 3m, got %s", s)
	}
}
