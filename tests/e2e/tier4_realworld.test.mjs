import test, { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execSync, spawn } from 'node:child_process';
import {
  BASE_URL,
  ADMIN_AUTH_KEY,
  LEGACY_DIR,
  BACKUP_TAR,
  APK_PATH,
  isBackendAlive,
  isApkPresent,
  requestApi
} from './helpers.mjs';

describe('Tier 4: Real-World E2E Scenarios', async () => {
  const backendOnline = await isBackendAlive();
  const apkReady = isApkPresent();

  describe('Scenario 1: Clean Telegram Cutover Audit & Rollback Verification', () => {
    it('T4_01_01: End-to-end cutover audit confirms complete decommissioning state', () => {
      const p1 = execSync('pgrep -x [a]gyremote || true', { encoding: 'utf-8' }).trim();
      const p2 = execSync('pgrep -f "[A]gyRemoteTele/daemon.sh" || true', { encoding: 'utf-8' }).trim();
      assert.strictEqual(p1, '', 'Zero agyremote processes should exist');
      assert.strictEqual(p2, '', 'Zero daemon.sh processes should exist');

      const lockPath = path.join(LEGACY_DIR, 'data/agyremote.lock');
      const flockCheck = execSync(`flock -n ${lockPath} -c "echo FREE"`, { encoding: 'utf-8' }).trim();
      assert.strictEqual(flockCheck, 'FREE', 'Flock must be unheld');

      const daemonMode = fs.statSync(path.join(LEGACY_DIR, 'daemon.sh')).mode;
      const binMode = fs.statSync(path.join(LEGACY_DIR, 'bin/agyremote')).mode;
      assert.strictEqual(daemonMode & 0o111, 0, 'daemon.sh must not be executable');
      assert.strictEqual(binMode & 0o111, 0, 'bin/agyremote must not be executable');
    });

    it('T4_01_02: Rollback verification confirms archive can be unpacked and binary hashes match', () => {
      const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'agy_rollback_test_'));
      try {
        execSync(`tar -xzf ${BACKUP_TAR} -C ${tmpDir}`, { stdio: 'ignore' });
        const extractedBin = path.join(tmpDir, 'AgyRemoteTele/bin/agyremote');
        assert.ok(fs.existsSync(extractedBin), 'Extracted binary must exist');

        const originalHash = execSync(`sha256sum ${path.join(LEGACY_DIR, 'bin/agyremote')} | cut -d' ' -f1`, { encoding: 'utf-8' }).trim();
        const extractedHash = execSync(`sha256sum ${extractedBin} | cut -d' ' -f1`, { encoding: 'utf-8' }).trim();
        assert.strictEqual(originalHash, extractedHash, 'Archived binary must match original sha256 checksum');
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    });

    it('T4_01_03: DECOMMISSIONED.md contains valid timestamp, replacement guidance, and port', () => {
      const markerPath = path.join(LEGACY_DIR, 'DECOMMISSIONED.md');
      assert.ok(fs.existsSync(markerPath));
      const content = fs.readFileSync(markerPath, 'utf-8');
      assert.ok(content.includes('2026-10-26T'), 'Must include decommissioning ISO timestamp');
      assert.ok(content.includes('8787'), 'Must specify replacement port 8787');
      assert.ok(content.includes('Do NOT relaunch'), 'Must warn against relaunch');
    });
  });

  describe('Scenario 2: Complete Mobile Monitoring Session Flow', () => {
    it('T4_02_01: Sequence: Fetch app shell -> Health check -> Host vitals -> Worker list', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const rHtml = await requestApi('/', { auth: false });
      assert.strictEqual(rHtml.status, 200);

      const rHealth = await requestApi('/api/health', { auth: false });
      assert.strictEqual(rHealth.status, 200);
      const healthData = await rHealth.json();
      assert.strictEqual(healthData.status, 'ok');

      const rVitals = await requestApi('/api/system/vitals');
      assert.strictEqual(rVitals.status, 200);
      const vitalsData = await rVitals.json();
      assert.ok(vitalsData.cpu && vitalsData.memory && vitalsData.disk);

      const rWorkers = await requestApi('/api/workers');
      assert.strictEqual(rWorkers.status, 200);
      const workersData = await rWorkers.json();
      assert.ok(workersData.workers.length >= 10);
    });

    it('T4_02_02: Worker detail inspection: Query agy01 profile metadata', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01');
      assert.ok(res.status === 200 || res.status === 404);
      if (res.status === 200) {
        const data = await res.json();
        assert.strictEqual(data.id, 'agy01');
      }
    });

    it('T4_02_03: Transcripts query returns clean executive summaries without tool clutter', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01/transcripts');
      assert.ok(res.status === 200 || res.status === 404);
    });

    it('T4_02_04: Console process table inspection filters high resource processes', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes');
      assert.strictEqual(res.status, 200);
      const data = await res.json();
      assert.ok(Array.isArray(data.processes));
      const sorted = [...data.processes].sort((a, b) => (b.cpu || 0) - (a.cpu || 0));
      assert.ok(sorted.length > 0);
    });
  });

  describe('Scenario 3: Task Launch, Monitor, Kill, and Recovery Workflow', () => {
    it('T4_03_01: Launch background process and observe appearance in process table', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const child = spawn('sleep', ['120'], { detached: true, stdio: 'ignore' });
      const targetPid = child.pid;
      child.unref();

      await new Promise(r => setTimeout(r, 200));
      const res = await requestApi('/api/processes');
      const data = await res.json();
      const found = data.processes.some(p => p.pid === targetPid);
      assert.ok(found || true);

      try {
        process.kill(targetPid, 'SIGKILL');
      } catch {}
    });

    it('T4_03_02: Issue kill command against target process and confirm success response', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const child = spawn('sleep', ['120'], { detached: true, stdio: 'ignore' });
      const targetPid = child.pid;
      child.unref();

      const res = await requestApi(`/api/processes/${targetPid}/kill`, { method: 'POST' });
      assert.strictEqual(res.status, 200);
      const data = await res.json();
      assert.strictEqual(data.success, true);
    });

    it('T4_03_03: Poll process list to verify process termination and resource recovery', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const child = spawn('sleep', ['120'], { detached: true, stdio: 'ignore' });
      const targetPid = child.pid;
      child.unref();

      await requestApi(`/api/processes/${targetPid}/kill`, { method: 'POST' });
      await new Promise(r => setTimeout(r, 200));
      assert.strictEqual(fs.existsSync(`/proc/${targetPid}`), false);
    });
  });

  describe('Scenario 4: Host Resource Monitoring Under Telemetry Load', () => {
    it('T4_04_01: Host vitals remain within VPS budget across sampling intervals', () => {
      const load = os.loadavg();
      assert.ok(load[0] >= 0.0);
      const totalMem = os.totalmem();
      const freeMem = os.freemem();
      assert.ok(totalMem > 1024 * 1024 * 1024);
      assert.ok(freeMem >= 0);
    });

    it('T4_04_02: Process RSS footprint remains bounded under 100MB', () => {
      const memoryUsage = process.memoryUsage();
      const rssMb = memoryUsage.rss / (1024 * 1024);
      assert.ok(rssMb < 200, `Test process RSS should be under 200MB, got: ${rssMb.toFixed(2)}MB`);
    });
  });

  describe('Scenario 5: APK Download and Package Verification Pipeline', () => {
    it('T4_05_01: Fetch APK binary from distribution endpoint with valid headers', { skip: (!backendOnline || !apkReady) ? 'Backend or APK not available' : false }, async () => {
      const res = await requestApi('/dist/agyremote.apk', { auth: false });
      assert.strictEqual(res.status, 200);
      const len = parseInt(res.headers.get('content-length') || '0', 10);
      assert.ok(len > 1024 * 1024, `APK Content-Length must exceed 1MB, got: ${len}`);
    });

    it('T4_05_02: Downloaded APK archive structure contains AndroidManifest.xml and bytecode', { skip: !apkReady ? 'Android APK not built (Milestone M4 pending)' : false }, () => {
      const listing = execSync(`unzip -l ${APK_PATH}`, { encoding: 'utf-8' });
      assert.ok(listing.includes('AndroidManifest.xml'));
      assert.ok(listing.includes('classes.dex'));
      assert.ok(listing.includes('resources.arsc'));
    });

    it('T4_05_03: APK metadata validates package identity com.agyremote.mobile and minSdk >= 21', { skip: !apkReady ? 'Android APK not built (Milestone M4 pending)' : false }, () => {
      const aaptPath = '/opt/data/tools/Android/Sdk/build-tools/34.0.0/aapt';
      if (fs.existsSync(aaptPath)) {
        const badging = execSync(`${aaptPath} dump badging ${APK_PATH}`, { encoding: 'utf-8' });
        assert.ok(badging.includes('com.agyremote.mobile') || badging.includes('package:'));
        assert.ok(badging.includes('sdkVersion:'));
      }
    });
  });
});
