"use strict";
// Integration test for the daemon lifecycle. Builds a local binary if none is
// present, serves a fake SearXNG instance, then drives the CLI:
// start -> status -> search -> logs -> restart -> stop -> status.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const http = require('node:http');
const os = require('os');
const path = require('path');
const { execFile } = require('node:child_process');
const { promisify } = require('node:util');

const execFileP = promisify(execFile);

const PKG_DIR = path.join(__dirname, '..');
const CLI = path.join(PKG_DIR, 'bin', 'alexandria.js');
const EXE = process.platform === 'win32' ? '.exe' : '';

async function run(args, env = {}) {
  try {
    const { stdout, stderr } = await execFileP(process.execPath, [CLI, ...args], {
      timeout: 60000,
      maxBuffer: 16 * 1024 * 1024,
      env: { ...process.env, ...env },
    });
    return { code: 0, stdout, stderr };
  } catch (e) {
    return { code: e.code || 1, stdout: e.stdout || '', stderr: e.stderr || e.message };
  }
}

function buildBinary() {
  const local = path.join(PKG_DIR, 'vendor', `alexandria${EXE}`);
  if (fs.existsSync(local)) return local;
  if (process.env.ALEXANDRIA_BIN) return process.env.ALEXANDRIA_BIN;
  const r = spawnSync('go', ['build', '-o', local, '../cmd/alexandria'], {
    cwd: PKG_DIR,
    encoding: 'utf8',
    timeout: 120000,
  });
  assert.equal(r.status, 0, `go build failed: ${r.stderr}`);
  return local;
}

test('daemon lifecycle: start, status, search, logs, restart, stop', { timeout: 90000 }, async (t) => {
  if (process.platform === 'win32') {
    t.skip('daemon test targets POSIX platforms');
    return;
  }
  const bin = buildBinary();

  // Fake SearXNG instance so the daemon has a working provider offline.
  const fixture = http.createServer((req, res) => {
    if (req.url.startsWith('/search')) {
      res.setHeader('content-type', 'application/json');
      res.end(
        JSON.stringify({
          results: [{ title: 'Fixture Result', url: 'https://example.test', content: 'fixture snippet', score: 1 }],
        })
      );
    } else {
      res.statusCode = 404;
      res.end();
    }
  });
  await new Promise((resolve) => fixture.listen(0, '127.0.0.1', resolve));
  t.after(() => fixture.close());

  const port = 19000 + Math.floor(Math.random() * 900);
  const runtime = fs.mkdtempSync(path.join(os.tmpdir(), 'alexandria-test-'));
  const env = {
    ALEXANDRIA_BIN: bin,
    ALEXANDRIA_ADDR: `127.0.0.1:${port}`,
    ALEXANDRIA_RUNTIME_DIR: runtime,
    SEARX_URL: `http://127.0.0.1:${fixture.address().port}`,
  };
  t.after(() => {
    run(['stop'], env);
    fs.rmSync(runtime, { recursive: true, force: true });
  });

  // start
  let r = await run(['start'], env);
  assert.equal(r.code, 0, `start failed: ${r.stdout} ${r.stderr}`);
  assert.match(r.stdout, /alexandria is up at http:\/\/127\.0\.0\.1:\d+/);
  assert.ok(fs.existsSync(path.join(runtime, 'alexandria.pid')), 'pid file missing');

  // start again: idempotent
  r = await run(['start'], env);
  assert.equal(r.code, 0, `second start failed: ${r.stdout} ${r.stderr}`);
  assert.match(r.stdout, /already running/);

  // status
  r = await run(['status'], env);
  assert.equal(r.code, 0, r.stderr);
  assert.match(r.stdout, /running \(pid \d+\) at http:\/\/127\.0\.0\.1:\d+ — healthy/);

  // search (JSON)
  r = await run(['search', 'hello world', '--format', 'json'], env);
  assert.equal(r.code, 0, `search failed: ${r.stdout} ${r.stderr}`);
  const parsed = JSON.parse(r.stdout);
  assert.equal(parsed.query, 'hello world');
  assert.equal(parsed.results[0].title, 'Fixture Result');
  assert.ok(parsed.usage.max_tokens > 0, 'usage missing');

  // search (TOON default)
  r = await run(['search', 'hello world'], env);
  assert.equal(r.code, 0, r.stderr);
  assert.match(r.stdout, /sources\[1\]\{id,title,url,snippet,source\}:/);

  // logs
  r = await run(['logs', '-n', '5'], env);
  assert.equal(r.code, 0, r.stderr);
  assert.match(r.stdout, /alexandria listening/);

  // restart
  r = await run(['restart'], env);
  assert.equal(r.code, 0, `restart failed: ${r.stdout} ${r.stderr}`);
  assert.match(r.stdout, /alexandria restarted/);
  r = await run(['status'], env);
  assert.match(r.stdout, /healthy/);

  // version
  r = await run(['version'], env);
  assert.equal(r.code, 0, r.stderr);
  assert.match(r.stdout, /alexandria-search npm 0\.1\.0/);
  assert.match(r.stdout, /binary:/);

  // stop
  r = await run(['stop'], env);
  assert.equal(r.code, 0, r.stderr);
  assert.match(r.stdout, /alexandria stopped/);
  assert.ok(!fs.existsSync(path.join(runtime, 'alexandria.pid')), 'pid file not cleaned up');

  r = await run(['status'], env);
  assert.equal(r.code, 0, r.stderr);
  assert.match(r.stdout, /not running/);
});
