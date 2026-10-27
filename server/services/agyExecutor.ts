import fs from 'node:fs';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { stripClutter, cleanPrompt, executiveSummary } from './clutterFilter.ts';

const PROFILES_DIR = process.env.PROFILES_ROOT || '/opt/data/home/agy-profiles';
const AGY_BIN = process.env.AGY_BIN_PATH || '/opt/data/home/.local/bin/agy';
const WORKER_ID_REGEX = /^agy(0[1-9]|1[0-5])$/;

export interface WorkerProfile {
  id: string;
  name: string;
  email: string;
  authenticated: boolean;
  activeSession: string | null;
  status: 'idle' | 'running' | 'unauthenticated';
  hostname: string;
}

const activeWorkerSessions = new Map<string, string>();

export function isValidWorkerId(id: string): boolean {
  return WORKER_ID_REGEX.test(id);
}

export function getProfileHome(id: string): string {
  const primary = path.join(PROFILES_DIR, id);
  if (fs.existsSync(primary)) {
    return primary;
  }
  const num = parseInt(id.replace('agy', ''), 10);
  const fallback = path.join(PROFILES_DIR, `account${num}`);
  if (fs.existsSync(fallback)) {
    return fallback;
  }
  return primary;
}

export function listWorkers(): WorkerProfile[] {
  const workers: WorkerProfile[] = [];

  for (let i = 1; i <= 15; i++) {
    const id = `agy${String(i).padStart(2, '0')}`;
    const pdir = getProfileHome(id);
    const cfgPath = path.join(pdir, '.gemini/config/config.json');
    const tokPath = path.join(pdir, '.gemini/antigravity-cli/antigravity-oauth-token');

    let hostname = `eka-${i}`;
    let name = `Worker ${id.toUpperCase()}`;
    let email = '-';
    let authenticated = false;

    if (fs.existsSync(cfgPath)) {
      try {
        const cfg = JSON.parse(fs.readFileSync(cfgPath, 'utf-8'));
        if (cfg?.userSettings?.cliRemoteControlHostname) {
          hostname = cfg.userSettings.cliRemoteControlHostname;
          name = hostname;
        }
      } catch {}
    }

    if (fs.existsSync(tokPath)) {
      try {
        const tok = JSON.parse(fs.readFileSync(tokPath, 'utf-8'));
        email = tok.id_token_email || '';
        let exp = 0;
        if (!email && tok.id_token) {
          const parts = tok.id_token.split('.');
          if (parts.length >= 2) {
            const payload = parts[1];
            const pad = payload.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (payload.length % 4)) % 4);
            const parsed = JSON.parse(Buffer.from(pad, 'base64').toString('utf-8'));
            email = parsed.email || '';
            exp = parsed.exp || 0;
          }
        } else if (tok.token?.expiry) {
          exp = Math.floor(new Date(tok.token.expiry).getTime() / 1000);
        }
        if (email && email !== 'unknown') {
          authenticated = exp === 0 || exp > Math.floor(Date.now() / 1000) - 3600;
        }
      } catch {}
    }

    const activeSession = activeWorkerSessions.get(id) || null;
    let status: 'idle' | 'running' | 'unauthenticated' = 'unauthenticated';
    if (authenticated) {
      status = activeSession ? 'running' : 'idle';
    }

    workers.push({
      id,
      name,
      email: email || '-',
      authenticated,
      activeSession,
      status,
      hostname
    });
  }

  return workers;
}

export function getWorker(id: string): WorkerProfile | null {
  if (!isValidWorkerId(id)) return null;
  const list = listWorkers();
  return list.find(w => w.id === id) || null;
}

