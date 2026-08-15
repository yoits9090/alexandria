'use strict';
// Resolves the Go binary: explicit ALEXANDRIA_BIN, a locally built
// vendor/alexandria, or the platform-named prebuilt download.
const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');

const PKG_DIR = path.join(__dirname, '..');
const EXE = process.platform === 'win32' ? '.exe' : '';

function platformBinaryName() {
  return `alexandria-${process.platform}-${process.arch}${EXE}`;
}

function localBinaryPath() {
  return path.join(PKG_DIR, 'vendor', `alexandria${EXE}`);
}

function resolveBinary() {
  if (process.env.ALEXANDRIA_BIN) return process.env.ALEXANDRIA_BIN;
  for (const p of [localBinaryPath(), path.join(PKG_DIR, 'vendor', platformBinaryName())]) {
    if (fs.existsSync(p)) return p;
  }
  return null;
}

function ensureBinary() {
  const bin = resolveBinary();
  if (!bin) {
    throw new Error(
      'alexandria binary not found. Install normally to fetch a prebuilt binary, ' +
      'or build it with `npm run build:binary` (requires Go 1.23+), ' +
      'or point ALEXANDRIA_BIN at an existing binary.'
    );
  }
  return bin;
}

function binaryVersion(bin) {
  try {
    const r = spawnSync(bin, ['--version'], { encoding: 'utf8', timeout: 5000 });
    if (r.status !== 0) return null;
    return r.stdout.trim();
  } catch {
    return null;
  }
}

module.exports = { resolveBinary, ensureBinary, binaryVersion, platformBinaryName };
