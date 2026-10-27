import express, { type Request, type Response } from 'express';

const router = express.Router();

router.get('/', (_req: Request, res: Response) => {
  res.status(200).json({
    status: 'ok',
    timestamp: new Date().toISOString(),
    version: '1.0.0',
    uptime: Math.floor(process.uptime())
  });
});

router.all('/', (_req: Request, res: Response) => {
  res.status(405).json({ error: 'Method Not Allowed' });
});

export default router;
