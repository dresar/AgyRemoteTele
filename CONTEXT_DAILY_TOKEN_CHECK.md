# Specification: Daily Token Healthcheck (1x Sehari)

## Background & Problem
Google OAuth refresh tokens in Antigravity last for weeks or months as long as they are active.
Checking token files every 60 seconds is completely unnecessary and leads to false alarms and CPU churn.
The requirement from Bos Eka is:
"auth agy itu bisa bertahan berhari hari Lo sebulan bisa kalau di pakek terus coba perbaikin lagi cek akun nya sehari sekali aja"
Check the accounts once a day (24 hours), NOT every 60 seconds!

## Requirements for AGY02

### 1. `internal/watchdog/watchdog.go`
- In `AlertConfig`:
  Add field `TokenCheckInterval time.Duration`.
- In `NewWatchdog`:
  If `cfg.TokenCheckInterval <= 0`, default to `24 * time.Hour`.
- In `Watchdog struct`:
  Add field `lastTokenCheck time.Time`.
- In `CheckNow()`:
  - RAM check (`checkMemory()`) and Disk check (`checkDisk()`) continue to run every tick (`CheckInterval`).
  - Token check (`checkTokens()`):
    Only execute if `w.lastTokenCheck.IsZero() || time.Since(w.lastTokenCheck) >= w.cfg.TokenCheckInterval`.
    Update `w.lastTokenCheck = time.Now()` when executed.
- Add `(w *Watchdog) ForceCheckTokens() []AlertEvent` so on-demand commands can run an immediate check when explicitly requested.

### 2. `cmd/bot/main.go`
- Set `TokenCheckInterval: 24 * time.Hour` in `wdCfg`.

### 3. `internal/watchdog/watchdog_test.go`
- Update unit tests to verify:
  - Rapid calls to `CheckNow()` only run `checkTokens()` once within `TokenCheckInterval`.
  - When `TokenCheckInterval` elapses, `checkTokens()` runs again.
  - All existing tests continue to pass.
- Run `export PATH="/opt/data/home/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64/bin:$PATH" && go test ./...` and ensure 100% PASS.
- Commit to branch `work/agy02-daily-token-check`.
