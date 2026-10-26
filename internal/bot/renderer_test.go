package bot

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dresar/AgyRemoteTele/internal/backup"
	"github.com/dresar/AgyRemoteTele/internal/cluster"
	"github.com/dresar/AgyRemoteTele/internal/docker"
	"github.com/dresar/AgyRemoteTele/internal/nettools"
	"github.com/dresar/AgyRemoteTele/internal/scheduler"
	"github.com/dresar/AgyRemoteTele/internal/session"
	"github.com/dresar/AgyRemoteTele/internal/system"
	"github.com/dresar/AgyRemoteTele/internal/watchdog"
)

func TestRendererChunking(t *testing.T) {
	r := NewRenderer()

	shortMsg := "This is a short message."
	chunks := r.ChunkMessage(shortMsg, 100)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "Line of sample message output that has some length."
	}
	longMsg := strings.Join(lines, "\n")

	chunks = r.ChunkMessage(longMsg, 200)
	if len(chunks) <= 1 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}
	if !strings.HasPrefix(chunks[0], "<b>[1/") {
		t.Errorf("expected pagination indicator in first chunk, got %s", chunks[0])
	}
}

func TestRendererHelp(t *testing.T) {
	r := NewRenderer()
	help := r.RenderHelp()
	expected := []string{"/ask", "/profile", "/ps", "/kill", "/ports", "/service", "/ping", "/curl", "/dns", "/netio", "/docker", "/cron", "/alerts", "/backup", "/swarm"}
	for _, exp := range expected {
		if !strings.Contains(help, exp) {
			t.Errorf("expected help card to include %s", exp)
		}
	}
}