export function getWorkerTranscripts(id: string, limit = 10) {
  if (!isValidWorkerId(id)) return null;

  const pdir = getProfileHome(id);
  const brainDir = path.join(pdir, '.gemini/antigravity-cli/brain');
  if (!fs.existsSync(brainDir)) {
    return { id, transcripts: [] };
  }

  try {
    const entries = fs.readdirSync(brainDir);
    const candidates: Array<{ id: string; logPath: string; mtime: number }> = [];

    for (const convId of entries) {
      const logPath = path.join(brainDir, convId, '.system_generated/logs/transcript.jsonl');
      try {
        const st = fs.statSync(logPath);
        candidates.push({ id: convId, logPath, mtime: st.mtimeMs });
      } catch {}
    }

    candidates.sort((a, b) => b.mtime - a.mtime);
    const selected = candidates.slice(0, limit);

    const transcripts = selected.map(c => {
      let firstPrompt = '';
      let lastResponse = '';
      let turns = 0;
      let startedAt = new Date(c.mtime).toISOString();

      try {
        const lines = fs.readFileSync(c.logPath, 'utf-8').split('\n');
        for (const line of lines) {
          if (!line.trim()) continue;
          try {
            const item = JSON.parse(line);
            if (!firstPrompt && item.created_at) {
              startedAt = item.created_at;
            }
            if (item.type === 'USER_INPUT') {
              turns++;
              if (!firstPrompt) {
                firstPrompt = cleanPrompt(item.content || '');
              }
            }
            if (item.type === 'PLANNER_RESPONSE' && item.content) {
              lastResponse = executiveSummary(item.content, 200);
            }
          } catch {}
        }
      } catch {}

      return {
        conversation_id: c.id,
        started_at: startedAt,
        last_updated_at: new Date(c.mtime).toISOString(),
        turn_count: turns,
        first_prompt: firstPrompt || '(No prompt recorded)',
        last_response: lastResponse || '(No response recorded)'
      };
    });

    return { id, transcripts };
  } catch {
    return { id, transcripts: [] };
  }
}

export interface FileChangeItem {
  path: string;
  name: string;
  action: 'modified' | 'created' | 'deleted';
  timestamp?: string;
}

export function getWorkerFileChanges(id: string): {
  worker: string;
  conversation_id: string;
  changes: FileChangeItem[];
  summary: {
    modified: number;
    created: number;
    deleted: number;
    total: number;
  };
} {
  if (!isValidWorkerId(id)) {
    return {
      worker: id,
      conversation_id: '',
      changes: [],
      summary: { modified: 0, created: 0, deleted: 0, total: 0 }
    };
  }

  const pdir = getProfileHome(id);
  const brainDir = path.join(pdir, '.gemini/antigravity-cli/brain');
  if (!fs.existsSync(brainDir)) {
    return {
      worker: id,
      conversation_id: '',
      changes: [],
      summary: { modified: 0, created: 0, deleted: 0, total: 0 }
    };
  }

  try {
    const entries = fs.readdirSync(brainDir);
    const candidates: Array<{ id: string; logPath: string; mtime: number }> = [];

    for (const convId of entries) {
      const logPath = path.join(brainDir, convId, '.system_generated/logs/transcript.jsonl');
      try {
        const st = fs.statSync(logPath);
        candidates.push({ id: convId, logPath, mtime: st.mtimeMs });
      } catch {}
    }

    candidates.sort((a, b) => b.mtime - a.mtime);
    if (candidates.length === 0) {
      return {
        worker: id,
        conversation_id: '',
        changes: [],
        summary: { modified: 0, created: 0, deleted: 0, total: 0 }
      };
    }

    const latest = candidates[0];
    const changesMap = new Map<string, FileChangeItem>();

    try {
      const lines = fs.readFileSync(latest.logPath, 'utf-8').split('\n');
      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          const item = JSON.parse(line);
          const toolCalls = item.tool_calls || [];
          for (const tc of toolCalls) {
            const name = tc.name;
            const args = tc.args || {};
            let filePath = args.Path || args.path || '';
            if (typeof filePath === 'string') {
              filePath = filePath.replace(/^"|"$/g, '').trim();
            }

            if (!filePath) {
              if (name === 'run_command' && args.CommandLine) {
                const cmd = args.CommandLine;
                const m = cmd.match(/\brm\s+(?:-[a-zA-Z]+\s+)?([^\s&|;]+)/);
                if (m && m[1]) {
                  filePath = m[1].replace(/^"|"$/g, '').trim();
                  const base = path.basename(filePath);
                  changesMap.set(filePath, {
                    path: filePath,
                    name: base,
                    action: 'deleted',
                    timestamp: item.created_at
                  });
                }
              }
              continue;
            }

            const baseName = path.basename(filePath);
            if (name === 'edit_file') {
              changesMap.set(filePath, {
                path: filePath,
                name: baseName,
                action: 'modified',
                timestamp: item.created_at
              });
            } else if (name === 'write_file') {
              const act = args.Overwrite || args.Append ? 'modified' : 'created';
              changesMap.set(filePath, {
                path: filePath,
                name: baseName,
                action: act,
                timestamp: item.created_at
              });
            }
          }
        } catch {}
      }
    } catch {}

    const changes = Array.from(changesMap.values());
    const modified = changes.filter((c) => c.action === 'modified').length;
    const created = changes.filter((c) => c.action === 'created').length;
    const deleted = changes.filter((c) => c.action === 'deleted').length;

    return {
      worker: id,
      conversation_id: latest.id,
      changes,
      summary: {
        modified,
        created,
        deleted,
        total: changes.length
      }
    };
  } catch {
    return {
      worker: id,
      conversation_id: '',
      changes: [],
      summary: { modified: 0, created: 0, deleted: 0, total: 0 }
    };
  }
}

