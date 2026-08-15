'use strict';
// Background daemon lifecycle: spawn the Go gateway detached with a pid file
// and appended log, wait for /healthz, stop with SIGTERM (the binary drains
// gracefully) and a SIGKILL fallback.
const fs = require('fs');
const { spawn } = require('child_process');
const { addrToUrl, daemonEnv, runtimeDir, pidFile, logFile } = require('./config');
const { ensureBinary } = require('./binary');

function readPid() {
  try {
    const p = Number(fs.readFileSync(pidFile(), 'utf8').trim());
    return Number.isInteger(p) && p > 0 ? p : null;
  } catch {
    return null;
  }
}

function isAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

async function isHealthy(url) {
  try {
    const res = await fetch(`${url}/healthz`, { signal: AbortSignal.timeout(2000) });
    return res.ok;
  } catch {
    return false;
  }
}

async function waitForHealth(url, timeoutMs) {
  const deadline = Date.now() + (timeoutMs || 15000);
  let lastErr = null;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${url}/healthz`, { signal: AbortSignal.timeout(2000) });
      if (res.ok) return;
      lastErr = new Error(`health check returned HTTP ${res.status}`);
    } catch (e) {
      lastErr = e;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw lastErr || new Error('health check timed out');
}

async function start(opts = {}) {
  const url = addrToUrl(process.env.ALEXANDRIA_ADDR);
  const bin = ensureBinary();

  // Adopt an already-healthy gateway instead of double-spawning.
  if (await isHealthy(url)) {
    const pid = readPid();
    return { already: true, pid, url, healthy: true };
  }
  const pid = readPid();
  if (pid && isAlive(pid)) {
    throw new Error(`alexandria (pid ${pid}) is running but not healthy at ${url}; check ${logFile()}`);
  }

  fs.mkdirSync(runtimeDir(), { recursive: true });
  const log = fs.openSync(logFile(), 'a');
  const child = spawn(bin, [], { detached: true, stdio: ['ignore', log, log], env: daemonEnv() });
  fs.closeSync(log);
  fs.writeFileSync(pidFile(), String(child.pid));
  child.unref();

  try {
    await waitForHealth(url, opts.timeoutMs);
  } catch (e) {
    try {
      process.kill(child.pid, 'SIGTERM');
    } catch {}
    try {
      fs.unlinkSync(pidFile());
    } catch {}
    throw new Error(`alexandria failed to become healthy at ${url}: ${e.message}\nSee ${logFile()}`);
  }
  return { pid: child.pid, url, healthy: true };
}

async function stop() {
  const pid = readPid();
  if (!pid || !isAlive(pid)) {
    try {
      fs.unlinkSync(pidFile());
    } catch {}
    return { stopped: false, pid: null };
  }
  try {
    process.kill(pid, 'SIGTERM');
  } catch {}
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline && isAlive(pid)) {
    await new Promise((r) => setTimeout(r, 200));
  }
  if (isAlive(pid)) {
    try {
      process.kill(pid, 'SIGKILL');
    } catch {}
    await new Promise((r) => setTimeout(r, 300));
  }
  try {
    fs.unlinkSync(pidFile());
  } catch {}
  return { stopped: !isAlive(pid), pid };
}

async function status() {
  const url = addrToUrl(process.env.ALEXANDRIA_ADDR);
  const pid = readPid();
  if (pid && isAlive(pid)) {
    return { running: true, pid, url, healthy: await isHealthy(url) };
  }
  return { running: false, pid: null, url, healthy: false };
}

function tailLog(n) {
  try {
    const lines = fs.readFileSync(logFile(), 'utf8').split(/\r?\n/).filter(Boolean);
    return lines.slice(-n).join('\n');
  } catch {
    throw new Error(`no log file yet at ${logFile()} — start the daemon first`);
  }
}

module.exports = { start, stop, status, tailLog, isHealthy };
