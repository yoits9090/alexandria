'use strict';
// Environment resolution shared by the CLI and the daemon. The Go binary is
// configured entirely through environment variables (see cmd/alexandria/main.go).
const fs = require('fs');
const os = require('os');
const path = require('path');

function runtimeDir() {
  return process.env.ALEXANDRIA_RUNTIME_DIR || path.join(os.homedir(), '.alexandria');
}

function pidFile() {
  return path.join(runtimeDir(), 'alexandria.pid');
}

function logFile() {
  return path.join(runtimeDir(), 'alexandria.log');
}

// addrToUrl turns an ALEXANDRIA_ADDR like ":8080", "127.0.0.1:8080",
// "0.0.0.0:8080", or "[::1]:8080" into a probe URL on loopback.
function addrToUrl(addr) {
  if (process.env.ALEXANDRIA_URL) {
    return String(process.env.ALEXANDRIA_URL).replace(/\/+$/, '');
  }
  const a = String(addr || '').trim() || ':8080';
  const m = a.match(/:(\d+)$/);
  const port = m ? m[1] : '8080';
  return `http://127.0.0.1:${port}`;
}

// loadEnvFile parses a simple KEY=VALUE file (blank lines and # comments ok).
function loadEnvFile(file) {
  const env = {};
  if (!file || !fs.existsSync(file)) return env;
  for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const t = line.trim();
    if (!t || t.startsWith('#') || !t.includes('=')) continue;
    const i = t.indexOf('=');
    const key = t.slice(0, i).trim();
    let value = t.slice(i + 1).trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    if (key) env[key] = value;
  }
  return env;
}

// daemonEnv is the environment handed to the spawned binary: the current
// process environment plus any ALEXANDRIA_ENV_FILE overrides.
function daemonEnv() {
  return { ...process.env, ...loadEnvFile(process.env.ALEXANDRIA_ENV_FILE) };
}

module.exports = { runtimeDir, pidFile, logFile, addrToUrl, daemonEnv, loadEnvFile };
