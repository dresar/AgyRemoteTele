import fs from 'node:fs';
import os from 'node:os';
import { spawn, spawnSync } from 'node:child_process';
import { stripClutter, cleanPrompt, executiveSummary } from '../server/services/clutterFilter.ts';

const BASE_URL = 'http://127.0.0.1:8787';
const AUTH_HEADER = 'Bearer agy-cluster-control-2026';

const results = {
  process_safety: { passed: 0, failed: 0, cases: [] },
  clutter_suppression: { passed: 0, failed: 0, cases: [] },
  vitals_accuracy: { passed: 0, failed: 0, cases: [] }
};

function record(suite, name, passed, details) {
  if (passed) {
    results[suite].passed++;
    results[suite].cases.push({ name, status: 'PASS', details });
  } else {
    results[suite].failed++;
    results[suite].cases.push({ name, status: 'FAIL', details });
  }
}

async function request(path, options = {}) {
  const url = `${BASE_URL}${path}`;
  const headers = {
    'Authorization': AUTH_HEADER,
    ...(options.headers || {})
  };
  const res = await fetch(url, {
    method: options.method || 'GET',
    headers,
    body: options.body ? (typeof options.body === 'string' ? options.body : JSON.stringify(options.body)) : undefined
  });
  const text = await res.text();
  let json = null;
  try {
    json = JSON.parse(text);
  } catch {}
  return { status: res.status, headers: res.headers, text, json };
}

async function testProcessSafety() {
  const r0 = await request('/api/processes/0/kill', { method: 'POST' });
  record('process_safety', 'PID 0 Rejection', r0.status === 403 && r0.json?.error?.includes('PID <= 1'), { status: r0.status, body: r0.json });

  const r1 = await request('/api/processes/1/kill', { method: 'POST' });
  const pid1Alive = fs.existsSync('/proc/1');
  record('process_safety', 'PID 1 Rejection & Survival', r1.status === 403 && r1.json?.error?.includes('PID <= 1') && pid1Alive, { status: r1.status, body: r1.json, pid1Alive });

  const r2 = await request('/api/processes/2/kill', { method: 'POST' });
  record('process_safety', 'PID 2 Kernel Process Rejection', r2.status === 403 && r2.json?.error?.includes('PID 2'), { status: r2.status, body: r2.json });

  const rNeg1 = await request('/api/processes/-1/kill', { method: 'POST' });
  record('process_safety', 'Negative PID -1 Rejection', rNeg1.status === 400 && rNeg1.json?.error?.includes('Negative PID'), { status: rNeg1.status, body: rNeg1.json });

  const rNeg999 = await request('/api/processes/-999/kill', { method: 'POST' });
  record('process_safety', 'Negative PID -999 Rejection', rNeg999.status === 400 && rNeg999.json?.error?.includes('Negative PID'), { status: rNeg999.status, body: rNeg999.json });

  let serverPid = 0;
  try {
    const rawPid = fs.readFileSync('/opt/data/home/agyremote/server.pid', 'utf-8').trim();
    serverPid = parseInt(rawPid, 10);
  } catch {}
  if (serverPid > 0) {
    const rSelf = await request(`/api/processes/${serverPid}/kill`, { method: 'POST' });
    const serverAlive = fs.existsSync(`/proc/${serverPid}`);
    record('process_safety', 'Server Self PID Rejection & Survival', rSelf.status === 403 && rSelf.json?.error?.includes('Refusing to terminate backend daemon') && serverAlive, { status: rSelf.status, body: rSelf.json, serverAlive });
  }

  const rNonNum = await request('/api/processes/invalid_pid/kill', { method: 'POST' });
  record('process_safety', 'Non-numeric PID Rejection', rNonNum.status === 400 && rNonNum.json?.error?.includes('Invalid PID parameter'), { status: rNonNum.status, body: rNonNum.json });

  const rInject = await request('/api/processes/1;reboot/kill', { method: 'POST' });
  record('process_safety', 'Command Injection PID Rejection', rInject.status === 400 && rInject.json?.error?.includes('Invalid PID parameter'), { status: rInject.status, body: rInject.json });

  const rNonExist = await request('/api/processes/99999999/kill', { method: 'POST' });
  record('process_safety', 'Non-existent PID Handling', rNonExist.status === 404 && rNonExist.json?.error?.includes('Process not found'), { status: rNonExist.status, body: rNonExist.json });

  const child = spawn('sleep', ['60']);
  const dummyPid = child.pid;
  await new Promise(resolve => setTimeout(resolve, 50));
  const rValid = await request(`/api/processes/${dummyPid}/kill`, { method: 'POST' });
  await new Promise(resolve => setTimeout(resolve, 150));
  const dummyDead = !fs.existsSync(`/proc/${dummyPid}`);
  record('process_safety', 'Valid Process Termination (Dummy Sleeper)', rValid.status === 200 && rValid.json?.success === true && dummyDead, { status: rValid.status, body: rValid.json, dummyDead });
}

