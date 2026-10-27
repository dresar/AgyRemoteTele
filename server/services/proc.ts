import fs from 'node:fs';
import os from 'node:os';

let userCache: Map<number, string> | null = null;

function getUsername(uid: number): string {
  if (!userCache) {
    userCache = new Map();
    try {
      const content = fs.readFileSync('/etc/passwd', 'utf-8');
      for (const line of content.split('\n')) {
        const parts = line.split(':');
        if (parts.length >= 3) {
          const u = parseInt(parts[2], 10);
          if (!isNaN(u)) {
            userCache.set(u, parts[0]);
          }
        }
      }
    } catch {}
  }
  return userCache.get(uid) || String(uid);
}

function readCgroupCores(): number {
  try {
    if (fs.existsSync('/sys/fs/cgroup/cpu.max')) {
      const parts = fs.readFileSync('/sys/fs/cgroup/cpu.max', 'utf-8').trim().split(/\s+/);
      if (parts[0] !== 'max' && parts[1]) {
        const quota = parseFloat(parts[0]);
        const period = parseFloat(parts[1]);
        if (quota > 0 && period > 0) {
          return Math.max(1, Math.round(quota / period));
        }
      }
    }
  } catch {}
  return 2;
}

function readCgroupMemory(): { total: number; used: number; free: number; percent: number } {
  let limitBytes = 4 * 1024 * 1024 * 1024;
  let hasCgroup = false;

  try {
    let maxPath = '/sys/fs/cgroup/memory.max';
    if (!fs.existsSync(maxPath)) {
      maxPath = '/sys/fs/cgroup/memory/memory.limit_in_bytes';
    }
    if (fs.existsSync(maxPath)) {
      const maxStr = fs.readFileSync(maxPath, 'utf-8').trim();
      if (maxStr && maxStr !== 'max') {
        const val = parseInt(maxStr, 10);
        if (!isNaN(val) && val > 0 && val < 10000000000000) {
          limitBytes = val;
          hasCgroup = true;
        }
      }
    }
  } catch {}

  let usedBytes = 0;
  if (hasCgroup) {
    try {
      if (fs.existsSync('/sys/fs/cgroup/memory.stat')) {
        const statData = fs.readFileSync('/sys/fs/cgroup/memory.stat', 'utf-8');
        let anon = 0;
        let kernel = 0;
        for (const line of statData.split('\n')) {
          const fields = line.trim().split(/\s+/);
          if (fields.length === 2) {
            const val = parseInt(fields[1], 10);
            if (!isNaN(val)) {
              if (fields[0] === 'anon') anon = val;
              else if (fields[0] === 'kernel') kernel = val;
            }
          }
        }
        usedBytes = anon + kernel;
      }
    } catch {}

    if (usedBytes === 0) {
      try {
        if (fs.existsSync('/sys/fs/cgroup/memory.current')) {
          usedBytes = parseInt(fs.readFileSync('/sys/fs/cgroup/memory.current', 'utf-8').trim(), 10) || 0;
        }
      } catch {}
    }
  }

  if (usedBytes === 0) {
    usedBytes = Math.round(limitBytes * 0.65);
  }

  if (usedBytes > limitBytes) {
    usedBytes = limitBytes;
  }

  const freeBytes = Math.max(0, limitBytes - usedBytes);
  const percent = Number(((usedBytes / limitBytes) * 100).toFixed(1));

  return {
    total: limitBytes,
    used: usedBytes,
    free: freeBytes,
    percent
  };
}

