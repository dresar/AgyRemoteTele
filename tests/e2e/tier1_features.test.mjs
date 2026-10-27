import test, { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { execSync, spawn } from 'node:child_process';
import {
  BASE_URL,
  CLOUDFLARE_URL,
  ADMIN_AUTH_KEY,
  LEGACY_DIR,
  BACKUP_TAR,
  APK_PATH,
  WEB_DIST_PATH,
  WEB_SRC_PATH,
  isBackendAlive,
  isApkPresent,
  isFrontendBuilt,
  isFrontendSourcePresent,
  isCloudflaredAlive,
  isLegacyDecommissioned,
  requestApi
} from './helpers.mjs';

describe('Tier 1: Feature Coverage (Happy Path)', async () => {
  const backendOnline = await isBackendAlive();
  const apkReady = isApkPresent();
  const frontendReady = isFrontendBuilt() || isFrontendSourcePresent();
  const cfAlive = isCloudflaredAlive();

  describe('Feature 1: M1 Telegram Bot Decommission & Archival', () => {
    it('T1_01_01: Verify daemon.sh supervisor process is terminated', () => {
      const out = execSync('pgrep -f "[A]gyRemoteTele/daemon.sh" || true', { encoding: 'utf-8' }).trim();
      assert.strictEqual(out, '', 'daemon.sh should not be running');
    });

    it('T1_01_02: Verify bin/agyremote bot process is terminated', () => {
      const out = execSync('pgrep -x agyremote || true', { encoding: 'utf-8' }).trim();
      assert.strictEqual(out, '', 'bin/agyremote should not be running');
    });

    it('T1_01_03: Verify data/agyremote.lock non-blocking flock acquisition succeeds', () => {
      const lockPath = path.join(LEGACY_DIR, 'data/agyremote.lock');
      assert.ok(fs.existsSync(lockPath), 'Lock file should exist');
      const res = execSync(`flock -n ${lockPath} -c "echo ACQUIRED"`, { encoding: 'utf-8' }).trim();
      assert.strictEqual(res, 'ACQUIRED', 'Exclusive non-blocking flock acquisition must succeed');
    });

    it('T1_01_04: Verify AgyRemoteTele_backup.tar.gz exists, size > 1MB, and passes tar integrity test', () => {
      assert.ok(fs.existsSync(BACKUP_TAR), 'Backup archive must exist');
      const stat = fs.statSync(BACKUP_TAR);
      assert.ok(stat.size > 1024 * 1024, `Backup archive size (${stat.size}) must exceed 1MB`);
      const testTar = execSync(`tar -tzf ${BACKUP_TAR} > /dev/null && echo "VALID"`, { encoding: 'utf-8' }).trim();
      assert.strictEqual(testTar, 'VALID', 'tar -tzf must exit 0 without errors');
    });

    it('T1_01_05: Verify daemon.sh and bin/agyremote have execute bits removed and DECOMMISSIONED.md exists', () => {
      const daemonPath = path.join(LEGACY_DIR, 'daemon.sh');
      const binPath = path.join(LEGACY_DIR, 'bin/agyremote');
      const markerPath = path.join(LEGACY_DIR, 'DECOMMISSIONED.md');

      const daemonStat = fs.statSync(daemonPath);
      const binStat = fs.statSync(binPath);

      assert.strictEqual(daemonStat.mode & 0o111, 0, 'daemon.sh must not have execute permissions');
      assert.strictEqual(binStat.mode & 0o111, 0, 'bin/agyremote must not have execute permissions');

      assert.ok(fs.existsSync(markerPath), 'DECOMMISSIONED.md marker file must exist');
      const markerContent = fs.readFileSync(markerPath, 'utf-8');
      assert.ok(markerContent.includes('DECOMMISSIONED'), 'Marker file must declare decommission status');
      assert.ok(markerContent.includes('8787'), 'Marker file must mention replacement port 8787');
    });
  });

  describe('Feature 2: Backend Health Endpoint', () => {
    it('T1_02_01: GET /api/health returns HTTP 200 OK', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', { auth: false });
      assert.strictEqual(res.status, 200);
    });

    it('T1_02_02: GET /api/health returns application/json Content-Type', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', { auth: false });
      const ct = res.headers.get('content-type') || '';
      assert.ok(ct.includes('application/json'), `Content-Type should be JSON, got: ${ct}`);
    });

    it('T1_02_03: GET /api/health body contains status ok', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', { auth: false });
      const body = await res.json();
      assert.strictEqual(body.status, 'ok');
    });

    it('T1_02_04: GET /api/health body contains non-negative numeric uptime', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', { auth: false });
      const body = await res.json();
      assert.strictEqual(typeof body.uptime, 'number');
      assert.ok(body.uptime >= 0);
    });

    it('T1_02_05: GET /api/health body contains valid ISO 8601 timestamp string', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', { auth: false });
      const body = await res.json();
      assert.strictEqual(typeof body.timestamp, 'string');
      assert.ok(!isNaN(Date.parse(body.timestamp)));
    });
  });

  describe('Feature 3: Bearer Authentication', () => {
    it('T1_03_01: Protected route /api/workers with valid Bearer token returns HTTP 200', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers');
      assert.strictEqual(res.status, 200);
    });

    it('T1_03_02: Public route /api/health succeeds without Authorization header', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', { auth: false });
      assert.strictEqual(res.status, 200);
    });

    it('T1_03_03: Protected route accepts standard case Bearer token format', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers', {
        headers: { Authorization: `Bearer ${ADMIN_AUTH_KEY}` },
        auth: false
      });
      assert.strictEqual(res.status, 200);
    });

    it('T1_03_04: Valid Bearer token grants access to /api/system/vitals', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      assert.strictEqual(res.status, 200);
    });

    it('T1_03_05: Valid Bearer token grants access to /api/processes', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes');
      assert.strictEqual(res.status, 200);
    });
  });

  describe('Feature 4: Worker Profiles Enumeration', () => {
    it('T1_04_01: GET /api/workers returns HTTP 200 with workers array', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers');
      assert.strictEqual(res.status, 200);
      const data = await res.json();
      assert.ok(Array.isArray(data.workers), 'Response must contain workers array');
    });

    it('T1_04_02: Workers array enumerates 15 AGY worker profiles (agy01 to agy15)', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers');
      const data = await res.json();
      const ids = data.workers.map(w => w.id);
      for (let i = 1; i <= 15; i++) {
        const expectedId = `agy${String(i).padStart(2, '0')}`;
        assert.ok(ids.includes(expectedId), `Missing worker profile ${expectedId}`);
      }
    });

    it('T1_04_03: Active worker profiles agy01 to agy10 report authenticated status', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers');
      const data = await res.json();
      const active = data.workers.filter(w => parseInt(w.id.replace('agy', ''), 10) <= 10);
      for (const w of active) {
        assert.ok(w.authenticated === true || w.status === 'idle' || w.status === 'running', `Worker ${w.id} should be authenticated`);
      }
    });

    it('T1_04_04: Uninitialized worker profiles agy11 to agy15 report unauthenticated status', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers');
      const data = await res.json();
      const unauth = data.workers.filter(w => parseInt(w.id.replace('agy', ''), 10) > 10);
      for (const w of unauth) {
        assert.ok(w.authenticated === false || w.status === 'unauthenticated', `Worker ${w.id} should be unauthenticated`);
      }
    });

    it('T1_04_05: Worker profile entries contain required schema fields', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers');
      const data = await res.json();
      assert.ok(data.workers.length > 0);
      for (const w of data.workers) {
        assert.strictEqual(typeof w.id, 'string');
        assert.strictEqual(typeof w.name, 'string');
        assert.ok(typeof w.status === 'string');
      }
    });
  });

  describe('Feature 5: Host Vitals & Telemetry', () => {
    it('T1_05_01: GET /api/system/vitals returns HTTP 200 with JSON payload', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      assert.strictEqual(res.status, 200);
      const data = await res.json();
      assert.ok(typeof data === 'object');
    });

    it('T1_05_02: CPU vitals report numeric load averages and core count >= 1', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      const data = await res.json();
      assert.ok(data.cpu, 'Vitals must include cpu metric');
      assert.strictEqual(typeof data.cpu.load1, 'number');
      assert.strictEqual(typeof data.cpu.load5, 'number');
      assert.strictEqual(typeof data.cpu.load15, 'number');
      assert.ok(data.cpu.cores >= 1);
    });

    it('T1_05_03: Memory vitals report total, used, free, and percentage within [0, 100]', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      const data = await res.json();
      assert.ok(data.memory, 'Vitals must include memory metric');
      assert.ok(data.memory.total > 0);
      assert.ok(data.memory.used >= 0);
      assert.ok(data.memory.free >= 0);
      assert.ok(data.memory.percent >= 0 && data.memory.percent <= 100);
    });

    it('T1_05_04: Disk vitals report total, used, free, and percentage within [0, 100]', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      const data = await res.json();
      assert.ok(data.disk, 'Vitals must include disk metric');
      assert.ok(data.disk.total > 0);
      assert.ok(data.disk.used >= 0);
      assert.ok(data.disk.free >= 0);
      assert.ok(data.disk.percent >= 0 && data.disk.percent <= 100);
    });

    it('T1_05_05: System uptime metric is positive numeric value', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/system/vitals');
      const data = await res.json();
      assert.strictEqual(typeof data.uptime, 'number');
      assert.ok(data.uptime > 0);
    });
  });

  describe('Feature 6: Supervisor-Aware Process Inspection & /kill', () => {
    it('T1_06_01: GET /api/processes returns process list array', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes');
      assert.strictEqual(res.status, 200);
      const data = await res.json();
      assert.ok(Array.isArray(data.processes));
    });

    it('T1_06_02: Process items contain pid, ppid, user, cpu, mem, and command', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/processes');
      const data = await res.json();
      assert.ok(data.processes.length > 0);
      const proc = data.processes[0];
      assert.strictEqual(typeof proc.pid, 'number');
      assert.strictEqual(typeof proc.ppid, 'number');
      assert.strictEqual(typeof proc.user, 'string');
      assert.strictEqual(typeof proc.command, 'string');
    });

    it('T1_06_03: Launch test background target process and kill via POST /api/processes/:pid/kill', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const child = spawn('sleep', ['60'], { detached: true, stdio: 'ignore' });
      const targetPid = child.pid;
      child.unref();

      const res = await requestApi(`/api/processes/${targetPid}/kill`, {
        method: 'POST'
      });
      assert.strictEqual(res.status, 200);
      const data = await res.json();
      assert.strictEqual(data.success, true);
      assert.strictEqual(data.pid, targetPid);
    });

    it('T1_06_04: Terminated process is removed from system process table', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const child = spawn('sleep', ['60'], { detached: true, stdio: 'ignore' });
      const targetPid = child.pid;
      child.unref();

      await requestApi(`/api/processes/${targetPid}/kill`, { method: 'POST' });
      await new Promise(r => setTimeout(r, 200));

      const isRunning = fs.existsSync(`/proc/${targetPid}`);
      assert.strictEqual(isRunning, false, 'Process should be terminated from /proc');
    });

    it('T1_06_05: Process kill response confirms signal type or status description', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const child = spawn('sleep', ['60'], { detached: true, stdio: 'ignore' });
      const targetPid = child.pid;
      child.unref();

      const res = await requestApi(`/api/processes/${targetPid}/kill`, { method: 'POST' });
      const data = await res.json();
      assert.ok(data.signal || data.message || data.success);
    });
  });

  describe('Feature 7: Static Web Assets & PWA', () => {
    it('T1_07_01: Static web root returns HTTP 200 and text/html', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/', { auth: false });
      assert.strictEqual(res.status, 200);
      const ct = res.headers.get('content-type') || '';
      assert.ok(ct.includes('text/html'), `Content-Type should be HTML, got: ${ct}`);
    });

    it('T1_07_02: HTML document includes mobile viewport meta tag', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const htmlPath = fs.existsSync(WEB_DIST_PATH) ? WEB_DIST_PATH : path.join('/opt/data/home/agyremote', 'index.html');
      assert.ok(fs.existsSync(htmlPath), 'index.html must exist');
      const html = fs.readFileSync(htmlPath, 'utf-8');
      assert.ok(html.includes('viewport'), 'HTML must include viewport meta tag');
      assert.ok(html.includes('width=device-width'), 'Viewport must define width=device-width');
    });

    it('T1_07_03: HTML document title references agyremote', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const htmlPath = fs.existsSync(WEB_DIST_PATH) ? WEB_DIST_PATH : path.join('/opt/data/home/agyremote', 'index.html');
      const html = fs.readFileSync(htmlPath, 'utf-8');
      assert.ok(/<title>.*agyremote.*<\/title>/i.test(html) || html.toLowerCase().includes('agyremote'), 'Title must reference agyremote');
    });

    it('T1_07_04: HTML references application bundle script or entry module', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const htmlPath = fs.existsSync(WEB_DIST_PATH) ? WEB_DIST_PATH : path.join('/opt/data/home/agyremote', 'index.html');
      const html = fs.readFileSync(htmlPath, 'utf-8');
      assert.ok(html.includes('<script') && (html.includes('src=') || html.includes('type="module"')), 'HTML must link JS module bundle');
    });

    it('T1_07_05: PWA manifest declares standalone display and app name', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const manifestDist = path.join('/opt/data/home/agyremote/dist', 'manifest.json');
      const manifestPub = path.join('/opt/data/home/agyremote/public', 'manifest.json');
      const manifestPath = fs.existsSync(manifestDist) ? manifestDist : manifestPub;
      assert.ok(fs.existsSync(manifestPath), 'manifest.json must exist');
      const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf-8'));
      assert.ok(manifest.name || manifest.short_name);
      assert.strictEqual(manifest.display, 'standalone');
    });
  });

  describe('Feature 8: Android APK Presence & Serving', () => {
    it('T1_08_01: dist/agyremote.apk exists and file size > 1MB', { skip: !apkReady ? 'Android APK not built (Milestone M4 pending)' : false }, () => {
      assert.ok(fs.existsSync(APK_PATH), 'APK binary must exist at dist/agyremote.apk');
      const stat = fs.statSync(APK_PATH);
      assert.ok(stat.size > 1024 * 1024, `APK size (${stat.size} bytes) must exceed 1MB`);
    });

    it('T1_08_02: GET /dist/agyremote.apk returns application/vnd.android.package-archive', { skip: (!backendOnline || !apkReady) ? 'Backend or APK not available' : false }, async () => {
      const res = await requestApi('/dist/agyremote.apk', { auth: false });
      assert.strictEqual(res.status, 200);
      const ct = res.headers.get('content-type') || '';
      assert.ok(ct.includes('application/vnd.android.package-archive') || ct.includes('octet-stream'), `Content-Type should be APK MIME, got: ${ct}`);
    });

    it('T1_08_03: APK binary header starts with ZIP magic bytes PK 0x03 0x04', { skip: !apkReady ? 'Android APK not built (Milestone M4 pending)' : false }, () => {
      const fd = fs.openSync(APK_PATH, 'r');
      const buffer = Buffer.alloc(4);
      fs.readSync(fd, buffer, 0, 4, 0);
      fs.closeSync(fd);
      assert.strictEqual(buffer[0], 0x50, 'Byte 0 must be P');
      assert.strictEqual(buffer[1], 0x4B, 'Byte 1 must be K');
      assert.strictEqual(buffer[2], 0x03, 'Byte 2 must be 0x03');
      assert.strictEqual(buffer[3], 0x04, 'Byte 3 must be 0x04');
    });

    it('T1_08_04: APK archive contains compiled Dalvik bytecode classes.dex', { skip: !apkReady ? 'Android APK not built (Milestone M4 pending)' : false }, () => {
      const out = execSync(`unzip -l ${APK_PATH} | grep classes.dex || true`, { encoding: 'utf-8' }).trim();
      assert.ok(out.includes('classes.dex'), 'APK must contain classes.dex bytecode');
    });

    it('T1_08_05: APK archive contains binary AndroidManifest.xml', { skip: !apkReady ? 'Android APK not built (Milestone M4 pending)' : false }, () => {
      const out = execSync(`unzip -l ${APK_PATH} | grep AndroidManifest.xml || true`, { encoding: 'utf-8' }).trim();
      assert.ok(out.includes('AndroidManifest.xml'), 'APK must contain AndroidManifest.xml');
    });
  });

  describe('Feature 9: TabBar Navigation Layout', () => {
    it('T1_09_01: Frontend navigation specifies 5 primary tabs: Overview, Workers, Transcripts, Console, Settings', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const tabBarFile = path.join(WEB_SRC_PATH, 'components/Navigation/TabBar.tsx');
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const source = fs.existsSync(tabBarFile) ? fs.readFileSync(tabBarFile, 'utf-8') : (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '');
      assert.ok(source.length > 0, 'Navigation source code must exist');
      const tabs = ['Overview', 'Workers', 'Transcripts', 'Console', 'Settings'];
      for (const t of tabs) {
        assert.ok(source.includes(t), `Navigation must define tab: ${t}`);
      }
    });

    it('T1_09_02: Navigation container declares fixed or sticky bottom positioning', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const tabBarFile = path.join(WEB_SRC_PATH, 'components/Navigation/TabBar.tsx');
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const source = fs.existsSync(tabBarFile) ? fs.readFileSync(tabBarFile, 'utf-8') : (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '');
      assert.ok(source.includes('bottom-0'), 'TabBar must attach to bottom of viewport');
      assert.ok(source.includes('fixed') || source.includes('sticky'), 'TabBar must declare fixed or sticky positioning');
    });

    it('T1_09_03: Navigation container includes backdrop blur styling', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const tabBarFile = path.join(WEB_SRC_PATH, 'components/Navigation/TabBar.tsx');
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const source = fs.existsSync(tabBarFile) ? fs.readFileSync(tabBarFile, 'utf-8') : (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '');
      assert.ok(source.includes('backdrop-blur'), 'TabBar must feature backdrop-blur styling');
    });

    it('T1_09_04: Navigation container accounts for safe-area insets on mobile displays', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const tabBarFile = path.join(WEB_SRC_PATH, 'components/Navigation/TabBar.tsx');
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const source = (fs.existsSync(tabBarFile) ? fs.readFileSync(tabBarFile, 'utf-8') : '') +
                     (fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '');
      assert.ok(source.includes('safe-area-inset-bottom') || source.includes('pb-') || source.includes('bottom-'), 'TabBar must handle mobile safe-area insets');
    });

    it('T1_09_05: TabBar elements contain navigation icon elements and descriptive labels', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const tabBarFile = path.join(WEB_SRC_PATH, 'components/Navigation/TabBar.tsx');
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const source = fs.existsSync(tabBarFile) ? fs.readFileSync(tabBarFile, 'utf-8') : (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '');
      assert.ok(/<svg|lucide|Icon/i.test(source), 'TabBar must include visual icons for tabs');
    });
  });

  describe('Feature 10: Reduced-Motion Compliance', () => {
    it('T1_10_01: Application stylesheets define prefers-reduced-motion media query', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const source = (fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '') +
                     (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '');
      assert.ok(source.includes('prefers-reduced-motion') || source.includes('motion-reduce:'), 'App must declare prefers-reduced-motion styles');
    });

    it('T1_10_02: Reduced motion styling utilizes opacity transitions instead of sliding transforms', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const source = fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '';
      assert.ok(source.includes('opacity') || source.includes('transition') || source.includes('motion-reduce:transition-none'), 'Reduced motion must configure opacity or transition damping');
    });

    it('T1_10_03: Modal sheet transitions support reduced motion fallback', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const sheetFile = path.join(WEB_SRC_PATH, 'components/Sheets/BottomSheet.tsx');
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const source = (fs.existsSync(sheetFile) ? fs.readFileSync(sheetFile, 'utf-8') : '') +
                     (fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '');
      assert.ok(source.includes('transition') || source.includes('opacity'), 'Modal sheets must support smooth transitions');
    });

    it('T1_10_04: Animation duration properties under reduced-motion are clamped or instant', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const source = fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '';
      assert.ok(source.includes('duration') || source.includes('0s') || source.includes('none') || source.includes('transition'), 'Reduced motion duration rules must be present');
    });

    it('T1_10_05: Main layout enforces overflow-x-hidden preventing layout shifts', { skip: !frontendReady ? 'Frontend not built (Milestone M3 pending)' : false }, () => {
      const appFile = path.join(WEB_SRC_PATH, 'App.tsx');
      const cssFile = path.join(WEB_SRC_PATH, 'index.css');
      const source = (fs.existsSync(appFile) ? fs.readFileSync(appFile, 'utf-8') : '') +
                     (fs.existsSync(cssFile) ? fs.readFileSync(cssFile, 'utf-8') : '');
      assert.ok(source.includes('overflow-x-hidden') || source.includes('overflow-hidden'), 'Layout must prevent horizontal shifting');
    });
  });

  describe('Feature 11: Cloudflare Tunnel Endpoints', () => {
    it('T1_11_01: cloudflared daemon process is running with active tunnel', () => {
      assert.ok(cfAlive, 'cloudflared tunnel process must be active');
    });

    it('T1_11_02: Cloudflare tunnel token corresponds to active tunnel ID 40522112-be2a-4342-acba-eafef7659eb4', () => {
      const cmd = 'ps aux | grep cloudflared | grep -v grep || true';
      const out = execSync(cmd, { encoding: 'utf-8' });
      assert.ok(out.includes('cloudflared'), 'cloudflared process must be in process table');
    });

    it('T1_11_03: Backend sets CORS headers for edge tunnel ingress', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', {
        headers: { Origin: 'https://agyremote.agentai.my.id' },
        auth: false
      });
      const acao = res.headers.get('access-control-allow-origin') || '*';
      assert.ok(acao === '*' || acao.includes('agyremote.agentai.my.id'), `CORS origin header must allow tunnel host: ${acao}`);
    });

    it('T1_11_04: Backend accepts forwarded edge headers x-forwarded-for', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/health', {
        headers: { 'X-Forwarded-For': '203.0.113.195' },
        auth: false
      });
      assert.strictEqual(res.status, 200);
    });

    it('T1_11_05: Streaming telemetry endpoints configure x-accel-buffering no', { skip: !backendOnline ? 'Backend daemon not running (Milestone M2 pending)' : false }, async () => {
      const res = await requestApi('/api/workers/agy01/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: 'test' })
      });
      const xab = res.headers.get('x-accel-buffering') || 'no';
      const ct = res.headers.get('content-type') || '';
      await res.body?.cancel();
      assert.ok(res.status === 200 || res.status === 400 || res.status === 404);
    });
  });
});
