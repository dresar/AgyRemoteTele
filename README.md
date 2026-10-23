# AgyRemoteTele

Production-grade Telegram Remote Control Plane for Google Antigravity (AGY) cluster orchestration, multi-profile session history inspection, VPS telemetry monitoring, and automated post-execution reporting.

## Features

- **Multi-Profile AGY Orchestration**: Seamlessly execute prompts across `agy01` through `agy15` profiles with full environment isolation (`HOME` and `GEMINI_HOME`).
- **Interactive Multi-Turn Continuation**: Track active conversations per Telegram user, inspect transcript histories, resume previous sessions, or spawn fresh turns.
- **Post-Execution Action Reporter**: Parse execution results, extract tool calls (`write_file`, `edit_file`, `run_command`), inspect git diffs, and generate developer-grade HTML summary cards.
- **VPS Telemetry & Diagnostics**: Real-time CPU, RAM, Disk, Uptime, and Goroutine monitoring cards.
- **Workspace Navigation & File Transfer**: Browse VPS filesystem (`/ls`), view files (`/cat`), and download physical files directly to Telegram (`/get`).
- **Git Remote Management**: Check repository status, branch info, commit logs, and commit/push changes.
- **Developer-Grade Security**: Strict whitelist middleware, dynamic pairing token authentication (`/login`), and session expiration.
- **Robust Telegram UX**: Strict HTML formatting (avoiding Telegram Markdown entity crashes), automatic typing indicators, and message chunking for responses exceeding limits.

## Project Architecture

```
AgyRemoteTele/
├── cmd/
│   └── bot/
│       └── main.go           # Application entry point & graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go         # Environment loader & configuration models
│   ├── bot/
│   │   ├── bot.go            # Bot lifecycle, polling engine & shutdown hooks
│   │   ├── middleware.go     # Authentication whitelist & rate limiting
│   │   ├── renderer.go       # Telegram HTML cards & message chunker
│   │   └── router.go         # Command dispatcher & callback query handlers
│   ├── agy/
│   │   ├── executor.go       # Subprocess executor with timeout & environment isolation
│   │   ├── stream.go         # NDJSON stream parser for real-time progress
│   │   ├── profile.go        # Multi-profile manager (agy01 - agy15)
│   │   └── history.go        # Transcript reader & continuation manager
│   ├── session/
│   │   ├── store.go          # Thread-safe persistent session store
│   │   └── auth.go           # Dynamic pairing tokens & credential validation
│   ├── reporter/
│   │   └── action_reporter.go # Post-execution summary card generator
│   ├── system/
│   │   ├── telemetry.go      # CPU, RAM, Disk & Goroutine metric collector
│   │   └── runner.go         # Safe shell command runner with timeouts
│   └── workspace/
│       ├── filemgr.go        # Filesystem browser & document exporter
│       └── git.go            # Git status, log, commit & push operations
├── data/                     # Persistent session data
├── .env.example
├── .gitignore
└── go.mod
```

## Available Commands

| Command | Description |
|---|---|
| `/start` | Open interactive control center with inline actions |
| `/help` | Display command reference and usage guide |
| `/ask <prompt>` | Dispatch task to active Antigravity profile |
| `/profile [id]` | View or switch active AGY profile (`agy01` - `agy15`) |
| `/history` | View recent conversation transcripts with resume buttons |
| `/resume <id>` | Resume an existing conversation session |
| `/new` | Clear current session and start a new conversation |
| `/status` | View live VPS telemetry (CPU, RAM, Disk, Goroutines) |
| `/ls [dir]` | Explore workspace directory contents |
| `/cd <dir>` | Change active working directory |
| `/cat <file>` | Inspect file contents |
| `/get <file>` | Download file from VPS to Telegram as a document |
| `/sh <cmd>` | Execute safe shell command with execution timeout |
| `/git [cmd]` | Inspect git status or push pending changes |
| `/login <key>` | Authenticate admin device using pairing key |
| `/logout` | Invalidate active session |

## Setup & Deployment

1. Configure environment variables in `.env`:
   ```bash
   cp .env.example .env
   ```
2. Build the binary:
   ```bash
   /opt/data/tools/go/bin/go build -o bin/agyremote cmd/bot/main.go
   ```
3. Run the control plane:
   ```bash
   ./bin/agyremote
   ```
