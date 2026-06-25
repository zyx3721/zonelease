import http from 'node:http';
import { pathToFileURL } from 'node:url';

const host = process.env.FRONTEND_HOST || '127.0.0.1';
const port = Number(process.env.FRONTEND_PORT || 5173);
const serverEntryPath = process.env.FRONTEND_SERVER_ENTRY || '/app/frontend/dist/server/server.js';

function importSpecifier(path) {
  if (/^(file|data|node):/.test(path)) return path;
  if (path.startsWith('/') || /^[a-zA-Z]:[\\/]/.test(path)) {
    return pathToFileURL(path).href;
  }
  return path;
}

const serverEntry = await import(importSpecifier(serverEntryPath));
const handler = serverEntry.default;

if (!handler || typeof handler.fetch !== 'function') {
  throw new Error(`Frontend server entry ${serverEntryPath} does not export a fetch handler`);
}

function requestOrigin(req) {
  const proto = req.headers['x-forwarded-proto'] || 'http';
  const hostHeader = req.headers['x-forwarded-host'] || req.headers.host || `${host}:${port}`;
  return `${proto}://${hostHeader}`;
}

function createFetchRequest(req) {
  const url = new URL(req.url || '/', requestOrigin(req));
  const headers = new Headers();

  for (const [name, value] of Object.entries(req.headers)) {
    if (Array.isArray(value)) {
      for (const item of value) headers.append(name, item);
    } else if (value !== undefined) {
      headers.set(name, value);
    }
  }

  const init = {
    method: req.method,
    headers,
  };

  if (req.method !== 'GET' && req.method !== 'HEAD') {
    init.body = req;
    init.duplex = 'half';
  }

  return new Request(url, init);
}

function writeFetchResponse(res, response) {
  res.statusCode = response.status;
  res.statusMessage = response.statusText;

  for (const [name, value] of response.headers) {
    if (name.toLowerCase() !== 'set-cookie') {
      res.setHeader(name, value);
    }
  }

  const cookies = response.headers.getSetCookie?.();
  if (cookies?.length) {
    res.setHeader('set-cookie', cookies);
  }

  if (!response.body) {
    res.end();
    return;
  }

  const reader = response.body.getReader();

  function pump() {
    reader.read().then(({ done, value }) => {
      if (done) {
        res.end();
        return;
      }

      if (!res.write(Buffer.from(value))) {
        res.once('drain', pump);
      } else {
        pump();
      }
    }).catch(error => {
      console.error(error);
      if (!res.headersSent) res.statusCode = 500;
      res.end();
    });
  }

  pump();
}

const server = http.createServer(async (req, res) => {
  try {
    const request = createFetchRequest(req);
    const response = await handler.fetch(request, {}, {});
    writeFetchResponse(res, response);
  } catch (error) {
    console.error(error);
    if (!res.headersSent) {
      res.statusCode = 500;
      res.setHeader('content-type', 'text/plain; charset=utf-8');
    }
    res.end('Frontend server error');
  }
});

server.listen(port, host, () => {
  console.log(`Frontend server listening on http://${host}:${port}`);
});
