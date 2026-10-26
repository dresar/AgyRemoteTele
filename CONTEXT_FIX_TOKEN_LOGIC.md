# Specification: Fix Token Expiry Evaluation in Watchdog

## Problem Analysis
1. `TokenWarningMinutes: 30` in `cmd/bot/main.go`:
   Google OAuth tokens have a total lifespan of only 60 minutes. A 30-minute threshold puts every token in warning status when it still has half its lifespan remaining.
2. Flawed Condition in `internal/watchdog/watchdog.go`:
   When `rem <= warnDuration`, watchdog calls `refreshFn(name)`.
   If the token still has 25 minutes left, `agy models` (Google's CLI) does not rewrite the token on disk because the token is not yet near expiry.
   When watchdog re-reads `newExp`, `newRem` is still 25 minutes.
   The code checked: `if newRem > warnDuration` (is 25m > 30m? NO).
   Because it was false, the code erroneously raised:
   `CRITICAL: Token refresh failed for %s: token still expired`!
   This sent false CRITICAL alerts to Bos Eka for active tokens that still had 25+ minutes of validity!

## Required Changes

### 1. `cmd/bot/main.go`
- Change `TokenWarningMinutes: 30` to `TokenWarningMinutes: 5` (or `10`).

### 2. `internal/watchdog/watchdog.go`
- In `checkTokens()`:
  When checking tokens, only trigger refresh when `rem <= warnDuration` (e.g. <= 5 minutes or <= 0).
  After `refreshFn`:
  - Read `newExp`.
  - Calculate `newRem := newExp.Sub(time.Now())`.
  - If `newRem > 0`:
    The token is **VALID AND ACTIVE** (`int(newRem.Minutes())` minutes remaining).
    Clean up any active warning/critical in `w.lastNotified`.
    Log `Token for %s is valid (%d minutes remaining)`.
    Do NOT generate any CRITICAL alert!
  - If `newRem <= 0`:
    The token is genuinely expired.
    Fire `CRITICAL` alert: `Token for %s expired at %s (auto-refresh failed: %v)`.

### 3. `internal/watchdog/watchdog_test.go`
- Update unit tests to verify:
  - If a token has positive remaining time after refresh attempt (e.g. 15 minutes or 25 minutes), no CRITICAL alert is generated.
  - If a token has expired (`newRem <= 0`) and refresh fails, a CRITICAL alert is generated.
- Run `export PATH="/opt/data/home/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64/bin:$PATH" && go test ./...` and ensure all tests PASS.