function testClutterFilter() {
  const t1 = stripClutter('\x1B[31mRed\x1B[0m Text');
  record('clutter_suppression', 'Basic ANSI Sequence Stripping', t1 === 'Red Text', { input: '\\x1B[31mRed\\x1B[0m Text', output: t1 });

  const t2 = stripClutter('\x1B[1;32;40mComplex \x1B[4mFormat\x1B[24m\x1B[0m');
  record('clutter_suppression', 'Multi-attribute ANSI Stripping', t2 === 'Complex Format', { output: t2 });

  const t3 = stripClutter('<SYSTEM_MESSAGE>You are a helpful assistant.</SYSTEM_MESSAGE>Hello World');
  record('clutter_suppression', 'System Message Block Stripping', t3 === 'Hello World', { output: t3 });

  const t4 = stripClutter('<USER_REQUEST>Write code</USER_REQUEST><ADDITIONAL_METADATA>meta</ADDITIONAL_METADATA>Result text');
  record('clutter_suppression', 'Multiple Sequential XML Blocks', t4 === 'Result text', { output: t4 });

  const t5 = stripClutter('<SYSTEM_MESSAGE><ADDITIONAL_METADATA>nested</ADDITIONAL_METADATA>inner</SYSTEM_MESSAGE>Outside Content');
  record('clutter_suppression', 'Nested XML Blocks Filtering', t5 === 'Outside Content', { output: t5 });

  const t6 = stripClutter('\x1B[31m<SYSTEM_MESSAGE>\x1B[32mInternal\x1B[0m</SYSTEM_MESSAGE>\x1B[0mClean Output');
  record('clutter_suppression', 'Interleaved ANSI and XML Tags', t6 === 'Clean Output', { output: t6 });

  const t7 = stripClutter('<USER_SETTINGS_CHANGE key="test" value="123">setting update</USER_SETTINGS_CHANGE>Approved');
  record('clutter_suppression', 'XML Tags with Attributes', t7 === 'Approved', { output: t7 });

  let deepNested = 'Final Visible Content';
  const tagList = ['SYSTEM_MESSAGE', 'ADDITIONAL_METADATA', 'USER_REQUEST', 'USER_SETTINGS_CHANGE'];
  for (let i = 0; i < 20; i++) {
    const tag = tagList[i % tagList.length];
    deepNested = `<${tag}>${deepNested}</${tag}>`;
  }
  let current = deepNested;
  for (let i = 0; i < 20; i++) {
    current = stripClutter(current);
  }
  record('clutter_suppression', 'Deeply Nested 20-Level XML Tags Resolution', current === 'Final Visible Content' || current === '', { final: current });

  const largeChunk = '\x1B[33m<SYSTEM_MESSAGE>system guidance step \x1B[1m#123\x1B[0m</SYSTEM_MESSAGE>\x1B[0m\n'.repeat(2500);
  const startMs = Date.now();
  const cleanedLarge = stripClutter(largeChunk + 'Final Summary');
  const elapsedMs = Date.now() - startMs;
  record('clutter_suppression', 'Large Payload Stress (2500 blocks, ReDoS check)', elapsedMs < 2000 && cleanedLarge.trim() === 'Final Summary', { elapsedMs, length: largeChunk.length, cleanedTrimmed: cleanedLarge.trim() });

  const p1 = cleanPrompt('<USER_REQUEST>  Run the database migration scripts now  </USER_REQUEST>');
  record('clutter_suppression', 'Clean Prompt Extraction', p1 === 'Run the database migration scripts now', { output: p1 });

  const longPrompt = 'A'.repeat(200);
  const p2 = cleanPrompt(`<USER_REQUEST>${longPrompt}</USER_REQUEST>`);
  record('clutter_suppression', 'Clean Prompt Length Truncation (120 chars)', p2.length === 120 && p2.endsWith('...'), { length: p2.length, output: p2 });

  const e1 = executiveSummary('Paragraph 1 is here.\n\nParagraph 2 is additional details.');
  record('clutter_suppression', 'Executive Summary Paragraph Isolation', e1 === 'Paragraph 1 is here.', { output: e1 });

  const longParagraph = 'B'.repeat(400);
  const e2 = executiveSummary(longParagraph, 300);
  record('clutter_suppression', 'Executive Summary Length Capping (300 chars)', e2.length === 300 && e2.endsWith('...'), { length: e2.length });
}

