import { getAuthToken } from './api.ts';
import type { PromptSSEEvent } from '../types/telemetry.ts';

export interface StreamCallbacks {
  onDelta: (text: string) => void;
  onTool: (tool: { name: string; state: string; path?: string; action?: string; detail?: string }) => void;
  onFileChange?: (file: { path: string; action: 'modified' | 'created' | 'deleted'; name: string }) => void;
  onResult: (result: { status: string; summary: string; conversation_id?: string }) => void;
  onError: (error: string) => void;
  onClose: () => void;
}

export function streamWorkerPrompt(
  workerId: string,
  prompt: string,
  callbacks: StreamCallbacks,
  model?: string
): () => void {
  const controller = new AbortController();
  const token = getAuthToken();

  async function execute() {
    try {
      const res = await fetch(`/api/workers/${workerId}/prompt`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ prompt, model }),
        signal: controller.signal
      });

      if (!res.ok) {
        const errorText = await res.text();
        callbacks.onError(`HTTP ${res.status}: ${errorText || res.statusText}`);
        callbacks.onClose();
        return;
      }

      if (!res.body) {
        callbacks.onError('ReadableStream body is absent from response');
        callbacks.onClose();
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder('utf-8');
      let buffer = '';

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed || !trimmed.startsWith('data:')) continue;

          const dataPayload = trimmed.slice(5).trim();
          if (dataPayload === '[DONE]') {
            callbacks.onClose();
            return;
          }

          try {
            const event: PromptSSEEvent = JSON.parse(dataPayload);
            if (event.type === 'text_delta' && event.text) {
              callbacks.onDelta(event.text);
            } else if (event.type === 'tool_status') {
              callbacks.onTool({
                name: (event as any).name || 'tool',
                state: (event as any).state || 'DONE',
                path: (event as any).path,
                action: (event as any).action,
                detail: (event as any).detail
              });
            } else if (event.type === 'file_change' && (event as any).file) {
              callbacks.onFileChange?.((event as any).file);
            } else if (event.type === 'result') {
              callbacks.onResult({
                status: event.status || 'COMPLETED',
                summary: event.summary || '',
                conversation_id: event.conversation_id
              });
            } else if (event.type === 'error') {
              callbacks.onError(event.message || 'Stream encountered error');
            }
          } catch {}
        }
      }

      callbacks.onClose();
    } catch (err: any) {
      if (err.name !== 'AbortError') {
        callbacks.onError(err.message || 'Network stream error');
      }
      callbacks.onClose();
    }
  }

  execute();

  return () => {
    controller.abort();
  };
}