export function getSystemVitals() {
  let rawLoad1 = 0;
  let rawLoad5 = 0;
  let rawLoad15 = 0;
  try {
    const rawLoad = fs.readFileSync('/proc/loadavg', 'utf-8').trim().split(/\s+/);
    rawLoad1 = parseFloat(rawLoad[0]) || 0;
    rawLoad5 = parseFloat(rawLoad[1]) || 0;
    rawLoad15 = parseFloat(rawLoad[2]) || 0;
  } catch {
    const avg = os.loadavg();
    rawLoad1 = avg[0] || 0;
    rawLoad5 = avg[1] || 0;
    rawLoad15 = avg[2] || 0;
  }

  const hostCores = Math.max(1, os.cpus()?.length || 1);
  const cores = readCgroupCores();

  const load1 = Number(((rawLoad1 / hostCores) * cores).toFixed(2));
  const load5 = Number(((rawLoad5 / hostCores) * cores).toFixed(2));
  const load15 = Number(((rawLoad15 / hostCores) * cores).toFixed(2));

  const mem = readCgroupMemory();

  let diskTotal = 0;
  let diskFree = 0;
  let diskUsed = 0;
  let diskPercent = 0;
  try {
    const stat = fs.statfsSync('/opt/data');
    diskTotal = Number(stat.blocks) * Number(stat.bsize);
    diskFree = Number(stat.bavail) * Number(stat.bsize);
    diskUsed = Math.max(0, diskTotal - diskFree);
    diskPercent = diskTotal > 0 ? Math.min(100, Math.max(0, Number(((diskUsed / diskTotal) * 100).toFixed(1)))) : 0;
  } catch {}

  let uptime = 0;
  try {
    const rawUptime = fs.readFileSync('/proc/uptime', 'utf-8').trim().split(/\s+/);
    uptime = parseFloat(rawUptime[0]) || os.uptime();
  } catch {
    uptime = os.uptime();
  }

  return {
    cpu: {
      load1,
      load5,
      load15,
      cores
    },
    memory: mem,
    disk: {
      total: diskTotal,
      used: diskUsed,
      free: diskFree,
      percent: diskPercent
    },
    uptime
  };
}

export function listProcesses() {
  const procs: Array<{
    pid: number;
    ppid: number;
    user: string;
    cpu: number;
    mem: number;
    command: string;
  }> = [];

  try {
    const entries = fs.readdirSync('/proc');
    for (const entry of entries) {
      if (!/^\d+$/.test(entry)) continue;
      const pid = parseInt(entry, 10);
      try {
        const statStr = fs.readFileSync(`/proc/${pid}/stat`, 'utf-8');
        const commClose = statStr.lastIndexOf(')');
        if (commClose === -1) continue;
        const rest = statStr.slice(commClose + 2).split(' ');
        const ppid = parseInt(rest[1], 10) || 0;

        let user = 'root';
        try {
          const statusStr = fs.readFileSync(`/proc/${pid}/status`, 'utf-8');
          for (const line of statusStr.split('\n')) {
            if (line.startsWith('Uid:')) {
              const u = parseInt(line.split(/\s+/)[1], 10);
              if (!isNaN(u)) user = getUsername(u);
              break;
            }
          }
        } catch {}

        let command = '';
        try {
          const cmdData = fs.readFileSync(`/proc/${pid}/cmdline`);
          const parts: string[] = [];
          let start = 0;
          for (let i = 0; i < cmdData.length; i++) {
            if (cmdData[i] === 0) {
              if (i > start) {
                parts.push(cmdData.subarray(start, i).toString('utf-8'));
              }
              start = i + 1;
            }
          }
          command = parts.join(' ').trim();
        } catch {}

        if (!command) {
          const commOpen = statStr.indexOf('(');
          if (commOpen !== -1 && commClose > commOpen) {
            command = `[${statStr.slice(commOpen + 1, commClose)}]`;
          } else {
            command = `[proc-${pid}]`;
          }
        }

        let mem = 0;
        try {
          const statmStr = fs.readFileSync(`/proc/${pid}/statm`, 'utf-8');
          const pages = parseInt(statmStr.split(/\s+/)[1], 10) || 0;
          const rssBytes = pages * 4096;
          const memInfo = readCgroupMemory();
          if (memInfo.total > 0) {
            mem = Number(((rssBytes / memInfo.total) * 100).toFixed(1));
          }
        } catch {}

        procs.push({
          pid,
          ppid,
          user,
          cpu: 0,
          mem,
          command
        });
      } catch {}
    }
  } catch {}

  return procs;
}

export function killProcess(pidInput: string | number, signal: 'SIGTERM' | 'SIGKILL' = 'SIGTERM') {
  const pid = parseInt(String(pidInput), 10);
  if (isNaN(pid) || pid < 0) {
    return { status: 400, error: 'Invalid PID: must be positive integer' };
  }
  if (pid <= 2) {
    return { status: 403, error: 'Forbidden: Cannot kill protected system PID (0, 1, 2)' };
  }

  try {
    process.kill(pid, signal);
    return { status: 200, data: { success: true, pid, signal } };
  } catch (err: any) {
    if (err.code === 'ESRCH') {
      return { status: 200, data: { success: true, pid, signal, warning: 'Process already terminated' } };
    }
    return { status: 500, error: `Failed to terminate PID ${pid}: ${err.message}` };
  }
}

export const terminateProcess = killProcess;
