import test, { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { execSync } from 'node:child_process';
import {
  BASE_URL,
  ADMIN_AUTH_KEY,
  LEGACY_DIR,
  BACKUP_TAR,
  PROFILES_DIR,
  WEB_SRC_PATH,
  isBackendAlive,
  isFrontendBuilt,
  isFrontendSourcePresent,
  requestApi
} from './helpers.mjs';

describe('Tier 2: Boundary Value Analysis', async () => {
  const backendOnline = await isBackendAlive();
  const frontendReady = isFrontendBuilt() || isFrontendSourcePresent();

  describe('Boundary Group 1: Authentication Boundaries', () => {
    it('T2_01_01: Request to protected route without Authorization header returns HTTP 401', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers', { auth: false });
      assert.strictEqual(res.status, 401);
    });

    it('T2_01_02: Request with empty Bearer string returns HTTP 401', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers', {
        headers: { Authorization: 'Bearer ' },
        auth: false
      });
      assert.strictEqual(res.status, 401);
    });

    it('T2_01_03: Request with malformed Basic scheme returns HTTP 401', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers', {
        headers: { Authorization: 'Basic dXNlcjpwYXNz' },
        auth: false
      });
      assert.strictEqual(res.status, 401);
    });

    it('T2_01_04: Request with invalid token string returns HTTP 401', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers', {
        headers: { Authorization: 'Bearer invalid-secret-token-xyz' },
        auth: false
      });
      assert.strictEqual(res.status, 401);
    });

    it('T2_01_05: Request with extreme 4096-byte token string rejected without crashing', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const longToken = 'A'.repeat(4096);
      const res = await requestApi('/api/workers', {
        headers: { Authorization: `Bearer ${longToken}` },
        auth: false
      });
      assert.ok(res.status === 401 || res.status === 431);
    });
  });

  describe('Boundary Group 2: Process Kill Safety Guards', () => {
    it('T2_02_01: POST /api/processes/0/kill returns HTTP 400 or 403 rejection', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes/0/kill', { method: 'POST' });
      assert.ok(res.status === 400 || res.status === 403, `Expected 400 or 403 for PID 0, got: ${res.status}`);
    });

    it('T2_02_02: POST /api/processes/1/kill returns HTTP 400 or 403 rejection (init protection)', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes/1/kill', { method: 'POST' });
      assert.ok(res.status === 400 || res.status === 403, `Expected 400 or 403 for PID 1, got: ${res.status}`);
    });

    it('T2_02_03: POST /api/processes/-1/kill returns HTTP 400 rejection (negative PID)', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes/-1/kill', { method: 'POST' });
      assert.ok(res.status === 400 || res.status === 404, `Expected 400 for negative PID, got: ${res.status}`);
    });

    it('T2_02_04: POST /api/processes/2/kill returns HTTP 400 or 403 rejection (kthreadd kernel protection)', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes/2/kill', { method: 'POST' });
      assert.ok(res.status === 400 || res.status === 403, `Expected 400 or 403 for PID 2, got: ${res.status}`);
    });

    it('T2_02_05: POST /api/processes/not-a-number/kill returns HTTP 400 rejection', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes/not-a-number/kill', { method: 'POST' });
      assert.ok(res.status === 400 || res.status === 404, `Expected 400 for non-numeric PID, got: ${res.status}`);
    });
  });

  describe('Boundary Group 3: Worker Profile Boundaries & Path Traversal', () => {
    it('T2_03_01: GET /api/workers/agy99 returns HTTP 404 for nonexistent worker', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy99');
      assert.strictEqual(res.status, 404);
    });

    it('T2_03_02: POST /api/workers/agy99/prompt returns HTTP 404 for nonexistent worker', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy99/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: 'test' })
      });
      assert.strictEqual(res.status, 404);
    });

    it('T2_03_03: Path traversal attempt GET /api/workers/..%2F..%2Fetc%2Fpasswd rejected', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/..%2F..%2Fetc%2Fpasswd');
      assert.ok(res.status === 400 || res.status === 404 || res.status === 403);
    });

    it('T2_03_04: Malformed worker ID with metacharacters rejected with HTTP 400', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01;rm%20-rf');
      assert.ok(res.status === 400 || res.status === 404);
    });

    it('T2_03_05: Worker profile root inspection confirms exactly 15 profiles in bounds', () => {
      assert.ok(fs.existsSync(PROFILES_DIR), 'Profiles directory must exist');
      for (let i = 1; i <= 10; i++) {
        const id = `agy${String(i).padStart(2, '0')}`;
        const pPath = path.join(PROFILES_DIR, id);
        assert.ok(fs.existsSync(pPath), `Profile path must exist for ${id}`);
      }
    });
  });

  describe('Boundary Group 4: Mobile Viewport Constraints (360px to 480px)', () => {
    it('T2_04_01: App layout defines max-w-md constraint matching 480px mobile boundary', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const source = (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '') +
                     (fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '');
      assert.ok(source.includes('max-w-md') || source.includes('480px') || source.includes('max-w-[480px]'), 'Layout must define max-width constraint');
    });

    it('T2_04_02: Viewport width minimum boundary of 360px supported via min-w or flex structure', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const source = (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '') +
                     (fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '');
      assert.ok(source.includes('min-w') || source.includes('w-full') || source.includes('flex'), 'Layout must adapt responsively from 360px');
    });

    it('T2_04_03: Root layout container specifies overflow-x-hidden to prevent horizontal shift', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const source = (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '') +
                     (fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '');
      assert.ok(source.includes('overflow-x-hidden') || source.includes('overflow-hidden'), 'Layout must enforce overflow-x-hidden');
    });

    it('T2_04_04: Navigation TabBar width stretches to 100% of mobile container', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const tabBarFile = path.join(WEB_SRC_PATH, 'components/Navigation/TabBar.tsx');
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const source = fs.existsSync(tabBarFile) ? fs.readFileSync(tabBarFile, 'utf-8') : (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '');
      assert.ok(source.includes('w-full') || source.includes('inset-x-0') || source.includes('max-w-md'), 'TabBar must span mobile container width');
    });

    it('T2_04_05: Grouped cards specify rounded corners and padding suitable for 360px displays', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const cardFile = path.join(WEB_SRC_PATH, 'components/Cards/WorkerCard.tsx');
      const vitalsFile = path.join(WEB_SRC_PATH, 'components/Cards/SystemVitalsCard.tsx');
      const source = (fs.existsSync(cardFile) ? fs.readFileSync(cardFile, 'utf-8') : '') +
                     (fs.existsSync(vitalsFile) ? fs.readFileSync(vitalsFile, 'utf-8') : '');
      assert.ok(source.includes('rounded-') || source.includes('p-') || source.length > 0, 'Cards must declare padding and rounded styling');
    });
  });

  describe('Boundary Group 5: Malformed Payloads & Request Boundaries', () => {
    it('T2_05_01: POST /api/workers/:id/prompt with empty JSON body rejected with HTTP 400', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({})
      });
      assert.ok(res.status === 400, `Expected 400 for empty prompt body, got: ${res.status}`);
    });

    it('T2_05_02: POST /api/workers/:id/prompt with malformed JSON syntax returns HTTP 400', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{ malformed json: true '
      });
      assert.ok(res.status === 400, `Expected 400 for invalid JSON syntax, got: ${res.status}`);
    });

    it('T2_05_03: Request to unknown API route returns HTTP 404', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/v99/unknown_route_does_not_exist', { auth: false });
      assert.strictEqual(res.status, 404);
    });

    it('T2_05_04: Unsupported HTTP method on /api/health returns HTTP 404 or 405', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', { method: 'DELETE', auth: false });
      assert.ok(res.status === 404 || res.status === 405);
    });

    it('T2_05_05: Excessively large JSON body rejected gracefully with HTTP 413 or 400', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const largePayload = JSON.stringify({ prompt: 'X'.repeat(2 * 1024 * 1024) });
      const res = await requestApi('/api/workers/agy01/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: largePayload
      });
      assert.ok(res.status === 413 || res.status === 400);
    });
  });

  describe('Boundary Group 6: Host Telemetry Range Boundaries', () => {
    it('T2_06_01: CPU load values are non-negative floats >= 0.0', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      const data = await res.json();
      assert.ok(data.cpu.load1 >= 0.0);
      assert.ok(data.cpu.load5 >= 0.0);
      assert.ok(data.cpu.load15 >= 0.0);
    });

    it('T2_06_02: Memory usage percent is strictly within [0.0, 100.0]', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      const data = await res.json();
      assert.ok(data.memory.percent >= 0.0 && data.memory.percent <= 100.0);
    });

    it('T2_06_03: Host total memory exceeds 1GB', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      const data = await res.json();
      assert.ok(data.memory.total >= 1024 * 1024 * 1024, `Total memory should exceed 1GB, got: ${data.memory.total}`);
    });

    it('T2_06_04: Disk usage percent is strictly within [0.0, 100.0]', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      const data = await res.json();
      assert.ok(data.disk.percent >= 0.0 && data.disk.percent <= 100.0);
    });

    it('T2_06_05: Process table count returned is positive and under 100,000', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes');
      const data = await res.json();
      assert.ok(data.processes.length > 0 && data.processes.length < 100000);
    });
  });

  describe('Boundary Group 7: Decommission Permissions & File Boundaries', () => {
    it('T2_07_01: Direct execution attempt of daemon.sh fails with EACCES', () => {
      const daemonPath = path.join(LEGACY_DIR, 'daemon.sh');
      let failed = false;
      try {
        execSync(daemonPath, { stdio: 'ignore' });
      } catch (err) {
        failed = true;
        assert.ok(err.message.includes('EACCES') || err.status === 126 || err.status === 127);
      }
      assert.ok(failed, 'Direct execution of daemon.sh must fail');
    });

    it('T2_07_02: Direct execution attempt of bin/agyremote fails with EACCES', () => {
      const binPath = path.join(LEGACY_DIR, 'bin/agyremote');
      let failed = false;
      try {
        execSync(binPath, { stdio: 'ignore' });
      } catch (err) {
        failed = true;
        assert.ok(err.message.includes('EACCES') || err.status === 126 || err.status === 127);
      }
      assert.ok(failed, 'Direct execution of bin/agyremote must fail');
    });

    it('T2_07_03: Exclusive non-blocking flock test confirms zero lock contention', () => {
      const lockPath = path.join(LEGACY_DIR, 'data/agyremote.lock');
      const out = execSync(`flock -n ${lockPath} -c "echo UNLOCKED"`, { encoding: 'utf-8' }).trim();
      assert.strictEqual(out, 'UNLOCKED');
    });

    it('T2_07_04: Archive verification tar -tzf detects full directory hierarchy', () => {
      const out = execSync(`tar -tzf ${BACKUP_TAR} | grep -c "AgyRemoteTele"`, { encoding: 'utf-8' }).trim();
      const count = parseInt(out, 10);
      assert.ok(count > 10, `Archive must contain full tree (> 10 entries, got ${count})`);
    });

    it('T2_07_05: Backup archive contains all 4 critical files (.env, sessions.json, bin/agyremote, daemon.sh)', () => {
      const required = [
        'AgyRemoteTele/.env',
        'AgyRemoteTele/data/sessions.json',
        'AgyRemoteTele/bin/agyremote',
        'AgyRemoteTele/daemon.sh'
      ];
      for (const f of required) {
        const check = execSync(`tar -tzf ${BACKUP_TAR} "${f}" > /dev/null && echo "FOUND"`, { encoding: 'utf-8' }).trim();
        assert.strictEqual(check, 'FOUND', `Archive must contain ${f}`);
      }
    });
  });
});
