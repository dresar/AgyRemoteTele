import fs from 'node:fs';
import path from 'node:path';
import express, { type Request, type Response, type NextFunction } from 'express';
import dotenv from 'dotenv';
import { bearerAuth } from './auth/bearer.ts';
import healthRouter from './routes/health.ts';
import telemetryRouter from './routes/telemetry.ts';
import processesRouter from './routes/processes.ts';
import workersRouter from './routes/workers.ts';

dotenv.config();

const app = express();
const PORT = parseInt(process.env.PORT || '8787', 10);
const HOST = process.env.HOST || '127.0.0.1';
const DIST_DIR = path.resolve('/opt/data/home/agyremote/dist');
const APK_PATH = path.join(DIST_DIR, 'agyremote.apk');

app.use((req: Request, res: Response, next: NextFunction) => {
  const origin = req.headers['origin'] || '*';
  res.setHeader('Access-Control-Allow-Origin', origin);
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS, HEAD');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Requested-With, X-Forwarded-For, X-Accel-Buffering');
  res.setHeader('Access-Control-Expose-Headers', 'Content-Length, Content-Type, X-Accel-Buffering');
  if (!req.path.includes('/prompt')) {
    res.setHeader('Connection', 'close');
  }

  if (req.method === 'OPTIONS') {
    res.status(204).end();
    return;
  }
  next();
});

app.use(express.json({ limit: '1mb' }));

app.use((err: any, _req: Request, res: Response, next: NextFunction) => {
  if (err) {
    if (err.type === 'entity.too.large') {
      res.status(413).json({ error: 'Payload too large' });
      return;
    }
    if (err instanceof SyntaxError && 'body' in err) {
      res.status(400).json({ error: 'Malformed JSON payload' });
      return;
    }
    res.status(400).json({ error: err.message || 'Bad Request' });
    return;
  }
  next();
});

app.get(['/agyremote.apk', '/dist/agyremote.apk'], (_req: Request, res: Response) => {
  if (fs.existsSync(APK_PATH)) {
    const stat = fs.statSync(APK_PATH);
    res.setHeader('Content-Type', 'application/vnd.android.package-archive');
    res.setHeader('Content-Disposition', 'attachment; filename="agyremote.apk"');
    res.setHeader('Content-Length', stat.size);
    fs.createReadStream(APK_PATH).pipe(res);
  } else {
    res.status(404).json({ error: 'APK binary not found' });
  }
});

const staticOptions = {
  setHeaders: (res: Response, filePath: string) => {
    if (filePath.endsWith('index.html') || filePath.endsWith('sw.js')) {
      res.setHeader('Cache-Control', 'no-cache, no-store, must-revalidate');
      res.setHeader('Pragma', 'no-cache');
      res.setHeader('Expires', '0');
    }
  }
};

app.use('/dist', express.static(DIST_DIR, staticOptions));
app.use(express.static(DIST_DIR, staticOptions));

app.get(['/', '/index.html'], (_req: Request, res: Response) => {
  const indexPath = path.join(DIST_DIR, 'index.html');
  res.setHeader('Cache-Control', 'no-cache, no-store, must-revalidate');
  res.setHeader('Pragma', 'no-cache');
  res.setHeader('Expires', '0');
  if (fs.existsSync(indexPath)) {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    fs.createReadStream(indexPath).pipe(res);
  } else {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.send('<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>agyremote</title></head><body><div id="root">agyremote</div></body></html>');
  }
});

app.use(bearerAuth);

app.use('/api/health', healthRouter);
app.use('/api/system', telemetryRouter);
app.use('/api/processes', processesRouter);
app.use('/api/workers', workersRouter);

app.use('/api', (_req: Request, res: Response) => {
  res.status(404).json({ error: 'Not Found' });
});

const server = app.listen(PORT, HOST, () => {
  console.log(`agyremote backend daemon listening on http://${HOST}:${PORT}`);
});

process.on('SIGTERM', () => {
  server.close(() => {
    process.exit(0);
  });
});

process.on('SIGINT', () => {
  server.close(() => {
    process.exit(0);
  });
});

export default app;