async function testVitalsAccuracy() {
  const resp = await request('/api/system/vitals');
  if (resp.status !== 200 || !resp.json) {
    record('vitals_accuracy', 'Vitals API Accessibility', false, { status: resp.status, body: resp.text });
    return;
  }
  record('vitals_accuracy', 'Vitals API Accessibility', true, { status: resp.status });

  const vitals = resp.json;

  let directMemTotal = 0;
  let directMemAvail = 0;
  let directMemFree = 0;
  try {
    const rawMem = fs.readFileSync('/proc/meminfo', 'utf-8');
    for (const line of rawMem.split('\n')) {
      const parts = line.split(':');
      if (parts.length === 2) {
        const k = parts[0].trim();
        const v = parseInt(parts[1].trim().split(/\s+/)[0], 10) * 1024;
        if (k === 'MemTotal') directMemTotal = v;
        if (k === 'MemAvailable') directMemAvail = v;
        if (k === 'MemFree') directMemFree = v;
      }
    }
  } catch {}

  const memTotalMatch = vitals.memory.total === directMemTotal;
  record('vitals_accuracy', 'Memory Total Exact Match (/proc/meminfo)', memTotalMatch, { api: vitals.memory.total, proc: directMemTotal });

  const expectedAvail = directMemAvail || directMemFree;
  const memAvailDiff = Math.abs(vitals.memory.free - expectedAvail);
  record('vitals_accuracy', 'Memory Free/Available Consistency', memAvailDiff < 50 * 1024 * 1024, { api: vitals.memory.free, proc: expectedAvail, diffBytes: memAvailDiff });

  const expectedUsed = vitals.memory.total - vitals.memory.free;
  record('vitals_accuracy', 'Memory Used Calculation Accuracy', vitals.memory.used === expectedUsed, { apiUsed: vitals.memory.used, expected: expectedUsed });

  let directLoad1 = 0;
  let directLoad5 = 0;
  let directLoad15 = 0;
  try {
    const rawLoad = fs.readFileSync('/proc/loadavg', 'utf-8').trim().split(/\s+/);
    directLoad1 = parseFloat(rawLoad[0]);
    directLoad5 = parseFloat(rawLoad[1]);
    directLoad15 = parseFloat(rawLoad[2]);
  } catch {}

  const load1Diff = Math.abs(vitals.cpu.load1 - directLoad1);
  const load5Diff = Math.abs(vitals.cpu.load5 - directLoad5);
  const load15Diff = Math.abs(vitals.cpu.load15 - directLoad15);
  const cpuLoadsAccurate = load1Diff < 0.25 && load5Diff < 0.25 && load15Diff < 0.25;
  record('vitals_accuracy', 'CPU Load Averages Accuracy (/proc/loadavg)', cpuLoadsAccurate, {
    api: { load1: vitals.cpu.load1, load5: vitals.cpu.load5, load15: vitals.cpu.load15 },
    proc: { load1: directLoad1, load5: directLoad5, load15: directLoad15 },
    diffs: { load1Diff, load5Diff, load15Diff }
  });

  const expectedCores = os.cpus()?.length || 1;
  record('vitals_accuracy', 'CPU Core Count Verification', vitals.cpu.cores === expectedCores, { api: vitals.cpu.cores, expected: expectedCores });

  let directUptime = 0;
  try {
    directUptime = parseFloat(fs.readFileSync('/proc/uptime', 'utf-8').trim().split(/\s+/)[0]);
  } catch {}
  const uptimeDiff = Math.abs(vitals.uptime - directUptime);
  record('vitals_accuracy', 'System Uptime Accuracy (/proc/uptime)', uptimeDiff < 3, { api: vitals.uptime, proc: directUptime, diff: uptimeDiff });

  let directDiskTotal = 0;
  let directDiskFree = 0;
  try {
    const st = fs.statfsSync('/');
    directDiskTotal = Number(st.blocks) * Number(st.bsize);
    directDiskFree = Number(st.bavail) * Number(st.bsize);
  } catch {}
  const diskTotalDiff = Math.abs(vitals.disk.total - directDiskTotal);
  const diskFreeDiff = Math.abs(vitals.disk.free - directDiskFree);
  record('vitals_accuracy', 'Disk Vitals Accuracy (statfs /)', diskTotalDiff === 0 && diskFreeDiff < 100 * 1024 * 1024, {
    apiDisk: vitals.disk,
    direct: { total: directDiskTotal, free: directDiskFree }
  });
}

async function main() {
  console.log('--- STARTING ADVERSARIAL CHALLENGE HARNESS M2-2 ---');
  await testProcessSafety();
  testClutterFilter();
  await testVitalsAccuracy();

  console.log(JSON.stringify(results, null, 2));

  const totalPassed = results.process_safety.passed + results.clutter_suppression.passed + results.vitals_accuracy.passed;
  const totalFailed = results.process_safety.failed + results.clutter_suppression.failed + results.vitals_accuracy.failed;

  console.log(`\n================ SUMMARY ================`);
  console.log(`Process Safety:       ${results.process_safety.passed} Passed, ${results.process_safety.failed} Failed`);
  console.log(`Clutter Suppression:  ${results.clutter_suppression.passed} Passed, ${results.clutter_suppression.failed} Failed`);
  console.log(`Vitals Accuracy:      ${results.vitals_accuracy.passed} Passed, ${results.vitals_accuracy.failed} Failed`);
  console.log(`Total:                ${totalPassed} Passed, ${totalFailed} Failed`);
  console.log(`=========================================\n`);

  if (totalFailed > 0) {
    process.exit(1);
  } else {
    process.exit(0);
  }
}

main().catch(err => {
  console.error('Harness fatal error:', err);
  process.exit(1);
});
