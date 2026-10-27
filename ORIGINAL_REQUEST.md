# Original User Request

## 2026-10-26T13:03:37Z

Build and package **agyremote**, a production-grade Android mobile application (.apk) and high-performance Node.js/TSX mobile control plane that fully replaces the Telegram bot interface, enabling comprehensive remote monitoring, live telemetry, and orchestration across all Google Antigravity (AGY) workers (agy01–agy15).

Working directory: /opt/data/home/agyremote
Integrity mode: development

## Requirements

### R1. Telegram Decommission & Archival
Archive the existing Telegram bot code and session data in `/opt/data/home/AgyRemoteTele` into a compressed backup (`AgyRemoteTele_backup.tar.gz`), stop the live Telegram bot daemon, and ensure zero duplicate daemon collisions.

### R2. Mobile-First Web Application (TSX/TS + SwiftUI UI Patterns)
Develop a responsive mobile application using Node.js, TypeScript, TSX, Vite, and TailwindCSS adhering to declarative SwiftUI mobile design principles:
- **Tab Architecture**: Sticky bottom navigation bar with icons (`Overview`, `Workers`, `Transcripts`, `Console`, `Settings`).
- **Cards & Stacks**: Clean grouped cards for worker profiles, health vitals (CPU, RAM, Disk), and running processes.
- **Modal Sheets**: Bottom sheet drawers for inspecting worker details, viewing logs, and launching tasks.
- **Accessibility & Performance**: Smooth 60 FPS rendering, zero layout shifts, and full `prefers-reduced-motion` compliance (Tier-3 opacity fades instead of large sliding displacements).
- **Dual Access**: Compiles into static bundle (`index.html`) served both as a standalone Progressive Web App (PWA) and inside the mobile wrapper.

### R3. Node.js Backend & Real-Time Telemetry API
Implement an asynchronous Node.js backend daemon providing REST and real-time streaming (SSE / WebSocket) endpoints for:
- Monitoring all AGY worker profiles (`agy01` through `agy15`) and their session status.
- Streaming live worker outputs and session transcripts with complete suppression of internal CLI tool clutter.
- Process inspection (`top`, `/proc`), CPU/memory metrics, and instant task termination (`/kill`).
- Secure bearer token authentication.

### R4. Cloudflare Tunnel Subdomain Integration
Configure and expose the backend service at `http://127.0.0.1:8787` through the active Cloudflare Tunnel under subdomain `agyremote.agentai.my.id`, ensuring CORS and WebSocket/SSE support.

### R5. Android APK Packaging
Assemble, sign, and build a standalone Android application package (`agyremote.apk`) using local portable Android/Java tooling or Capacitor, and place the output file in `/opt/data/home/agyremote/dist/agyremote.apk` with a direct one-tap download link in the web interface.

## Acceptance Criteria

### Core AGY Parity & Decommission
- [ ] Telegram bot daemon is cleanly stopped and backed up.
- [ ] All worker profiles (`agy01` to `agy15`) can be viewed, filtered, and managed via the new mobile interface.
- [ ] Prompts sent to AGY workers return clean executive summaries without leaking raw tool commands.

### Mobile UI Quality
- [ ] Mobile viewport renders responsively across 360px to 480px widths without horizontal scrolling.
- [ ] Bottom TabBar switches smoothly between tabs without page reloads.
- [ ] Bottom sheet modals open and dismiss cleanly.
- [ ] Reduced motion preference is honored gracefully.

### Connectivity & Distribution
- [ ] Backend API responds with HTTP 200 on `http://127.0.0.1:8787/api/health`.
- [ ] Web app is live and accessible on `https://agyremote.agentai.my.id`.
- [ ] Standalone `agyremote.apk` is generated, verified, and downloadable.
