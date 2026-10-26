# AgyRemoteTele Multi-Agent System Architecture & Contracts (CONTEXT.md)

This document establishes the architecture, package boundaries, data structures, and interfaces for expanding AgyRemoteTele with enterprise-grade system management, network diagnostics, Docker administration, cron scheduling, proactive watchdog alerting, automated backups, and cluster swarming.

---

## 1. Non-Overlapping Worker Boundary Matrix

| Worker | Role | Assigned Files & Directories | Core Responsibilities |
|---|---|---|---|
| **AGY03** | Integration & Systems | `internal/system/process.go`<br>`internal/system/ports.go`<br>`internal/system/services.go`<br>`internal/system/system_test.go`<br>`internal/nettools/`<br>`internal/docker/` | Native `/proc` process & port inspectors, service supervisor, network diagnostics (`ping`, `http`, `dns`, `netio`), Docker CLI manager. |
| **AGY02** | Core Subsystems | `internal/scheduler/`<br>`internal/watchdog/`<br>`internal/backup/`<br>`internal/cluster/` | Persistent cron job scheduler, proactive health watchdog with threshold alerts, tar.gz backup archiver, multi-worker swarm executor. |
| **AGY04** | Telegram UI & Routing | `internal/bot/renderer.go`<br>`internal/bot/renderer_test.go`<br>`internal/bot/router.go`<br>`internal/bot/router_test.go`<br>`cmd/bot/main.go` | Telegram HTML cards, interactive inline submenus, command routing (`/ps`, `/kill`, `/ports`, `/service`, `/ping`, `/curl`, `/dns`, `/netio`, `/docker`, `/cron`, `/alerts`, `/backup`, `/swarm`), graceful lifecycle wiring. |
| **AGY05** | QA & Integration | Integration tests, benchmarks, build verification | Verification of end-to-end integration and compilation under Go 1.24. |

---

## 2. Component Specifications

### A. `internal/system` (Owner: AGY03)
- **`process.go`**:
  - `type ProcessInfo struct`: `PID int`, `User string`, `CPU float64`, `Mem float64`, `RSSMB float64`, `State string`, `Command string`
  - `ListProcesses(topN int, sortBy string) ([]ProcessInfo, error)`: Parses `/proc/[pid]/stat`, `/proc/[pid]/cmdline`, `/proc/[pid]/status`.
  - `KillProcess(pid int, sig syscall.Signal) error`: Terminate process with safety barriers (rejects PID <= 1, self PID, parent PID, or kernel tasks).
- **`ports.go`**:
  - `type PortInfo struct`: `Protocol string`, `LocalIP string`, `LocalPort int`, `RemoteIP string`, `RemotePort int`, `State string`, `Inode uint64`, `ProcessName string`, `PID int`
  - `ListListeningPorts() ([]PortInfo, error)`: Parses `/proc/net/tcp`, `/proc/net/tcp6`, `/proc/net/udp`, `/proc/net/udp6`, hex IP/port converter, mapping inodes to `/proc/[pid]/fd`.
- **`services.go`**:
  - `type ServiceInfo struct`: `Name string`, `Status string`, `Active bool`, `SubState string`, `Description string`
  - `ListCommonServices(ctx context.Context) ([]ServiceInfo, error)`: Checks common services (`docker`, `cloudflared`, `nginx`, `cron`, `ssh`, `systemd-resolved`) via `systemctl is-active`.
  - `RestartService(ctx context.Context, serviceName string) (string, error)`: Validated against safe allowlist (`docker`, `nginx`, `cloudflared`, `cron`).

### B. `internal/nettools` (Owner: AGY03)
- **`nettools.go`**:
  - `type PingResult struct`: `Host string`, `PacketsSent int`, `PacketsRecv int`, `PacketLoss float64`, `MinRTT time.Duration`, `AvgRTT time.Duration`, `MaxRTT time.Duration`, `RawOutput string`
  - `Ping(ctx context.Context, host string, count int) (*PingResult, error)`: Executes `/bin/ping -c <count> -W 2 <host>` safely.
  - `type HTTPProbeResult struct`: `URL string`, `StatusCode int`, `StatusText string`, `Proto string`, `TTFB time.Duration`, `TotalDuration time.Duration`, `ContentLength int64`, `ContentType string`, `TLSIssuer string`, `TLSExpiry *time.Time`, `Headers map[string]string`
  - `ProbeHTTP(ctx context.Context, targetURL string, timeout time.Duration) (*HTTPProbeResult, error)`: HTTP GET/HEAD using standard `net/http` client with `httptrace.ClientTrace`.
  - `type DNSRecord struct`: `Type string`, `Values []string`
  - `LookupDNS(ctx context.Context, host string, recordType string) ([]DNSRecord, error)`: Uses `net.DefaultResolver` for A, AAAA, CNAME, MX, TXT records.
  - `type InterfaceStats struct`: `Name string`, `RXBytes uint64`, `TXBytes uint64`, `RXPackets uint64`, `TXPackets uint64`, `RXErrors uint64`, `TXErrors uint64`
  - `GetNetworkIO() ([]InterfaceStats, error)`: Parses `/proc/net/dev`.

