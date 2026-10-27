import express, { type Request, type Response } from 'express';
import { listProcesses, killProcess } from '../services/proc.ts';

const router = express.Router();

router.get('/', (_req: Request, res: Response) => {
  try {
    const processes = listProcesses();
    res.status(200).json({ processes });
  } catch (err: any) {
    res.status(500).json({ error: err.message || 'Failed to list processes' });
  }
});

router.post('/:pid/kill', (req: Request, res: Response) => {
  const pid = req.params.pid;
  const result = killProcess(pid);
  if (result.status === 200) {
    res.status(200).json(result.data);
  } else {
    res.status(result.status).json({ error: result.error, success: false });
  }
});

export default router;