export function executePrompt(
  id: string,
  prompt: string,
  model: string | undefined,
  callbacks: {
    onDelta: (text: string) => void;
    onTool: (tool: { name: string; state: string; path?: string; action?: string; detail?: string }) => void;
    onResult: (result: { status: string; summary: string; conversation_id?: string }) => void;
    onClose: () => void;
    onError: (err: Error) => void;
  }
): () => void {
  const pdir = getProfileHome(id);
  const args = ['-p', prompt, '--output-format', 'stream-json', '--dangerously-skip-permissions'];
  if (model) {
    args.push('--model', model);
  }

  const env = {
    ...process.env,
    HOME: pdir,
    GEMINI_HOME: path.join(pdir, '.gemini'),
    PATH: `/opt/data/tools/go/bin:/opt/data/home/.local/bin:${process.env.PATH || ''}`
  };

  const workspaceCwd = fs.existsSync('/opt/data/home/AgyRemoteTele') ? '/opt/data/home/AgyRemoteTele' : (process.env.WORKSPACE_ROOT || '/opt/data');
  const child = spawn(AGY_BIN, args, { cwd: workspaceCwd, env, stdio: ['ignore', 'pipe', 'pipe'] });
  activeWorkerSessions.set(id, `running_${Date.now()}`);

  let buffer = '';
  let fullResponse = '';
  let convId = '';
  let resultEmitted = false;

  child.stderr.on('data', (errChunk: Buffer) => {
    const errStr = errChunk.toString('utf-8').trim();
    if (errStr && !errStr.includes('ExperimentalWarning')) {
      console.error(`[agy:${id}:stderr]`, errStr);
    }
  });

  child.stdout.on('data', (chunk: Buffer) => {
    buffer += chunk.toString('utf-8');
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      try {
        const event = JSON.parse(trimmed);
        if (event.conversation_id && !convId) {
          convId = event.conversation_id;
        }

        if (event.event === 'step_update' && event.step_update) {
          const u = event.step_update;
          if (u.conversation_id && !convId) convId = u.conversation_id;
          if (u.step_type === 'agent_response' && u.text_delta) {
            const clean = stripClutter(u.text_delta);
            if (clean) {
              fullResponse += clean;
              callbacks.onDelta(clean);
            }
          }
          if (u.step_type === 'tool') {
            const toolName = u.tool_name || u.tool_info?.name || 'tool';
            let targetPath = '';
            let action = '';
            let detail = '';

            const rawArgs = u.args || u.tool_input || u.input || u.tool_call?.args || {};
            if (typeof rawArgs === 'string') {
              try {
                const parsed = JSON.parse(rawArgs);
                targetPath = (parsed.Path || parsed.path || '').replace(/^"|"$/g, '').trim();
                if (parsed.CommandLine) detail = parsed.toolSummary || parsed.CommandLine.slice(0, 40);
              } catch {}
            } else if (typeof rawArgs === 'object' && rawArgs !== null) {
              targetPath = (rawArgs.Path || rawArgs.path || '').replace(/^"|"$/g, '').trim();
              if (rawArgs.CommandLine) detail = rawArgs.toolSummary || rawArgs.CommandLine.slice(0, 40);
            }

            if (toolName === 'edit_file') action = 'modified';
            else if (toolName === 'write_file') action = 'created';
            else if (toolName === 'read_file') action = 'read';
            else if (toolName === 'run_command') action = 'command';

            callbacks.onTool({
              name: toolName,
              state: u.state || 'DONE',
              path: targetPath,
              action,
              detail
            });
          }
        }

        if (event.event === 'result' && event.result) {
          const res = event.result;
          if (res.conversation_id) convId = res.conversation_id;
          const summary = executiveSummary(res.response || fullResponse);
          resultEmitted = true;
          callbacks.onResult({
            status: res.status || 'COMPLETED',
            summary,
            conversation_id: convId
          });
        }
      } catch {}
    }
  });

  child.on('close', (code: number | null) => {
    activeWorkerSessions.delete(id);
    if (!resultEmitted) {
      resultEmitted = true;
      callbacks.onResult({
        status: code === 0 ? 'COMPLETED' : 'FAILED',
        summary: executiveSummary(fullResponse || 'Execution finished'),
        conversation_id: convId
      });
    }
    callbacks.onClose();
  });

  child.on('error', (err: Error) => {
    activeWorkerSessions.delete(id);
    callbacks.onError(err);
  });

  return () => {
    activeWorkerSessions.delete(id);
    try {
      child.kill('SIGTERM');
    } catch {}
  };
}