### C. `internal/docker` (Owner: AGY03)
- **`manager.go`**:
  - `type ContainerInfo struct`: `ID string`, `Names string`, `Image string`, `Status string`, `State string`, `Ports string`, `Created string`
  - `type ContainerStats struct`: `ID string`, `Name string`, `CPUPerc string`, `MemUsage string`, `MemPerc string`, `NetIO string`, `BlockIO string`
  - `ListContainers(ctx context.Context, all bool) ([]ContainerInfo, error)`: Executes `docker ps [--all] --format json` or parsed tab format.
  - `GetContainerLogs(ctx context.Context, containerID string, tailLines int) (string, error)`: `docker logs --tail <tailLines> <containerID>`.
  - `RestartContainer(ctx context.Context, containerID string) error`: `docker restart <containerID>`.
  - `GetContainerStats(ctx context.Context) ([]ContainerStats, error)`: `docker stats --no-stream --format json`.

### D. `internal/scheduler` (Owner: AGY02)
- **`scheduler.go`**:
  - `type CronJob struct`: `ID string`, `Spec string`, `Command string`, `ChatID int64`, `Enabled bool`, `CreatedAt time.Time`, `LastRun *time.Time`, `NextRun time.Time`, `RunCount int`, `LastResult string`
  - `type Scheduler struct`: Thread-safe job manager with ticker/goroutine.
  - `NewScheduler(storePath string, dispatchFunc func(chatID int64, cmd string)) *Scheduler`
  - `(s *Scheduler) Start(ctx context.Context)`
  - `(s *Scheduler) Stop()`
  - `(s *Scheduler) AddJob(spec string, command string, chatID int64) (*CronJob, error)`: Supports `@every <duration>` (e.g. `@every 30m`, `@every 1h`) and standard 5-part cron specs (`0 * * * *`, `*/15 * * * *`).
  - `(s *Scheduler) RemoveJob(id string) bool`
  - `(s *Scheduler) ListJobs() []*CronJob`
  - `(s *Scheduler) RunJobNow(id string) error`
  - `(s *Scheduler) Save() error` / `(s *Scheduler) Load() error`: Persisted to `data/cron_jobs.json`.

### E. `internal/watchdog` (Owner: AGY02)
- **`watchdog.go`**:
  - `type AlertConfig struct`: `CheckInterval time.Duration`, `HighMemThreshold float64`, `CritMemThreshold float64`, `HighDiskThreshold float64`, `TokenWarningMinutes int`
  - `type AlertEvent struct`: `ID string`, `Timestamp time.Time`, `Level string`, `Metric string`, `Message string`, `Resolved bool`
  - `type Watchdog struct`: Background ticker checking memory (from `/proc/meminfo`), disk (from `syscall.Statfs`), and worker token expiry.
  - `NewWatchdog(cfg AlertConfig, notifyFunc func(level string, title string, message string)) *Watchdog`
  - `(w *Watchdog) Start(ctx context.Context)`
  - `(w *Watchdog) Stop()`
  - `(w *Watchdog) GetRecentAlerts() []AlertEvent`
  - `(w *Watchdog) CheckNow() []AlertEvent`

### F. `internal/backup` (Owner: AGY02)
- **`backup.go`**:
  - `type BackupResult struct`: `ArchivePath string`, `FileCount int`, `TotalSizeBytes int64`, `Duration time.Duration`
  - `CreateTarGz(sourceDirs []string, destFile string, excludePatterns []string) (*BackupResult, error)`: Pure Go `archive/tar` + `compress/gzip`.
  - `ListBackups(backupDir string) ([]os.FileInfo, error)`

### G. `internal/cluster` (Owner: AGY02)
- **`swarm.go`**:
  - `type SwarmTaskResult struct`: `Account string`, `TaskID string`, `Output string`, `Duration time.Duration`, `Error error`
  - `type SwarmManager struct`: Dispatches parallel prompts to multiple AGY profiles using `orchestrator.py dispatch`.
  - `NewSwarmManager(profilesRoot string, orchestratorPy string) *SwarmManager`
  - `(sm *SwarmManager) BroadcastPrompt(ctx context.Context, accounts []string, prompt string, workspace string) ([]SwarmTaskResult, error)`
  - `(sm *SwarmManager) QuickStatus(ctx context.Context) (map[string]string, error)`

### H. `internal/bot` & `cmd/bot` (Owner: AGY04)
- **`renderer.go`**:
  - `RenderProcessList(procs []system.ProcessInfo) string`
  - `RenderPortList(ports []system.PortInfo) string`
  - `RenderServiceList(services []system.ServiceInfo) string`
  - `RenderPingResult(p *nettools.PingResult) string`
  - `RenderHTTPProbe(res *nettools.HTTPProbeResult) string`
  - `RenderDNSResult(host string, records []nettools.DNSRecord) string`
  - `RenderNetworkIO(stats []nettools.InterfaceStats) string`
  - `RenderContainerList(containers []docker.ContainerInfo) string`
  - `RenderContainerStats(stats []docker.ContainerStats) string`
  - `RenderCronList(jobs []*scheduler.CronJob) string`
  - `RenderAlerts(alerts []watchdog.AlertEvent) string`
  - `RenderBackupResult(res *backup.BackupResult) string`
  - `RenderSwarmResult(results []cluster.SwarmTaskResult) string`
  - Multi-tier inline menu keyboards with category navigation.
- **`router.go`**:
  - Dispatch handlers for all new commands and inline callbacks.
  - Proactive notification broadcaster to active admin sessions.
- **`main.go`**:
  - Initialize `Scheduler`, `Watchdog`, `SwarmManager`.
  - Wire notifications to `bot.BroadcastAlert(level, title, message)`.
  - Graceful cancellation and cleanup on shutdown.
