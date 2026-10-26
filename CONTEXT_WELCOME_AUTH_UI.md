# Specification: Fix Welcome Card & Session Auth UI

## Problem Analysis
When user sends `/start` to `@Orcyagy_bot`, `RenderWelcome` prints:
```
Status: 🟡 Unauthenticated
Active Worker: agy01
Working Directory: /opt/data/home
```
This causes major confusion:
1. The user believes `agy01` is "unauthenticated / no auth", even though `agy01`'s Google OAuth is 100% active (`eka.ckp16799@gmail.com`).
2. The `Status: 🟡 Unauthenticated` field actually referred to the Telegram device pairing session (`sess.IsAuthenticated`), NOT the Google OAuth status of `agy01`.
3. If the Telegram user's `chatID` is in `allowedChatIDs` (the whitelist from `.env`), the user IS an authorized administrator! They should never see `🟡 Unauthenticated`.

## Requirements for AGY04 (Telegram UI & Routing Owner)

### 1. `internal/bot/router.go`
- In `HandleUpdate()`:
  If `r.authManager.IsAuthorized(chatID, sess, r.sessionExpiration)` is true (e.g. chatID is in `allowedChatIDs`):
  Ensure `sess.IsAuthenticated = true` so the session store reflects that the admin is authorized.

### 2. `internal/bot/renderer.go`
- In `RenderWelcome(username string, defaultProfile string, sess *session.Session)`:
  Change the misleading labels:
  - Instead of ambiguous `Status:`, use:
    `<b>Telegram Session:</b> 🟢 Authorized Admin` (if `sess.IsAuthenticated` is true) or `🟡 Unpaired (Guest)` (if false).
  - For the worker:
    `<b>Active Worker:</b> <code>%s</code>`
- Make sure formatting follows HTML standard and antislop UI rules.

### 3. Tests (`internal/bot/renderer_test.go`, `internal/bot/router_test.go`)
- Update any assertions that check for `Status: 🟡 Unauthenticated` or `Status: 🟢 Authenticated` to match the new clearer labels.
- Run `export PATH="/opt/data/home/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64/bin:$PATH" && go test ./...` and ensure 100% PASS.
- Commit to branch `work/agy04-fix-welcome-auth-ui`.
