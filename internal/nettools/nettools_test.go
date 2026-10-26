package nettools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := Ping(ctx, "127.0.0.1", 1)
	if err != nil {
		t.Fatalf("Ping 127.0.0.1 failed: %v", err)
	}

	if res.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", res.Host)
	}
	if res.PacketsSent < 1 {
		t.Errorf("expected PacketsSent >= 1, got %d", res.PacketsSent)
	}
	if res.PacketsRecv < 1 {
		t.Errorf("expected PacketsRecv >= 1, got %d", res.PacketsRecv)
	}
	if res.PacketLoss != 0 {
		t.Errorf("expected 0%% loss on loopback, got %f", res.PacketLoss)
	}

	_, errInvalid := Ping(ctx, "-c 5 127.0.0.1", 1)
	if errInvalid == nil {
		t.Error("expected error for invalid host injection")
	}

	_, errEmpty := Ping(ctx, "", 1)
	if errEmpty == nil {
		t.Error("expected error for empty host")
	}
}

func TestParsePingOutput(t *testing.T) {
	mockSuccess := `PING 1.1.1.1 (1.1.1.1) 56(84) bytes of data.
64 bytes from 1.1.1.1: icmp_seq=1 ttl=58 time=12.4 ms
64 bytes from 1.1.1.1: icmp_seq=2 ttl=58 time=16.0 ms

--- 1.1.1.1 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1002ms
rtt min/avg/max/mdev = 12.410/14.230/16.050/1.820 ms`

	res, err := parsePingOutput("1.1.1.1", mockSuccess)
	if err != nil {
		t.Fatalf("parsePingOutput failed: %v", err)
	}
	if res.PacketsSent != 2 || res.PacketsRecv != 2 {
		t.Errorf("expected 2 sent/recv, got %d/%d", res.PacketsSent, res.PacketsRecv)
	}
	if res.PacketLoss != 0 {
		t.Errorf("expected 0 loss, got %f", res.PacketLoss)
	}
	if res.MinRTT <= 0 || res.AvgRTT <= 0 || res.MaxRTT <= 0 {
		t.Error("expected positive RTT durations")
	}

	mockLoss := `--- 192.0.2.1 ping statistics ---
3 packets transmitted, 0 received, 100% packet loss, time 2048ms`

	resLoss, errLoss := parsePingOutput("192.0.2.1", mockLoss)
	if errLoss != nil {
		t.Fatalf("parsePingOutput loss failed: %v", errLoss)
	}
	if resLoss.PacketsSent != 3 || resLoss.PacketsRecv != 0 {
		t.Errorf("expected 3 sent 0 recv, got %d/%d", resLoss.PacketsSent, resLoss.PacketsRecv)
	}
	if resLoss.PacketLoss != 100 {
		t.Errorf("expected 100 loss, got %f", resLoss.PacketLoss)
	}
}

func TestProbeHTTP(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Test", "Passed")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello from test server")
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := ProbeHTTP(ctx, ts.URL, 3*time.Second)
	if err != nil {
		t.Fatalf("ProbeHTTP failed: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}
	if res.StatusText != "200 OK" {
		t.Errorf("expected 200 OK, got %s", res.StatusText)
	}
	if res.Headers["X-Custom-Test"] != "Passed" {
		t.Errorf("expected header X-Custom-Test to be Passed, got %s", res.Headers["X-Custom-Test"])
	}
	if res.TTFB <= 0 {
		t.Errorf("expected positive TTFB, got %v", res.TTFB)
	}
	if res.TotalDuration <= 0 {
		t.Errorf("expected positive TotalDuration, got %v", res.TotalDuration)
	}
	if res.TLSExpiry == nil {
		t.Error("expected non-nil TLSExpiry for TLS server")
	}

	_, errEmpty := ProbeHTTP(ctx, "", time.Second)
	if errEmpty == nil {
		t.Error("expected error for empty target URL")
	}
}

func TestLookupDNS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	records, err := LookupDNS(ctx, "localhost", "A")
	if err == nil {
		if len(records) > 0 && records[0].Type != "A" {
			t.Errorf("expected type A, got %s", records[0].Type)
		}
	}

	_, errInvalid := LookupDNS(ctx, "localhost", "UNSUPPORTED_XYZ")
	if errInvalid == nil {
		t.Error("expected error for unsupported record type")
	}

	_, errEmpty := LookupDNS(ctx, "", "A")
	if errEmpty == nil {
		t.Error("expected error for empty host")
	}
}

func TestGetNetworkIO(t *testing.T) {
	stats, err := GetNetworkIO()
	if err != nil {
		t.Fatalf("GetNetworkIO failed: %v", err)
	}

	if len(stats) == 0 {
		t.Fatal("expected at least one network interface")
	}

	foundLo := false
	for _, s := range stats {
		if s.Name == "lo" {
			foundLo = true
			break
		}
	}
	if !foundLo {
		t.Log("lo interface not found in /proc/net/dev, continuing")
	}
}

func TestParseNetworkIOBytes(t *testing.T) {
	mockDev := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000 10    1    0    0     0          0         0 2000 20    2    0    0     0       0          0
  eth0: 5000 50    0    0    0     0          0         0 6000 60    0    0    0     0       0          0
`
	stats, err := parseNetworkIOBytes([]byte(mockDev))
	if err != nil {
		t.Fatalf("parseNetworkIOBytes failed: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(stats))
	}

	lo := stats[0]
	if lo.Name != "lo" {
		t.Errorf("expected lo, got %s", lo.Name)
	}
	if lo.RXBytes != 1000 || lo.RXPackets != 10 || lo.RXErrors != 1 {
		t.Errorf("unexpected lo RX: bytes=%d, pkts=%d, errs=%d", lo.RXBytes, lo.RXPackets, lo.RXErrors)
	}
	if lo.TXBytes != 2000 || lo.TXPackets != 20 || lo.TXErrors != 2 {
		t.Errorf("unexpected lo TX: bytes=%d, pkts=%d, errs=%d", lo.TXBytes, lo.TXPackets, lo.TXErrors)
	}

	eth0 := stats[1]
	if eth0.Name != "eth0" {
		t.Errorf("expected eth0, got %s", eth0.Name)
	}
	if eth0.RXBytes != 5000 || eth0.TXBytes != 6000 {
		t.Errorf("unexpected eth0 bytes: RX=%d, TX=%d", eth0.RXBytes, eth0.TXBytes)
	}
}
