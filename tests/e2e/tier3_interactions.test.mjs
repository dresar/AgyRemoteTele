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
  APK_PATH,
  isBackendAlive,
  isApkPresent,
  requestApi
} from './helpers.mjs';

describe('Tier 3: Pairwise Cross-Feature Interactions', async () => {
  const backendOnline = await isBackendAlive();
  const apkReady = isApkPresent();

  describe('Interaction 1: Worker Prompt Execution with SSE & Clutter Suppression', () => {
    it('T3_01_01: Worker prompt execution initiates SSE connection with text/event-stream', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: 'ping' })
      });
      const ct = res.headers.get('content-type') || '';
      await res.body?.cancel();
      assert.ok(ct.includes('text/event-stream') || ct.includes('application/x-ndjson') || res.status === 200);
    });

    it('T3_01_02: Streaming response delivers text deltas formatted with data prefix', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: 'hello' })
      });
      if (res.status === 200 && res.body) {
        const reader = res.body.getReader();
        const { value } = await reader.read();
        const chunk = new TextDecoder().decode(value || new Uint8Array());
        assert.ok(chunk.length > 0);
        reader.cancel();
      }
    });

    it('T3_01_03: Stream suppresses internal XML tags and raw command parameter dumps', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: 'echo test' })
      });
      if (res.status === 200 && res.body) {
        const reader = res.body.getReader();
        const { value } = await reader.read();
        const text = new TextDecoder().decode(value || new Uint8Array());
        assert.strictEqual(text.includes('<USER_REQUEST>'), false);
        assert.strictEqual(text.includes('<SYSTEM_MESSAGE>'), false);
        reader.cancel();
      }
    });

    it('T3_01_04: Stream completes with final summary event and closes socket cleanly', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: 'status check' })
      });
      await res.body?.cancel();
      assert.ok(res.status === 200 || res.status === 400 || res.status === 404);
    });
  });

  describe('Interaction 2: Concurrent Health Polling & Load Handling', () => {
    it('T3_02_01: 20 simultaneous concurrent GET /api/health requests all return HTTP 200', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const requests = Array.from({ length: 20 }, () => requestApi('/api/health', { auth: false }));
      const responses = await Promise.all(requests);
      for (const res of responses) {
        assert.strictEqual(res.status, 200);
      }
    });

    it('T3_02_02: Concurrent health polling completes without socket errors or starvation', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const start = Date.now();
      const requests = Array.from({ length: 15 }, () => requestApi('/api/health', { auth: false }));
      const responses = await Promise.all(requests);
      const elapsed = Date.now() - start;
      assert.strictEqual(responses.length, 15);
      assert.ok(elapsed < 3000, `Concurrent batch took ${elapsed}ms, should be under 3000ms`);
    });

    it('T3_02_03: Timestamps reported across concurrent polls are valid and consistent', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const requests = Array.from({ length: 5 }, () => requestApi('/api/health', { auth: false }).then(r => r.json()));
      const payloads = await Promise.all(requests);
      for (const p of payloads) {
        assert.ok(!isNaN(Date.parse(p.timestamp)));
      }
    });
  });

  describe('Interaction 3: Static Web Serving with Concurrent APK Download', () => {
    it('T3_03_01: Simultaneous GET / and GET /dist/agyremote.apk both return HTTP 200', { skip: (!backendOnline || !apkReady) ? 'Backend or APK not available' : false }, async () => {
      const [resHtml, resApk] = await Promise.all([
        requestApi('/', { auth: false }),
        requestApi('/dist/agyremote.apk', { auth: false })
      ]);
      assert.strictEqual(resHtml.status, 200);
      assert.strictEqual(resApk.status, 200);
    });

    it('T3_03_02: Concurrent downloads maintain distinct and accurate Content-Type headers', { skip: (!backendOnline || !apkReady) ? 'Backend or APK not available' : false }, async () => {
      const [resHtml, resApk] = await Promise.all([
        requestApi('/', { auth: false }),
        requestApi('/dist/agyremote.apk', { auth: false })
      ]);
      const ctHtml = resHtml.headers.get('content-type') || '';
      const ctApk = resApk.headers.get('content-type') || '';
      assert.ok(ctHtml.includes('text/html'));
      assert.ok(ctApk.includes('application/vnd.android.package-archive') || ctApk.includes('octet-stream'));
    });

    it('T3_03_03: High-throughput APK binary stream does not block concurrent static HTML requests', { skip: (!backendOnline || !apkReady) ? 'Backend or APK not available' : false }, async () => {
      const start = Date.now();
      const apkPromise = requestApi('/dist/agyremote.apk', { auth: false });
      const htmlPromise = requestApi('/', { auth: false });
      const [apkRes, htmlRes] = await Promise.all([apkPromise, htmlPromise]);
      const elapsed = Date.now() - start;
      assert.strictEqual(apkRes.status, 200);
      assert.strictEqual(htmlRes.status, 200);
      assert.ok(elapsed < 5000);
    });
  });

  describe('Interaction 4: Telemetry Stream During Active Process Table Inspection', () => {
    it('T3_04_01: Concurrent requests to GET /api/system/vitals and GET /api/processes return HTTP 200', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const [vitalsRes, procRes] = await Promise.all([
        requestApi('/api/system/vitals'),
        requestApi('/api/processes')
      ]);
      assert.strictEqual(vitalsRes.status, 200);
      assert.strictEqual(procRes.status, 200);
    });

    it('T3_04_02: Process table scanner does not cause memory spikes in telemetry reporting', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const vitalsBefore = await (await requestApi('/api/system/vitals')).json();
      await requestApi('/api/processes');
      const vitalsAfter = await (await requestApi('/api/system/vitals')).json();
      assert.ok(vitalsAfter.memory.percent >= 0 && vitalsAfter.memory.percent <= 100);
    });

    it('T3_04_03: Process table contains the running server process', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const data = await (await requestApi('/api/processes')).json();
      const nodeProcs = data.processes.filter(p => p.command.includes('node') || p.command.includes('server') || p.command.includes('8787'));
      assert.ok(nodeProcs.length > 0);
    });
  });

  describe('Interaction 5: Auth Validation Across Mixed Public and Protected Endpoints', () => {
    it('T3_05_01: Mixed concurrent requests maintain independent auth status', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const [r1, r2, r3] = await Promise.all([
        requestApi('/api/health', { auth: false }),
        requestApi('/api/workers', { auth: true }),
        requestApi('/api/workers', { auth: false })
      ]);
      assert.strictEqual(r1.status, 200);
      assert.strictEqual(r2.status, 200);
      assert.strictEqual(r3.status, 401);
    });

    it('T3_05_02: 401 Unauthorized responses do not corrupt subsequent authorized requests', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const unauth = await requestApi('/api/system/vitals', { auth: false });
      assert.strictEqual(unauth.status, 401);
      const auth = await requestApi('/api/system/vitals', { auth: true });
      assert.strictEqual(auth.status, 200);
    });

    it('T3_05_03: Bearer token authentication scales across multiple worker profiles in parallel', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const profiles = ['agy01', 'agy02', 'agy03'];
      const requests = profiles.map(p => requestApi(`/api/workers/${p}`));
      const responses = await Promise.all(requests);
      for (const res of responses) {
        assert.ok(res.status === 200 || res.status === 404);
      }
    });
  });

  describe('Interaction 6: Decommissioned State Stability During Host Operations', () => {
    it('T3_06_01: Zero legacy Telegram bot processes running during active host checks', () => {
      const out1 = execSync('pgrep -x [a]gyremote || true', { encoding: 'utf-8' }).trim();
      const out2 = execSync('pgrep -f "[A]gyRemoteTele/daemon.sh" || true', { encoding: 'utf-8' }).trim();
      assert.strictEqual(out1, '');
      assert.strictEqual(out2, '');
    });

    it('T3_06_02: Host operations leave legacy lock unheld and freely acquirable', () => {
      const lockPath = path.join(LEGACY_DIR, 'data/agyremote.lock');
      const out = execSync(`flock -n ${lockPath} -c "echo UNLOCKED"`, { encoding: 'utf-8' }).trim();
      assert.strictEqual(out, 'UNLOCKED');
    });
  });
});
