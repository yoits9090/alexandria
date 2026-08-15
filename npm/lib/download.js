"use strict";
// postinstall: fetch a prebuilt Go binary from GitHub releases.
// Fails soft (warns, exits 0) when no release exists yet so `npm install`
// never breaks; `npm run build:binary` and ALEXANDRIA_BIN remain the local
// paths in that case.
const fs = require('fs');
const path = require('path');
const https = require('https');

const PKG_DIR = path.join(__dirname, '..');
const pkg = require('../package.json');

// Platforms with published binaries. Unknown platforms fall back to a source
// build instead of failing the install.
const TARGETS = new Set(['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64', 'win32-x64']);

function log(msg) {
  console.log(`[alexandria] ${msg}`);
}

function get(url) {
  return new Promise((resolve, reject) => {
    https.get(url, { headers: { 'User-Agent': `alexandria-search/${pkg.version}` } }, resolve).on('error', reject);
  });
}

async function download(url, dest) {
  let current = url;
  for (let i = 0; i < 5; i++) {
    const res = await get(current);
    if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
      res.resume();
      current = new URL(res.headers.location, current).toString();
      continue;
    }
    if (res.statusCode !== 200) {
      res.resume();
      throw new Error(`HTTP ${res.statusCode} from ${current}`);
    }
    const tmp = `${dest}.tmp`;
    const out = fs.createWriteStream(tmp);
    await new Promise((resolve, reject) => {
      res.pipe(out);
      res.on('end', resolve);
      res.on('error', reject);
      out.on('error', reject);
    });
    fs.renameSync(tmp, dest);
    fs.chmodSync(dest, 0o755);
    return;
  }
  throw new Error('too many redirects');
}

async function main() {
  if (process.env.ALEXANDRIA_SKIP_DOWNLOAD === '1') {
    log('skipping binary download (ALEXANDRIA_SKIP_DOWNLOAD=1)');
    return;
  }
  const key = `${process.platform}-${process.arch}`;
  if (!TARGETS.has(key)) {
    log(`no prebuilt binary for ${key}; run \`npm run build:binary\` (requires Go 1.23+)`);
    return;
  }
  const exe = process.platform === 'win32' ? '.exe' : '';
  const dest = path.join(PKG_DIR, 'vendor', `alexandria-${key}${exe}`);
  if (fs.existsSync(dest)) {
    log('prebuilt binary already present');
    return;
  }
  const base =
    process.env.ALEXANDRIA_DOWNLOAD_URL ||
    `https://github.com/yoits9090/alexandria/releases/download/v${pkg.version}/alexandria-${key}${exe}`;
  try {
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    await download(base, dest);
    log(`downloaded prebuilt binary v${pkg.version}`);
  } catch (e) {
    try {
      fs.unlinkSync(`${dest}.tmp`);
    } catch {}
    log(`could not download prebuilt binary (${e.message}); use \`npm run build:binary\` or set ALEXANDRIA_BIN`);
  }
}

main().catch((e) => log(`download failed: ${e.message}`));
