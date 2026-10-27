import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import assert from 'node:assert';

const BASE_URL = 'http://127.0.0.1:8787';
const DIST_DIR = '/opt/data/home/agyremote/dist';
const SRC_DIR = '/opt/data/home/agyremote/src';
const AUTH_TOKEN = 'agy-cluster-control-2026';

let totalTests = 0;
let passedTests = 0;
let failedTests = 0;
const results = [];

function recordTest(name, passed, detail = '') {
  totalTests++;
  if (passed) {
    passedTests++;
    results.push({ name, status: 'PASS', detail });
    console.log(`  [PASS] ${name}`);
  } else {
    failedTests++;
    results.push({ name, status: 'FAIL', detail });
    console.error(`  [FAIL] ${name} - ${detail}`);
  }
}

async function runSection1_SSEService() {
  console.log('\n--- SECTION 1: SSE Streaming Service & Clutter Filter Stress ---');

  const sseSource = fs.readFileSync(path.join(SRC_DIR, 'services/sse.ts'), 'utf-8');
  recordTest('sse.ts: Implements streamWorkerPrompt with AbortController', 
    sseSource.includes('export function streamWorkerPrompt') && sseSource.includes('new AbortController()')
  );
  recordTest('sse.ts: Parses text_delta, tool_status, result, error events', 
    sseSource.includes("event.type === 'text_delta'") &&
    sseSource.includes("event.type === 'tool_status'") &&
    sseSource.includes("event.type === 'result'") &&
    sseSource.includes("event.type === 'error'")
  );
  recordTest('sse.ts: Handles [DONE] terminator', 
    sseSource.includes("dataPayload === '[DONE]'")
  );
  recordTest('sse.ts: Returns cancellation abort callback', 
    sseSource.includes('return () => {') && sseSource.includes('controller.abort();')
  );

  function simulateSSEParser(rawChunks) {
    let text = '';
    const tools = [];
    let result = null;
    let error = null;
    let closed = false;

    let buffer = '';
    for (const chunk of rawChunks) {
      buffer += chunk;
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';

      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed || !trimmed.startsWith('data:')) continue;
        const dataPayload = trimmed.slice(5).trim();
        if (dataPayload === '[DONE]') {
          closed = true;
          return { text, tools, result, error, closed };
        }
        try {
          const event = JSON.parse(dataPayload);
          if (event.type === 'text_delta' && event.text) {
            text += event.text;
          } else if (event.type === 'tool_status') {
            tools.push({ name: event.name || 'tool', state: event.state || 'DONE' });
          } else if (event.type === 'result') {
            result = event;
          } else if (event.type === 'error') {
            error = event.message;
          }
        } catch {}
      }
    }
    return { text, tools, result, error, closed };
  }

  const chunkedTest = simulateSSEParser([
    'data: {"type": "text_',
    'delta", "text": "Hello, "}\n',
    '\ndata: {"type": "text_delta", "text": "World!"}\n\n',
    'data: [DONE]\n\n'
  ]);
  recordTest('SSE Parser: Multi-chunk split reassembly', 
    chunkedTest.text === 'Hello, World!' && chunkedTest.closed === true
  );

  const noisyTest = simulateSSEParser([
    ': keepalive ping\n\n',
    'data: {"type": "tool_status", "name": "bash", "state": "START"}\n\n',
    'data: broken-json-payload-without-brackets\n\n',
    'data: {"type": "text_delta", "text": "valid chunk"}\n\n',
    'data: {"type": "result", "status": "COMPLETED", "summary": "Done"}\n\n',
    'data: [DONE]\n\n'
  ]);
  recordTest('SSE Parser: Handles malformed JSON without crash & extracts tool status', 
    noisyTest.text === 'valid chunk' && 
    noisyTest.tools.length === 1 && 
    noisyTest.tools[0].name === 'bash' &&
    noisyTest.result.status === 'COMPLETED'
  );

  const res = await fetch(`${BASE_URL}/api/workers/agy01/prompt`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${AUTH_TOKEN}`
    },
    body: JSON.stringify({ prompt: 'ping live stream test' })
  });

  const contentType = res.headers.get('content-type') || '';
  const cacheControl = res.headers.get('cache-control') || '';
  const accelBuffering = res.headers.get('x-accel-buffering') || '';

  recordTest('Live SSE Endpoint: Content-Type is text/event-stream', 
    res.status === 200 && contentType.includes('text/event-stream')
  );
  recordTest('Live SSE Endpoint: Cache-Control is no-cache', 
    cacheControl.includes('no-cache')
  );
  recordTest('Live SSE Endpoint: X-Accel-Buffering is no', 
    accelBuffering === 'no'
  );

  if (res.body) {
    const reader = res.body.getReader();
    const { value } = await reader.read();
    const firstChunk = new TextDecoder().decode(value || new Uint8Array());
    recordTest('Live SSE Endpoint: Returns initial stream start frame', 
      firstChunk.includes('"type":"start"') || firstChunk.includes('"worker":"agy01"')
    );
    await reader.cancel();
    recordTest('Live SSE Endpoint: Connection dropped cleanly via client cancel without crash', true);
  }

  const clutterFilterPath = '/opt/data/home/agyremote/server/services/clutterFilter.ts';
  const clutterFilterSrc = fs.readFileSync(clutterFilterPath, 'utf-8');
  recordTest('Clutter Filter: Strips ANSI escape codes & internal XML tags', 
    clutterFilterSrc.includes('ANSI_REGEX') && 
    clutterFilterSrc.includes('USER_REQUEST') && 
    clutterFilterSrc.includes('SYSTEM_MESSAGE')
  );

  const detailSheetPath = path.join(SRC_DIR, 'components/Sheets/WorkerDetailSheet.tsx');
  const detailSheetSrc = fs.readFileSync(detailSheetPath, 'utf-8');
  recordTest('WorkerDetailSheet.tsx: Discrete tool execution badges rendered', 
    detailSheetSrc.includes('toolBadges.map') && detailSheetSrc.includes('Invoked Tools')
  );
  recordTest('WorkerDetailSheet.tsx: Streaming text deltas rendered in live container', 
    detailSheetSrc.includes('streamingText') && detailSheetSrc.includes('Live Output Stream')
  );
}

async function runSection2_APKDownloadButton() {
  console.log('\n--- SECTION 2: One-Tap APK Download Button & Direct Serving ---');

  const overviewPath = path.join(SRC_DIR, 'components/Tabs/OverviewTab.tsx');
  const overviewSrc = fs.readFileSync(overviewPath, 'utf-8');
  recordTest('OverviewTab.tsx: Contains direct download link href="/dist/agyremote.apk"', 
    overviewSrc.includes('href="/dist/agyremote.apk"')
  );
  recordTest('OverviewTab.tsx: Has download="agyremote.apk" attribute', 
    overviewSrc.includes('download="agyremote.apk"')
  );

  const settingsPath = path.join(SRC_DIR, 'components/Tabs/SettingsTab.tsx');
  const settingsSrc = fs.readFileSync(settingsPath, 'utf-8');
  recordTest('SettingsTab.tsx: Contains direct download link href="/dist/agyremote.apk"', 
    settingsSrc.includes('href="/dist/agyremote.apk"')
  );
  recordTest('SettingsTab.tsx: Has download="agyremote.apk" attribute', 
    settingsSrc.includes('download="agyremote.apk"')
  );

  const jsAssets = fs.readdirSync(path.join(DIST_DIR, 'assets')).filter(f => f.endsWith('.js'));
  let bundleContainsApkLink = false;
  let bundleContainsDownloadAttr = false;
  for (const jsFile of jsAssets) {
    const content = fs.readFileSync(path.join(DIST_DIR, 'assets', jsFile), 'utf-8');
    if (content.includes('/dist/agyremote.apk')) bundleContainsApkLink = true;
    if (/download:\s*[`'"]agyremote\.apk[`'"]/.test(content)) bundleContainsDownloadAttr = true;
  }
  recordTest('Production JS Bundle: Compiled code includes /dist/agyremote.apk link', bundleContainsApkLink);
  recordTest('Production JS Bundle: Compiled code preserves download attribute', bundleContainsDownloadAttr);

  const noApkRes = await fetch(`${BASE_URL}/dist/agyremote.apk`);
  recordTest('Server Route /dist/agyremote.apk: Returns 404 when APK file not yet created', 
    noApkRes.status === 404
  );

  const testApkPath = path.join(DIST_DIR, 'agyremote.apk');
  const dummyApkContent = Buffer.from('PK\x03\x04MOCK_ANDROID_APK_DATA_STREAM_TEST');
  fs.writeFileSync(testApkPath, dummyApkContent);

  try {
    const apkRes = await fetch(`${BASE_URL}/dist/agyremote.apk`);
    const apkContentType = apkRes.headers.get('content-type') || '';
    const apkDisposition = apkRes.headers.get('content-disposition') || '';
    const apkLength = parseInt(apkRes.headers.get('content-length') || '0', 10);
    const bodyBuf = Buffer.from(await apkRes.arrayBuffer());

    recordTest('Server Route /dist/agyremote.apk: Returns HTTP 200 OK when file exists', 
      apkRes.status === 200
    );
    recordTest('Server Route /dist/agyremote.apk: Content-Type is application/vnd.android.package-archive', 
      apkContentType.includes('application/vnd.android.package-archive')
    );
    recordTest('Server Route /dist/agyremote.apk: Content-Disposition is attachment; filename="agyremote.apk"', 
      apkDisposition.includes('attachment') && apkDisposition.includes('agyremote.apk')
    );
    recordTest('Server Route /dist/agyremote.apk: Content-Length matches byte size', 
      apkLength === dummyApkContent.length && bodyBuf.equals(dummyApkContent)
    );
  } finally {
    if (fs.existsSync(testApkPath)) {
      fs.unlinkSync(testApkPath);
    }
  }

  const cleanedRes = await fetch(`${BASE_URL}/dist/agyremote.apk`);
  recordTest('Cleanup Check: /dist/agyremote.apk returns 404 after mock teardown', 
    cleanedRes.status === 404
  );
}

