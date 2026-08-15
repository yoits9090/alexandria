'use strict';
// Minimal search client for the daemon's /v1/search endpoint.
const { addrToUrl } = require('./config');

async function search(query, opts = {}) {
  const url = opts.url || addrToUrl(process.env.ALEXANDRIA_ADDR);
  const params = new URLSearchParams({
    q: query,
    max_results: String(opts.maxResults ?? 5),
    max_tokens: String(opts.maxTokens ?? 800),
    format: opts.format || 'toon',
  });
  const headers = {};
  if (process.env.ALEXANDRIA_API_KEY) headers['X-API-Key'] = process.env.ALEXANDRIA_API_KEY;
  const res = await fetch(`${url}/v1/search?${params}`, {
    headers,
    signal: AbortSignal.timeout(opts.timeoutMs || 30000),
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`search failed (HTTP ${res.status}): ${body.slice(0, 500)}`);
  }
  const type = res.headers.get('content-type') || '';
  return type.includes('json') ? res.json() : res.text();
}

module.exports = { search };
