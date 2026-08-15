"use strict";

const daemon = require('./daemon');
const api = require('./api');
const { ensureBinary, binaryVersion } = require('./binary');
const { logFile } = require('./config');
const pkg = require('../package.json');

const HELP = `alexandria — local search daemon for LLM apps

Usage: alexandria <command> [options]

Commands:
  start                  start the daemon in the background (waits for health)
  stop                   stop the daemon (graceful SIGTERM)
  restart                stop then start
  status                 show whether the daemon is running and healthy
  logs [-n N]            print the last N lines of the daemon log (default 100)
  search <query>         search through the daemon
                         [--max-results N] [--max-tokens N]
                         [--format json|toon|text|html] [--url http://host:port]
  version                print npm package and Go binary versions
  help                   show this help

Environment:
  ALEXANDRIA_ADDR        listen address (default :8080)
  ALEXANDRIA_API_KEY     protect /v1/search with a bearer / X-API-Key secret
  ALEXANDRIA_RATE_LIMIT  requests/minute per client (0 disables)
  BRAVE_SEARCH_API_KEY, TAVILY_API_KEY, SERPER_API_KEY, EXA_API_KEY
  GOOGLE_CSE_API_KEY + GOOGLE_CSE_ID       provider keys (any subset)
  SEARX_URL              fixed SearXNG instance (default: public online pool)
  ALEXANDRIA_ENV_FILE    KEY=VALUE file loaded into the daemon environment
  ALEXANDRIA_BIN         explicit path to the Go binary
  ALEXANDRIA_RUNTIME_DIR pid file + log location (default ~/.alexandria)
  ALEXANDRIA_URL         base URL when the daemon runs elsewhere
`;

function parseArgs(argv) {
  const positional = [];
  const options = {};
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith('--')) {
      const eq = a.indexOf('=');
      if (eq !== -1) {
        options[a.slice(2, eq)] = a.slice(eq + 1);
        continue;
      }
      const next = argv[i + 1];
      if (next !== undefined && !next.startsWith('-')) {
        options[a.slice(2)] = next;
        i++;
      } else {
        options[a.slice(2)] = true;
      }
    } else if (a.startsWith('-') && a.length > 1) {
      const next = argv[i + 1];
      if (next !== undefined && !next.startsWith('-')) {
        options[a.slice(1)] = next;
        i++;
      } else {
        options[a.slice(1)] = true;
      }
    } else {
      positional.push(a);
    }
  }
  return { positional, options };
}

function print(value) {
  if (typeof value === 'string') {
    console.log(value);
  } else {
    console.log(JSON.stringify(value, null, 2));
  }
}

async function main(argv) {
  const [cmd, ...rest] = argv;
  const { positional, options } = parseArgs(rest);

  switch (cmd) {
    case undefined:
    case 'help':
    case '--help':
    case '-h':
      console.log(HELP);
      return;

    case 'start': {
      const r = await daemon.start();
      if (r.already) {
        console.log(`alexandria already running${r.pid ? ` (pid ${r.pid})` : ''} — ${r.url}`);
      } else {
        console.log(`alexandria is up at ${r.url} (pid ${r.pid})`);
        console.log(`logs: ${logFile()}`);
      }
      return;
    }

    case 'stop': {
      const r = await daemon.stop();
      console.log(r.stopped ? `alexandria stopped (pid ${r.pid})` : 'alexandria is not running');
      return;
    }

    case 'restart': {
      await daemon.stop();
      const r = await daemon.start();
      console.log(`alexandria restarted — ${r.url} (pid ${r.pid})`);
      return;
    }

    case 'status': {
      const s = await daemon.status();
      if (s.running) {
        console.log(`running (pid ${s.pid}) at ${s.url} — ${s.healthy ? 'healthy' : 'NOT healthy'}`);
      } else {
        console.log(`not running (would serve at ${s.url})`);
      }
      return;
    }

    case 'logs': {
      const n = options.n ? parseInt(options.n, 10) : 100;
      console.log(daemon.tailLog(Number.isFinite(n) && n > 0 ? n : 100));
      return;
    }

    case 'version': {
      const bin = ensureBinary();
      console.log(`alexandria-search npm ${pkg.version}`);
      console.log(`binary: ${bin}`);
      console.log(binaryVersion(bin) || 'binary version unknown');
      return;
    }

    case 'search': {
      const query = positional.join(' ');
      if (!query) {
        throw new Error('search requires a query, e.g. alexandria search "go 1.24 release notes"');
      }
      const result = await api.search(query, {
        maxResults: options['max-results'] ? parseInt(options['max-results'], 10) : 5,
        maxTokens: options['max-tokens'] ? parseInt(options['max-tokens'], 10) : 800,
        format: options.format || 'toon',
        url: options.url,
      });
      print(result);
      return;
    }

    default:
      throw new Error(`unknown command "${cmd}" — run \`alexandria help\``);
  }
}

module.exports = { main };
