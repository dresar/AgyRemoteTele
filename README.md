# AgyRemoteTele

Production-grade Telegram Remote Control Plane for Google Antigravity (AGY) cluster orchestration, multi-profile session history inspection, VPS telemetry monitoring, Docker management, network diagnostics, cron scheduling, and automated health watchdog alerting.

## Core Architecture & Subsystems

```
AgyRemoteTele/
├── cmd/
│   └── bot/
│       └── main.go             # Entrypoint, lifecycle supervisor & graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go           # Environment loader & configuration models
│   ├── bot/
│   │   ├── bot.go              # Bot lifecycle, polling engine & command registrar
│   │   ├── middleware.go       # Authentication whitelist & rate limiting
│   │   ├── renderer.go         # HTML cards, message chunker & category submenus
│   │   └── router.go           # Command dispatcher & callback query handlers
│   ├── agy/
│   │   ├── executor.go         # Subprocess executor with isolation (HOME/GEMINI_HOME)
│   │   ├── stream.go           # NDJSON stream parser for real-time progress
│   │   ├── profile.go          # Multi-profile manager (agy01 - agy15)
│   │   └── history.go          # Transcript reader & session continuation
│   ├── system/
│   │   ├── process.go          # Native /proc top process monitor & safe kill
│   │   ├── ports.go            # Native /proc/net listening socket & port inspector
│   │   ├── services.go         # Systemctl service supervisor (status/restart)
│   │   ├── telemetry.go        # CPU, RAM, Disk & Goroutine metric collector
│   │   └── runner.go           # Safe shell command execution
│   ├── nettools/
│   │   └── nettools.go         # Ping latency, HTTP probe (TTFB/TLS), DNS, /proc/net/dev I/O
│   ├── docker/
│   │   └── manager.go          # Container listing, tail logs, restart & live stats
│   ├── scheduler/
│   │   └── scheduler.go        # In-memory persistent cron & interval job scheduler
│   ├── watchdog/
│   │   └── watchdog.go         # Background health monitor, threshold alerts & token expiry
│   ├── backup/
│   │   └── backup.go           # Pure Go .tar.gz workspace archiver & exporter
│   ├── cluster/
│   │   └── swarm.go            # Multi-worker parallel broadcast dispatcher & cluster inspector
│   ├── session/
│   │   ├── store.go            # Thread-safe persistent session store
│   │   └── auth.go             # Pairing tokens & credential validation
│   ├── reporter/
│   │   └── action_reporter.go  # Post-execution summary card generator
│   └── workspace/
│       ├── filemgr.go          # Filesystem browser & document exporter
│       └── git.go              # Git status, log, commit & push operations
├── data/                       # Persistent session & cron data
├── .env.example
├── .gitignore
└── go.mod
```

## Available Commands

### ⚡ System & VPS Diagnostics
| Command | Description |
|---|---|
| `/status` | View real-time VPS telemetry (CPU, RAM, Disk, Goroutines) |
| `/ps [topN] [cpu\|mem]` | Inspect top active processes with PID, user, CPU%, and Mem% |
| `/kill <pid> [sig]` | Terminate a process safely (with PID 1 and self-protection guards) |
| `/ports` or `/netstat` | List listening TCP & UDP sockets decoded from `/proc/net` |
| `/service [list\|status\|restart] [name]` | Inspect status or restart safe system services |

### 🌐 Network & Web Diagnostics
| Command | Description |
|---|---|
| `/ping <host>` | Measure ICMP latency, packet loss, and RTT stats |
| `/curl <url>` or `/http <url>` | Probe HTTP/HTTPS endpoint (TTFB, total latency, TLS expiry, headers) |
| `/dns <domain> [type]` | Query DNS records (A, AAAA, CNAME, MX, TXT) via net.Resolver |
| `/netio` | Real-time network interface traffic and I/O rates from `/proc/net/dev` |

### 🐳 Docker & Containers
| Command | Description |
|---|---|
| `/docker ps [all]` | List running and stopped Docker containers |
| `/docker stats` | Live container CPU% and memory utilization |
| `/docker logs <id\|name> [lines]` | Inspect tail logs of a specific container |
| `/docker restart <id\|name>` | Restart a Docker container with validation |

### ⏱ Scheduler & Health Alerts
| Command | Description |
|---|---|
| `/cron list` | List all active scheduled recurring jobs |
| `/cron add <spec> <cmd>` | Add recurring job (e.g. `@every 30m /refresh`, `0 */6 * * * /status`) |
| `/cron run <id>` | Manually trigger a scheduled job immediately |
| `/cron rm <id>` | Remove a scheduled job |
| `/alerts` | View recent health watchdog threshold events (RAM, Disk, Worker Tokens) |

### 🤖 AGY Cluster, Workers & Swarm
| Command | Description |
|---|---|
| `/ask <prompt>` | Dispatch prompt to active Antigravity profile |
| `/swarm <prompt>` | Broadcast prompt concurrently across all available worker accounts |
| `/workers` | View daemon remote-control status of `agy01` - `agy10` |
| `/accounts` | Inspect Google OAuth token status and remaining session lifetime |
| `/refresh [acc]` | Auto-refresh OAuth credentials for workers |
| `/usage` | View cluster usage monitor (finished tasks, tokens, daemons) |
| `/tasks` | View active and historical delegated worker tasks |
| `/profile [id]` | View or switch active AGY profile (`agy01` - `agy15`) |
| `/history` | View recent conversation transcripts with resume buttons |
| `/resume <id>` | Resume an existing conversation session |
| `/new` | Clear current session and start a new conversation |

### 📁 Workspace & Archival
| Command | Description |
|---|---|
| `/ls [dir]` | Explore workspace directory contents |
| `/cd <dir>` | Change active working directory |
| `/cat <file>` | Inspect file contents |
| `/get <file>` | Download file from VPS to Telegram as a document |
| `/backup [path]` | Create compressed `.tar.gz` archive (auto-sends to Telegram if < 50MB) |
| `/sh <cmd>` | Execute safe shell command with execution timeout |
| `/git [cmd]` | Inspect git status or push pending changes |
| `/login <key>` | Authenticate admin device using pairing key |
| `/logout` | Invalidate active session |

## Setup & Deployment

1. Configure environment variables in `.env`:
   ```bash
   cp .env.example .env
   ```
2. Build the production binary:
   ```bash
   /opt/data/tools/go/bin/go build -ldflags="-s -w" -o bin/agyremote cmd/bot/main.go
   ```
3. Run the control plane via daemon:
   ```bash
   ./daemon.sh
   ```
