import type { WorkerProfile, SystemVitals, ProcessItem, TranscriptItem } from '../types/telemetry.ts';

const DEFAULT_AUTH_TOKEN = 'agy-cluster-control-2026';

export function getAuthToken(): string {
  if (typeof window !== 'undefined' && window.localStorage) {
    return window.localStorage.getItem('agy_token') || DEFAULT_AUTH_TOKEN;
  }
  return DEFAULT_AUTH_TOKEN;
}

export function setAuthToken(token: string): void {
  if (typeof window !== 'undefined' && window.localStorage) {
    window.localStorage.setItem('agy_token', token);
  }
}

async function apiRequest<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const token = getAuthToken();
  const headers = new Headers(options.headers || {});
  if (!headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  if (!headers.has('Content-Type') && options.body) {
    headers.set('Content-Type', 'application/json');
  }

  const res = await fetch(endpoint, {
    ...options,
    headers
  });

  if (!res.ok) {
    const errorText = await res.text();
    throw new Error(`API error ${res.status}: ${errorText || res.statusText}`);
  }

  return res.json() as Promise<T>;
}

export async function fetchHealth(): Promise<{ status: string; uptime: number; timestamp: string }> {
  const res = await fetch('/api/health');
  if (!res.ok) throw new Error('Health check failed');
  return res.json();
}

export async function fetchVitals(): Promise<SystemVitals> {
  return apiRequest<SystemVitals>('/api/system/vitals');
}

export async function fetchWorkers(): Promise<WorkerProfile[]> {
  const data = await apiRequest<{ workers: WorkerProfile[] }>('/api/workers');
  return data.workers;
}

export async function fetchWorker(id: string): Promise<WorkerProfile> {
  return apiRequest<WorkerProfile>(`/api/workers/${id}`);
}

export async function fetchWorkerTranscripts(id: string): Promise<TranscriptItem[]> {
  const data = await apiRequest<{ id: string; transcripts: TranscriptItem[] }>(`/api/workers/${id}/transcripts`);
  return data.transcripts || [];
}

export async function fetchProcesses(): Promise<ProcessItem[]> {
  const data = await apiRequest<{ processes: ProcessItem[] }>('/api/processes');
  return data.processes || [];
}

export async function killProcess(pid: number): Promise<{ success: boolean; signal: string; pid: number }> {
  return apiRequest<{ success: boolean; signal: string; pid: number }>(`/api/processes/${pid}/kill`, {
    method: 'POST'
  });
}

export interface FileChangeItem {
  path: string;
  name: string;
  action: 'modified' | 'created' | 'deleted';
  timestamp?: string;
}

export interface WorkerFileChanges {
  worker: string;
  conversation_id: string;
  changes: FileChangeItem[];
  summary: {
    modified: number;
    created: number;
    deleted: number;
    total: number;
  };
}

export async function fetchWorkerChanges(id: string): Promise<WorkerFileChanges> {
  return apiRequest<WorkerFileChanges>(`/api/workers/${id}/changes`);
}

export async function activateWorker(id: string): Promise<{ success: boolean; message: string; worker?: WorkerProfile }> {
  return apiRequest<{ success: boolean; message: string; worker?: WorkerProfile }>(`/api/workers/${id}/activate`, {
    method: 'POST'
  });
}

export async function startWorkerLogin(id: string): Promise<{ success: boolean; authUrl: string; worker: string }> {
  return apiRequest<{ success: boolean; authUrl: string; worker: string }>(`/api/workers/${id}/auth/login`, {
    method: 'POST'
  });
}

export async function submitWorkerAuthCode(id: string, code: string): Promise<{ success: boolean; message: string; worker?: WorkerProfile }> {
  return apiRequest<{ success: boolean; message: string; worker?: WorkerProfile }>(`/api/workers/${id}/auth/submit`, {
    method: 'POST',
    body: JSON.stringify({ code })
  });
}

export async function logoutWorker(id: string): Promise<{ success: boolean; message: string; worker?: WorkerProfile }> {
  return apiRequest<{ success: boolean; message: string; worker?: WorkerProfile }>(`/api/workers/${id}/auth/logout`, {
    method: 'POST'
  });
}

