import express, { type Request, type Response } from 'express';
import { getSystemVitals } from '../services/proc.ts';

const router = express.Router();

router.get('/vitals', (_req: Request, res: Response) => {
  try {
    const vitals = getSystemVitals();
    res.status(200).json(vitals);
  } catch (err: any) {
    res.status(500).json({ error: err.message || 'Failed to collect vitals' });
  }
});

export default router;
