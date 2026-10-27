import fs from 'node:fs';
import path from 'node:path';
import { execSync } from 'node:child_process';

export const BASE_URL = process.env.AGYREMOTE_BASE_URL || 'http://127.0.0.1:8787';
export const CLOUDFLARE_URL = 'https://agyremote.agentai.my.id';
export const ADMIN_AUTH_KEY = process.env.ADMIN_AUTH_KEY || 'agy-cluster-control-2026';
export const LEGACY_DIR = '/opt/data/home/AgyRemoteTele';
export const BACKUP_TAR = '/opt/data/home/agyremote/AgyRemoteTele_backup.tar.gz';
export const PROFILES_DIR = '/opt/data/home/agy-profiles';
export const APK_PATH = '/opt/data/home/agyremote/dist/agyremote.apk';
export const WEB_DIST_PATH = '/opt/data/home/agyremote/dist/index.html';
export const WEB_SRC_PATH = '/opt/data/home/agyremote/src';

export async function isBackendAlive(timeoutMs = 400) {
  try {
    const res = await fetch(`${BASE_URL}/api/health`, {
      signal: AbortSignal.timeout(timeoutMs)
    });
    return res.status === 200;
  } catch {
    return false;
  }
}

export function isApkPresent() {
  try {
    return fs.existsSync(APK_PATH) && fs.statSync(APK_PATH).size > 1024 * 1024;
  } catch {
    return false;
  }
}

export function isFrontendBuilt() {
  try {
    return fs.existsSync(WEB_DIST_PATH) && fs.statSync(WEB_DIST_PATH).size > 0;
  } catch {
    return false;
  }
}

export function isFrontendSourcePresent() {
  try {
    return fs.existsSync(path.join(WEB_SRC_PATH, 'App.tsx')) || fs.existsSync(path.join(WEB_SRC_PATH, 'main.tsx'));
  } catch {
    return false;
  }
}

export function isCloudflaredAlive() {
  try {
    const out = execSync('pgrep -f cloudflared || true', { encoding: 'utf-8' }).trim();
    return out.length > 0;
  } catch {
    return false;
  }
}

export function isLegacyDecommissioned() {
  try {
    const p1 = execSync('pgrep -x [a]gyremote || true', { encoding: 'utf-8' }).trim();
    const p2 = execSync('pgrep -f "[A]gyRemoteTele/daemon.sh" || true', { encoding: 'utf-8' }).trim();
    const marker = fs.existsSync(path.join(LEGACY_DIR, 'DECOMMISSIONED.md'));
    const backup = fs.existsSync(BACKUP_TAR) && fs.statSync(BACKUP_TAR).size > 1024 * 1024;
    return p1.length === 0 && p2.length === 0 && marker && backup;
  } catch {
    return false;
  }
}

export async function requestApi(endpoint, options = {}) {
  const url = endpoint.startsWith('http') ? endpoint : `${BASE_URL}${endpoint}`;
  const headers = { ...options.headers };
  if (options.auth !== false && !headers['Authorization'] && !headers['authorization']) {
    headers['Authorization'] = `Bearer ${ADMIN_AUTH_KEY}`;
  }
  const opts = { ...options, headers };
  delete opts.auth;
  return fetch(url, opts);
}
