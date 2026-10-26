# Watchdog Auto-Refresh & Canonical Profile Filtering Specification

## Problem Statement
1. **Duplicate & False Expiry Alerts**:
   `w.checkTokens()` reads all subdirectories in `profilesRoot` without filtering. It scans both `agy01..agy10` AND legacy alias folders `account1..account10`, generating duplicate alerts like `TOKEN_account5` and `TOKEN_agy05`.
2. **Passive Alerting vs Autonomous Refresh**:
   When an access token hits expiration (OAuth tokens expire every 1 hour), the watchdog fires CRITICAL alerts directly to Telegram even though the Google OAuth `refresh_token` is completely healthy and valid.
3. **Telegram Spam**:
   Alerts are sent individually for each account simultaneously instead of being refreshed silently or consolidated.

## Requirements for AGY02 (Core Subsystem Owner)

### 1. Canonical Profile Filtering (`internal/watchdog/watchdog.go` & `internal/cluster/swarm.go`)
- In `checkTokens()`, filter subdirectory names using regex `^agy(0[1-9]|1[0-5])$`.
- Strictly ignore directory entries like `account*`, `ORCHESTRATOR`, `.git`, `orchestrator_tasks`, or temporary files.
- In `internal/cluster/swarm.go`, ensure `GetClusterStatus()` also only checks canonical `agy01` through `agy10` (or up to `agy15`).

### 2. Autonomous Token Refresh in Watchdog
- Add `AutoRefreshFunc func(profileName string) error` field or method to `Watchdog`.
- Default auto-refresh implementation:
  Execute `python3 /opt/data/home/agy-profiles/fast_login.py refresh <profileName>` with a 30-second timeout.
- In `checkTokens()`:
  When `rem <= warnDuration` (or `rem <= 0`):
  First attempt auto-refresh using `AutoRefreshFunc`.
  After refresh, re-read the token file:
  - If re-read token has `exp.Sub(now) > warnDuration`:
    Log `Token for %s successfully auto-refreshed (valid for %d minutes)`.
    Do NOT fire an alert to Telegram! (Suppress false alarm).
  - If refresh fails or token is still expired:
    Fire `CRITICAL` alert: `Token refresh failed for %s: %v`.

### 3. Tests (`internal/watchdog/watchdog_test.go`)
- Update existing tests to conform to canonical naming (`agy01`, `agy02`, etc.).
- Add a test verifying that non-canonical directories (e.g. `account1`, `random_dir`) are completely ignored.
- Add a test verifying that when `AutoRefreshFunc` succeeds, the warning/critical alert is suppressed.
- Ensure all tests in `go test ./...` pass with 0 failures.
