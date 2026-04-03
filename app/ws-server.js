const { WebSocketServer } = require('ws');
const http = require('http');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const os = require('os');

// Generate a session token — random, never stored to disk
const sessionToken = crypto.randomBytes(16).toString('hex');

let wss = null;
let httpServer = null;
let wsPort = 0;
const authenticatedClients = new Set();
const pwaDir = path.join(__dirname, '..', 'pwa');

// Message handlers — set by main.js to bridge into SDK
let onSendMessage = null;
let onApproveTool = null;
let onDenyTool = null;
let onAnswerQuestion = null;

function startServer() {
  return new Promise((resolve) => {
    // HTTP server serves PWA files on the same port as WebSocket
    // This avoids mixed-content (https→ws) browser restrictions
    const mimeTypes = { '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.json': 'application/json' };

    httpServer = http.createServer((req, res) => {
      let filePath = req.url.split('?')[0];
      if (filePath === '/') filePath = '/index.html';

      const fullPath = path.join(pwaDir, filePath);
      const ext = path.extname(fullPath);

      if (fs.existsSync(fullPath)) {
        res.writeHead(200, { 'Content-Type': mimeTypes[ext] || 'text/plain', 'Access-Control-Allow-Origin': '*' });
        res.end(fs.readFileSync(fullPath));
      } else {
        // SPA fallback — serve index.html (preserves query params)
        res.writeHead(200, { 'Content-Type': 'text/html', 'Access-Control-Allow-Origin': '*' });
        res.end(fs.readFileSync(path.join(pwaDir, 'index.html')));
      }
    });

    wss = new WebSocketServer({ server: httpServer });

    httpServer.listen(0, '0.0.0.0', () => {
      wsPort = httpServer.address().port;
      console.log(`[WS+HTTP] Server listening on port ${wsPort}`);
      resolve(wsPort);
    });
  });
}

function setupConnectionHandler() {
  wss.on('connection', (ws) => {
    let authed = false;

    ws.on('message', (raw) => {
      let msg;
      try { msg = JSON.parse(raw); } catch { return; }

      // Auth handshake — must be first message
      if (!authed) {
        if (msg.type === 'auth' && msg.token === sessionToken) {
          authed = true;
          authenticatedClients.add(ws);
          ws.send(JSON.stringify({ type: 'auth-ok' }));
          console.log('[WS] Client authenticated');
        } else {
          ws.send(JSON.stringify({ type: 'auth-fail' }));
          ws.close();
        }
        return;
      }

      // Route authenticated messages
      switch (msg.type) {
        case 'send-message':
          if (onSendMessage) onSendMessage(msg.text);
          broadcastExcept(ws, 'user-message', { text: msg.text });
          break;
        case 'approve-tool':
          if (onApproveTool) onApproveTool(msg.toolUseID);
          break;
        case 'deny-tool':
          if (onDenyTool) onDenyTool(msg.toolUseID);
          break;
        case 'answer-question':
          if (onAnswerQuestion) onAnswerQuestion(msg.toolUseID, msg.answers);
          break;
      }
    });

    ws.on('close', () => {
      authenticatedClients.delete(ws);
      console.log('[WS] Client disconnected');
    });
  });
}

async function start() {
  await startServer();
  setupConnectionHandler();
}

// Broadcast to all authenticated PWA clients
function broadcast(type, payload) {
  if (authenticatedClients.size === 0) return;
  const msg = JSON.stringify({ type, payload });
  for (const client of authenticatedClients) {
    try {
      if (client.readyState === 1) client.send(msg);
    } catch { authenticatedClients.delete(client); }
  }
}

// Broadcast to all clients EXCEPT the sender
function broadcastExcept(sender, type, payload) {
  if (authenticatedClients.size === 0) return;
  const msg = JSON.stringify({ type, payload });
  for (const client of authenticatedClients) {
    try {
      if (client !== sender && client.readyState === 1) client.send(msg);
    } catch { authenticatedClients.delete(client); }
  }
}

// Get the local IP for QR code (same WiFi)
function getLocalIP() {
  const interfaces = os.networkInterfaces();
  for (const name of Object.keys(interfaces)) {
    for (const iface of interfaces[name]) {
      if (iface.family === 'IPv4' && !iface.internal) {
        return iface.address;
      }
    }
  }
  return '127.0.0.1';
}

function getConnectionInfo() {
  return {
    host: getLocalIP(),
    port: wsPort,
    token: sessionToken,
  };
}

module.exports = {
  startServer: start,
  broadcast,
  getConnectionInfo,
  setHandlers(handlers) {
    onSendMessage = handlers.onSendMessage;
    onApproveTool = handlers.onApproveTool;
    onDenyTool = handlers.onDenyTool;
    onAnswerQuestion = handlers.onAnswerQuestion;
  },
};