func TestRendererProcesses(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderProcesses(nil)
	if !strings.Contains(empty, "No processes found") {
		t.Errorf("expected empty message, got: %s", empty)
	}

	procs := []system.ProcessInfo{
		{
			PID:     1234,
			User:    "root",
			CPU:     12.5,
			Mem:     3.2,
			RSSMB:   128.0,
			State:   "S",
			Command: "go run main.go",
		},
	}
	out := r.RenderProcesses(procs)
	if !strings.Contains(out, "PID 1234") || !strings.Contains(out, "12.5% CPU") || !strings.Contains(out, "go run main.go") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererListeningPorts(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderListeningPorts(nil)
	if !strings.Contains(empty, "No active listening ports") {
		t.Errorf("expected empty ports message, got: %s", empty)
	}

	ports := []system.PortInfo{
		{
			Protocol:    "tcp",
			LocalIP:     "0.0.0.0",
			LocalPort:   8080,
			State:       "LISTEN",
			ProcessName: "bot",
			PID:         5678,
		},
	}
	out := r.RenderListeningPorts(ports)
	if !strings.Contains(out, "TCP") || !strings.Contains(out, "8080") || !strings.Contains(out, "bot") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererServices(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderServices(nil)
	if !strings.Contains(empty, "No common services") {
		t.Errorf("expected empty services message, got: %s", empty)
	}

	services := []system.ServiceInfo{
		{
			Name:        "docker",
			Status:      "running",
			Active:      true,
			SubState:    "active",
			Description: "Docker Application Container Engine",
		},
		{
			Name:        "nginx",
			Status:      "stopped",
			Active:      false,
			SubState:    "dead",
			Description: "A high performance web server",
		},
	}
	out := r.RenderServices(services)
	if !strings.Contains(out, "docker") || !strings.Contains(out, "nginx") || !strings.Contains(out, "🟢") || !strings.Contains(out, "🔴") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererPing(t *testing.T) {
	r := NewRenderer()

	if nilRes := r.RenderPing(nil); !strings.Contains(nilRes, "No ping result") {
		t.Errorf("expected nil result message, got: %s", nilRes)
	}

	res := &nettools.PingResult{
		Host:        "1.1.1.1",
		PacketsSent: 3,
		PacketsRecv: 3,
		PacketLoss:  0.0,
		MinRTT:      10 * time.Millisecond,
		AvgRTT:      12 * time.Millisecond,
		MaxRTT:      15 * time.Millisecond,
	}
	out := r.RenderPing(res)
	if !strings.Contains(out, "1.1.1.1") || !strings.Contains(out, "0.0%") || !strings.Contains(out, "Min:") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererHTTPProbe(t *testing.T) {
	r := NewRenderer()

	if nilRes := r.RenderHTTPProbe(nil); !strings.Contains(nilRes, "No probe result") {
		t.Errorf("expected nil result message, got: %s", nilRes)
	}

	expiry := time.Now().Add(24 * time.Hour)
	res := &nettools.HTTPProbeResult{
		URL:           "https://example.com",
		StatusCode:    200,
		StatusText:    "OK",
		Proto:         "HTTP/2.0",
		TTFB:          50 * time.Millisecond,
		TotalDuration: 120 * time.Millisecond,
		ContentLength: 1024,
		ContentType:   "text/html",
		TLSIssuer:     "Let's Encrypt",
		TLSExpiry:     &expiry,
	}
	out := r.RenderHTTPProbe(res)
	if !strings.Contains(out, "200 OK") || !strings.Contains(out, "Encrypt") || !strings.Contains(out, "TTFB:") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererDNS(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderDNS("empty.test", nil)
	if !strings.Contains(empty, "No DNS records found") {
		t.Errorf("expected empty dns message, got: %s", empty)
	}

	recs := []nettools.DNSRecord{
		{Type: "A", Values: []string{"1.2.3.4", "5.6.7.8"}},
	}
	out := r.RenderDNS("example.com", recs)
	if !strings.Contains(out, "example.com") || !strings.Contains(out, "1.2.3.4") || !strings.Contains(out, "[A Records]") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererNetworkIO(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderNetworkIO(nil)
	if !strings.Contains(empty, "No network interfaces") {
		t.Errorf("expected empty netio message, got: %s", empty)
	}

	stats := []nettools.InterfaceStats{
		{
			Name:      "eth0",
			RXBytes:   1048576,
			TXBytes:   2097152,
			RXPackets: 100,
			TXPackets: 200,
			RXErrors:  0,
			TXErrors:  0,
		},
	}
	out := r.RenderNetworkIO(stats)
	if !strings.Contains(out, "eth0") || !strings.Contains(out, "RX:") || !strings.Contains(out, "TX:") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererContainers(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderContainers(nil)
	if !strings.Contains(empty, "No containers found") {
		t.Errorf("expected empty containers message, got: %s", empty)
	}

	containers := []docker.ContainerInfo{
		{
			ID:      "c1234567890abcdef",
			Names:   "web-server",
			Image:   "nginx:latest",
			Status:  "Up 2 hours",
			State:   "running",
			Ports:   "0.0.0.0:80->80/tcp",
			Created: "2 hours ago",
		},
	}
	out := r.RenderContainers(containers)
	if !strings.Contains(out, "web-server") || !strings.Contains(out, "nginx:latest") || !strings.Contains(out, "c1234567890a") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererContainerStats(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderContainerStats(nil)
	if !strings.Contains(empty, "No container resource statistics") {
		t.Errorf("expected empty stats message, got: %s", empty)
	}

	stats := []docker.ContainerStats{
		{
			ID:       "c1234567890abcdef",
			Name:     "web-server",
			CPUPerc:  "0.50%",
			MemUsage: "25MiB / 512MiB",
			MemPerc:  "4.88%",
			NetIO:    "1.2MB / 3.4MB",
			BlockIO:  "0B / 0B",
		},
	}
	out := r.RenderContainerStats(stats)
	if !strings.Contains(out, "web-server") || !strings.Contains(out, "0.50%") || !strings.Contains(out, "25MiB") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererCronJobs(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderCronJobs(nil)
	if !strings.Contains(empty, "No scheduled cron jobs") {
		t.Errorf("expected empty cron message, got: %s", empty)
	}

	now := time.Now()
	jobs := []*scheduler.CronJob{
		{
			ID:         "job_01",
			Spec:       "@every 1h",
			Command:    "/status",
			Enabled:    true,
			NextRun:    now.Add(1 * time.Hour),
			RunCount:   5,
			LastResult: "Success",
		},
	}
	out := r.RenderCronJobs(jobs)
	if !strings.Contains(out, "job_01") || !strings.Contains(out, "/status") || !strings.Contains(out, "@every 1h") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererAlerts(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderAlerts(nil)
	if !strings.Contains(empty, "All systems nominal") {
		t.Errorf("expected nominal message, got: %s", empty)
	}

	alerts := []watchdog.AlertEvent{
		{
			ID:        "alt_1",
			Timestamp: time.Now(),
			Level:     "WARNING",
			Metric:    "RAM",
			Message:   "RAM usage exceeded 85%",
			Resolved:  false,
		},
		{
			ID:        "alt_2",
			Timestamp: time.Now(),
			Level:     "CRITICAL",
			Metric:    "DISK",
			Message:   "Disk free space is below 5GB",
			Resolved:  false,
		},
	}
	out := r.RenderAlerts(alerts)
	if !strings.Contains(out, "RAM usage exceeded 85%") || !strings.Contains(out, "CRITICAL") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererBackup(t *testing.T) {
	r := NewRenderer()

	if nilRes := r.RenderBackup(nil); !strings.Contains(nilRes, "No backup result") {
		t.Errorf("expected nil backup message, got: %s", nilRes)
	}

	res := &backup.BackupResult{
		ArchivePath:    "/data/backups/backup.tar.gz",
		FileCount:      42,
		TotalSizeBytes: 10485760,
		Duration:       500 * time.Millisecond,
	}
	out := r.RenderBackup(res)
	if !strings.Contains(out, "/data/backups/backup.tar.gz") || !strings.Contains(out, "42 files") || !strings.Contains(out, "10.0 MB") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestRendererSwarm(t *testing.T) {
	r := NewRenderer()

	empty := r.RenderSwarm(nil)
	if !strings.Contains(empty, "No swarm task results") {
		t.Errorf("expected empty swarm message, got: %s", empty)
	}

	results := []cluster.SwarmTaskResult{
		{
			Account:  "agy02",
			TaskID:   "task_001",
			Output:   "Git status is clean",
			Duration: 250 * time.Millisecond,
			Error:    nil,
		},
		{
			Account:  "agy03",
			TaskID:   "",
			Output:   "",
			Duration: 50 * time.Millisecond,
			Error:    errors.New("connection refused"),
		},
	}
	out := r.RenderSwarm(results)
	if !strings.Contains(out, "agy02") || !strings.Contains(out, "task_001") || !strings.Contains(out, "connection refused") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestKeyboards(t *testing.T) {
	r := NewRenderer()

	startKb := r.GetStartKeyboard()
	if len(startKb.InlineKeyboard) < 5 {
		t.Errorf("expected at least 5 rows in start keyboard, got %d", len(startKb.InlineKeyboard))
	}

	backKb := r.GetBackKeyboard()
	if len(backKb.InlineKeyboard) != 1 {
		t.Errorf("expected 1 row in back keyboard, got %d", len(backKb.InlineKeyboard))
	}

	cats := []string{"system", "docker", "net", "scheduler", "swarm", "backup", "unknown"}
	for _, c := range cats {
		kb := r.GetCategoryKeyboard(c)
		if len(kb.InlineKeyboard) == 0 {
			t.Errorf("expected non-empty category keyboard for %s", c)
		}
	}
}

func TestRenderWelcome(t *testing.T) {
	r := NewRenderer()

	tests := []struct {
		name           string
		username       string
		defaultProfile string
		sess           *session.Session
		wantStatus     string
		wantWorker     string
	}{
		{
			name:           "NilSession",
			username:       "bot",
			defaultProfile: "agy01",
			sess:           nil,
			wantStatus:     "<b>Telegram Session:</b> 🟡 Unpaired (Guest)",
			wantWorker:     "<b>Active Worker:</b> <code>agy01</code>",
		},
		{
			name:           "GuestUnpaired",
			username:       "bot",
			defaultProfile: "agy01",
			sess: &session.Session{
				IsAuthenticated: false,
				ActiveProfile:   "agy02",
			},
			wantStatus: "<b>Telegram Session:</b> 🟡 Unpaired (Guest)",
			wantWorker: "<b>Active Worker:</b> <code>agy02</code>",
		},
		{
			name:           "AuthorizedAdmin",
			username:       "bot",
			defaultProfile: "agy01",
			sess: &session.Session{
				IsAuthenticated: true,
				ActiveProfile:   "agy01",
			},
			wantStatus: "<b>Telegram Session:</b> 🟢 Authorized Admin",
			wantWorker: "<b>Active Worker:</b> <code>agy01</code>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := r.RenderWelcome(tc.username, tc.defaultProfile, tc.sess)
			if !strings.Contains(out, tc.wantStatus) {
				t.Errorf("RenderWelcome() status mismatch, want %q in output:\n%s", tc.wantStatus, out)
			}
			if !strings.Contains(out, tc.wantWorker) {
				t.Errorf("RenderWelcome() worker mismatch, want %q in output:\n%s", tc.wantWorker, out)
			}
			if strings.Contains(out, "Status: 🟡") || strings.Contains(out, "Status: 🟢") {
				t.Errorf("RenderWelcome() should not contain legacy Status label:\n%s", out)
			}
		})
	}
}