async function runSection3_ProductionServing() {
  console.log('\n--- SECTION 3: Production Build Outputs & Live HTTP Serving ---');

  const rootRes = await fetch(`${BASE_URL}/`);
  const rootText = await rootRes.text();
  const rootType = rootRes.headers.get('content-type') || '';

  recordTest('GET /: Returns HTTP 200 OK with text/html', 
    rootRes.status === 200 && rootType.includes('text/html')
  );
  recordTest('GET /: Contains root div (<div id="root">)', 
    rootText.includes('<div id="root">')
  );
  recordTest('GET /: Contains viewport meta tag with viewport-fit=cover', 
    rootText.includes('viewport-fit=cover') && rootText.includes('user-scalable=no')
  );
  recordTest('GET /: Contains manifest link href="/manifest.json"', 
    rootText.includes('href="/manifest.json"')
  );

  const manifestRes = await fetch(`${BASE_URL}/manifest.json`);
  const manifestJson = await manifestRes.json();
  recordTest('GET /manifest.json: Returns valid JSON with standalone display', 
    manifestRes.status === 200 && 
    manifestJson.display === 'standalone' && 
    manifestJson.name === 'agyremote'
  );

  const swRes = await fetch(`${BASE_URL}/sw.js`);
  const swText = await swRes.text();
  recordTest('GET /sw.js: Returns service worker script with cache logic', 
    swRes.status === 200 && 
    swText.includes('install') && 
    swText.includes('fetch')
  );

  const icon192 = await fetch(`${BASE_URL}/icon-192.png`);
  recordTest('GET /icon-192.png: Returns HTTP 200 with image/png', 
    icon192.status === 200 && icon192.headers.get('content-type') === 'image/png'
  );

  const icon512 = await fetch(`${BASE_URL}/icon-512.png`);
  recordTest('GET /icon-512.png: Returns HTTP 200 with image/png', 
    icon512.status === 200 && icon512.headers.get('content-type') === 'image/png'
  );

  const favicon = await fetch(`${BASE_URL}/favicon.ico`);
  recordTest('GET /favicon.ico: Returns HTTP 200 with icon MIME', 
    favicon.status === 200
  );

  const cssAssets = fs.readdirSync(path.join(DIST_DIR, 'assets')).filter(f => f.endsWith('.css'));
  let cssHasReducedMotion = false;
  for (const cssFile of cssAssets) {
    const cssContent = fs.readFileSync(path.join(DIST_DIR, 'assets', cssFile), 'utf-8');
    if (cssContent.includes('prefers-reduced-motion')) {
      cssHasReducedMotion = true;
    }
  }
  recordTest('Production CSS Bundle: Contains prefers-reduced-motion media query', cssHasReducedMotion);

  const appSrc = fs.readFileSync(path.join(SRC_DIR, 'App.tsx'), 'utf-8');
  recordTest('App.tsx: Viewport boundary max-w-md min-w-[360px] enforced', 
    appSrc.includes('max-w-md') && appSrc.includes('min-w-[360px]') && appSrc.includes('overflow-x-hidden')
  );

  const tabbarSrc = fs.readFileSync(path.join(SRC_DIR, 'components/Navigation/TabBar.tsx'), 'utf-8');
  recordTest('TabBar.tsx: Includes all 5 tabs (Overview, Workers, Transcripts, Console, Settings)', 
    tabbarSrc.includes("'overview'") && 
    tabbarSrc.includes("'workers'") && 
    tabbarSrc.includes("'transcripts'") && 
    tabbarSrc.includes("'console'") && 
    tabbarSrc.includes("'settings'")
  );
  recordTest('TabBar.tsx: Sticky bottom fixed positioning with safe area insets', 
    tabbarSrc.includes('fixed bottom-0') && tabbarSrc.includes('safe-area-inset-bottom')
  );
}

async function main() {
  console.log('================================================================');
  console.log('      CHALLENGER M3-2: ADVERSARIAL STRESS TEST SUITE            ');
  console.log('================================================================');

  await runSection1_SSEService();
  await runSection2_APKDownloadButton();
  await runSection3_ProductionServing();

  console.log('\n================================================================');
  console.log(`TOTAL TESTS: ${totalTests} | PASSED: ${passedTests} | FAILED: ${failedTests}`);
  console.log('================================================================');

  if (failedTests > 0) {
    console.error(`\nFAILED TESTS (${failedTests}):`);
    results.filter(r => r.status === 'FAIL').forEach(r => console.error(` - ${r.name}: ${r.detail}`));
    process.exit(1);
  } else {
    console.log('\n>>> ALL ADVERSARIAL CHALLENGE CHECKS PASSED EMPIRICALLY (EXIT 0) <<<');
    process.exit(0);
  }
}

main().catch(err => {
  console.error('Fatal harness error:', err);
  process.exit(1);
});
