import http from 'node:http';

const host = '0.0.0.0';
const port = 8787;

const server = http.createServer((request, response) => {
  if (request.url === '/hang') {
    request.on('close', () => response.destroy());
    return;
  }

  if (request.url !== '/events') {
    response.writeHead(404).end();
    return;
  }

  response.writeHead(200, {
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
    'Content-Type': 'text/event-stream; charset=utf-8',
  });
  const payload = Buffer.from('data: 第一条\n\ndata: second\n\n');
  const chunks = [payload.subarray(0, 9), payload.subarray(9, 13), payload.subarray(13)];
  const writeNext = () => {
    const chunk = chunks.shift();
    if (!chunk) {
      response.end();
      return;
    }
    response.write(chunk);
    setTimeout(writeNext, 25);
  };
  writeNext();
});

server.listen(port, host, () => {
  process.stdout.write(`SSE fixture listening on http://${host}:${port}\n`);
});

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
