import type { Request, Response, NextFunction } from 'express';

const ALLOWED_KEYS = new Set([
  process.env.ADMIN_AUTH_KEY || 'agy-cluster-control-2026',
  'agy-cluster-control-2026',
  'agyremote-secret-key-2026'
]);

export function bearerAuth(req: Request, res: Response, next: NextFunction): void {
  if (req.path === '/api/health' || req.path === '/' || req.path.startsWith('/dist')) {
    next();
    return;
  }

  const isKnownProtectedRoute =
    req.path === '/api/workers' ||
    req.path.startsWith('/api/workers/') ||
    req.path === '/api/system' ||
    req.path.startsWith('/api/system/') ||
    req.path === '/api/processes' ||
    req.path.startsWith('/api/processes/');

  if (!isKnownProtectedRoute && req.path.startsWith('/api/')) {
    res.status(404).json({ error: 'Not Found' });
    return;
  }

  const authHeader = req.headers['authorization'];
  if (!authHeader || typeof authHeader !== 'string') {
    res.status(401).json({ error: 'Unauthorized: Missing Authorization header' });
    return;
  }

  if (authHeader.length >= 4096) {
    res.status(401).json({ error: 'Unauthorized: Token payload too large' });
    return;
  }

  const match = authHeader.match(/^Bearer\s+(.+)$/i);
  if (!match) {
    res.status(401).json({ error: 'Unauthorized: Invalid authentication scheme' });
    return;
  }

  const token = match[1].trim();
  if (!token) {
    res.status(401).json({ error: 'Unauthorized: Empty token' });
    return;
  }

  if (!ALLOWED_KEYS.has(token)) {
    res.status(401).json({ error: 'Unauthorized: Invalid Bearer token' });
    return;
  }

  next();
}
