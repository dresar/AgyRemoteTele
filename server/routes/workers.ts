import express, { type Request, type Response } from 'express';
import {
  listWorkers,
  getWorker,
  getWorkerTranscripts,
  getWorkerFileChanges,
  executePrompt,
  isValidWorkerId,
  startWorkerLogin,
  submitWorkerAuthCode,
  logoutWorker
} from '../services/agyExecutor.ts';

const router = express.Router();

function validateWorkerId(id: string, res: Response): boolean {
  if (id.includes(';') || id.includes(' ') || id.includes('`') || id.includes('$') || id.includes('|') || id.includes('&')) {
    res.status(400).json({ error: 'Malformed worker ID with invalid characters' });
    return false;
  }
  if (!isValidWorkerId(id)) {
    res.status(404).json({ error: 'Worker profile not found' });
    return false;
  }
  return true;
}

router.get('/', (_req: Request, res: Response) => {
  try {
    const workers = listWorkers();
    res.status(200).json({ workers });
  } catch (err: any) {
    res.status(500).json({ error: err.message || 'Failed to list workers' });
  }
});

router.get('/:id', (req: Request, res: Response) => {
  const id = req.params.id;
  if (!validateWorkerId(id, res)) return;

  const worker = getWorker(id);
  if (!worker) {
    res.status(404).json({ error: 'Worker profile not found' });
    return;
  }
  res.status(200).json(worker);
});

router.get('/:id/transcripts', (req: Request, res: Response) => {
  const id = req.params.id;
  if (!validateWorkerId(id, res)) return;

  const data = getWorkerTranscripts(id);
  if (!data) {
    res.status(404).json({ error: 'Worker profile not found' });
    return;
  }
  res.status(200).json(data);
});

router.get('/:id/changes', (req: Request, res: Response) => {
  const id = req.params.id;
  if (!validateWorkerId(id, res)) return;

  const data = getWorkerFileChanges(id);
  res.status(200).json(data);
});

router.post('/:id/prompt', (req: Request, res: Response) => {
  const id = req.params.id;
  if (!validateWorkerId(id, res)) return;

  if (!req.body || typeof req.body !== 'object' || !req.body.prompt || typeof req.body.prompt !== 'string') {
    res.status(400).json({ error: 'Prompt field is required and must be a non-empty string' });
    return;
  }

  const promptText = req.body.prompt.trim();
  if (!promptText) {
    res.status(400).json({ error: 'Prompt must not be empty' });
    return;
  }

  if (promptText.length > 1000000) {
    res.status(413).json({ error: 'Prompt exceeds maximum payload size' });
    return;
  }

  res.setHeader('Content-Type', 'text/event-stream; charset=utf-8');
  res.setHeader('Cache-Control', 'no-cache, no-transform');
  res.setHeader('Connection', 'keep-alive');
  res.setHeader('X-Accel-Buffering', 'no');
  res.flushHeaders?.();

  res.write(`data: ${JSON.stringify({ type: 'start', worker: id })}\n\n`);

  let closed = false;
  const keepAlive = setInterval(() => {
    if (!closed) {
      try {
        res.write(': ping\n\n');
      } catch {}
    }
  }, 10000);

  const finish = () => {
    clearInterval(keepAlive);
    if (!closed) {
      closed = true;
      try {
        res.write('data: [DONE]\n\n');
        res.end();
      } catch {}
    }
  };

  const cancel = executePrompt(id, promptText, req.body.model, {
    onDelta: (text: string) => {
      if (!closed) {
        res.write(`data: ${JSON.stringify({ type: 'text_delta', text })}\n\n`);
      }
    },
    onTool: (tool: { name: string; state: string; path?: string; action?: string; detail?: string }) => {
      if (!closed) {
        res.write(`data: ${JSON.stringify({ type: 'tool_status', ...tool })}\n\n`);
        if (tool.path && (tool.action === 'modified' || tool.action === 'created' || tool.action === 'deleted')) {
          res.write(`data: ${JSON.stringify({
            type: 'file_change',
            file: {
              path: tool.path,
              action: tool.action,
              name: tool.path.split('/').pop() || tool.path
            }
          })}\n\n`);
        }
      }
    },
    onResult: (result: { status: string; summary: string; conversation_id?: string }) => {
      if (!closed) {
        res.write(`data: ${JSON.stringify({ type: 'result', ...result })}\n\n`);
      }
    },
    onClose: () => {
      finish();
    },
    onError: (err: Error) => {
      if (!closed) {
        res.write(`data: ${JSON.stringify({ type: 'error', message: err.message })}\n\n`);
        finish();
      }
    }
  });

  res.on('close', () => {
    if (!res.writableEnded) {
      clearInterval(keepAlive);
      closed = true;
      cancel();
    }
  });
});

router.post('/:id/activate', (req: Request, res: Response) => {
  const id = req.params.id;
  if (!validateWorkerId(id, res)) return;

  const worker = getWorker(id);
  res.status(200).json({
    success: true,
    message: worker?.authenticated ? 'Worker aktif' : 'Worker diinisialisasi',
    worker
  });
});

router.post('/:id/auth/login', async (req: Request, res: Response) => {
  const id = req.params.id;
  if (!validateWorkerId(id, res)) return;

  try {
    const result = await startWorkerLogin(id);
    if (!result.success) {
      res.status(500).json({ error: result.error || 'Gagal memulai sesi login' });
      return;
    }
    res.status(200).json({
      success: true,
      authUrl: result.authUrl,
      worker: id
    });
  } catch (err: any) {
    res.status(500).json({ error: err.message || 'Gagal memulai login' });
  }
});

router.post('/:id/auth/submit', async (req: Request, res: Response) => {
  const id = req.params.id;
  if (!validateWorkerId(id, res)) return;

  const code = req.body?.code;
  if (!code || typeof code !== 'string' || !code.trim()) {
    res.status(400).json({ error: 'Kode otorisasi diperlukan' });
    return;
  }

  try {
    const result = await submitWorkerAuthCode(id, code.trim());
    if (!result.success) {
      res.status(400).json({ error: result.error || 'Kode otorisasi tidak valid' });
      return;
    }
    const worker = getWorker(id);
    res.status(200).json({
      success: true,
      message: result.message || 'Login berhasil',
      worker
    });
  } catch (err: any) {
    res.status(500).json({ error: err.message || 'Gagal memproses kode otorisasi' });
  }
});

router.post('/:id/auth/logout', (req: Request, res: Response) => {
  const id = req.params.id;
  if (!validateWorkerId(id, res)) return;

  try {
    const result = logoutWorker(id);
    const worker = getWorker(id);
    res.status(200).json({
      success: true,
      message: result.message,
      worker
    });
  } catch (err: any) {
    res.status(500).json({ error: err.message || 'Gagal logout worker' });
  }
});

export default router;