export function logoutWorker(id: string): { success: boolean; message: string } {
  const pdir = getProfileHome(id);
  const tokenFile = path.join(pdir, '.gemini/antigravity-cli/antigravity-oauth-token');
  const tokenBak = `${tokenFile}.bak`;
  try {
    if (fs.existsSync(tokenFile)) {
      fs.unlinkSync(tokenFile);
    }
    if (fs.existsSync(tokenBak)) {
      fs.unlinkSync(tokenBak);
    }
  } catch {}

  activeWorkerSessions.delete(id);

  const pidFile = `/tmp/agy_login_${id}.pid`;
  if (fs.existsSync(pidFile)) {
    try {
      const pid = parseInt(fs.readFileSync(pidFile, 'utf-8').trim(), 10);
      if (pid && !isNaN(pid)) {
        process.kill(pid, 'SIGTERM');
      }
      fs.unlinkSync(pidFile);
    } catch {}
  }

  for (const ext of ['.sock', '.url']) {
    const f = `/tmp/agy_login_${id}${ext}`;
    if (fs.existsSync(f)) {
      try {
        fs.unlinkSync(f);
      } catch {}
    }
  }

  return { success: true, message: `Worker ${id} disconnected` };
}

export function startWorkerLogin(id: string): Promise<{ success: boolean; authUrl?: string; error?: string }> {
  return new Promise((resolve) => {
    const pyScript = '/opt/data/home/agy-profiles/login_session.py';
    const child = spawn('python3', [pyScript, 'start', id], {
      stdio: ['ignore', 'pipe', 'pipe']
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (c: Buffer) => {
      stdout += c.toString('utf-8');
    });

    child.stderr.on('data', (c: Buffer) => {
      stderr += c.toString('utf-8');
    });

    child.on('close', (code: number | null) => {
      const match = stdout.match(/AUTH_URL=(https:\/\/accounts\.google\.com\/o\/oauth2\/auth\S+)/);
      if (match && match[1]) {
        resolve({ success: true, authUrl: match[1] });
      } else if (stdout.includes('ERROR:')) {
        const errMatch = stdout.match(/ERROR: (.*)/);
        resolve({ success: false, error: errMatch ? errMatch[1].trim() : 'Timeout generating auth URL' });
      } else if (code !== 0) {
        resolve({ success: false, error: stderr.trim() || stdout.trim() || 'Process exited with error' });
      } else {
        const urlFile = `/tmp/agy_login_${id}.url`;
        if (fs.existsSync(urlFile)) {
          const content = fs.readFileSync(urlFile, 'utf-8').trim();
          if (content.startsWith('https://')) {
            resolve({ success: true, authUrl: content });
            return;
          }
        }
        resolve({ success: false, error: 'Failed to extract authorization URL' });
      }
    });

    child.on('error', (err: Error) => {
      resolve({ success: false, error: err.message });
    });
  });
}

export function submitWorkerAuthCode(id: string, code: string): Promise<{ success: boolean; message?: string; error?: string }> {
  return new Promise((resolve) => {
    const pyScript = '/opt/data/home/agy-profiles/login_session.py';
    const child = spawn('python3', [pyScript, 'submit', id, code], {
      stdio: ['ignore', 'pipe', 'pipe']
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (c: Buffer) => {
      stdout += c.toString('utf-8');
    });

    child.stderr.on('data', (c: Buffer) => {
      stderr += c.toString('utf-8');
    });

    child.on('close', (exitCode: number | null) => {
      if (stdout.includes('OK:')) {
        const msg = stdout.replace('OK:', '').trim();
        resolve({ success: true, message: msg });
      } else if (stdout.includes('ERROR:')) {
        const err = stdout.replace('ERROR:', '').trim();
        resolve({ success: false, error: err });
      } else if (exitCode !== 0) {
        resolve({ success: false, error: stderr.trim() || stdout.trim() || 'Authentication failed' });
      } else {
        resolve({ success: true, message: stdout.trim() || 'Authentication completed' });
      }
    });

    child.on('error', (err: Error) => {
      resolve({ success: false, error: err.message });
    });
  });
}

