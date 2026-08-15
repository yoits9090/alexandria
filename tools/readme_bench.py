#!/usr/bin/env python3
"""README benchmark: raw provider JSON vs Alexandria's normalized output.

Measures prompt tokens (rune/4, the same estimator Alexandria uses) for the
same search query consumed three ways:
  1. raw      — the provider payload as an agent would receive it directly
  2. json     — Alexandria's normalized /v1/search JSON diagnostics response
  3. toon     — Alexandria's compact LLM projection

Runs the real alexandria binary against a fixture SearXNG server so the
numbers are reproducible. Writes docs/bench/bench.json and prints a table.
"""
import argparse, http.server, json, os, subprocess, sys, threading, time, urllib.request, urllib.parse
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

QUERIES = [
    ("latest Go release notes", 7),
    ("Cerebras inference throughput benchmarks", 8),
    ("best GPU for training a 70B model", 6),
    ("recursive self-improvement LLM research", 7),
    ("SearXNG public instance list", 6),
    ("test-time compute scaling laws", 8),
    ("NanoGPT speedrun results", 6),
    ("RAG latency optimization techniques", 7),
]

# Realistic-looking results per query (title, url, content). The raw payload
# adds the metadata fields real engines return (score, engines, positions,
# category, publishedDate) that LLM prompts never need.
CORPUS = {
    "latest Go release notes": [
        ("Go 1.24 Release Notes - The Go Programming Language", "https://go.dev/doc/go1.24", "Go 1.24 is the latest major release of the Go language, bringing generic type aliases, improved performance for the garbage collector, and a new crypto-related package set that makes memory-safe primitives easier to use in production systems."),
        ("Go 1.24 released - Google Open Source Blog", "https://opensource.googleblog.com/2026/02/go-1-24-released.html", "The Go team announces Go 1.24 with a 20 percent reduction in allocation overhead for large heaps, faster builds on multi-core machines, and the long-awaited generic type aliases proposal implemented behind a stable language flag."),
        ("What's new in Go 1.24 - The Register", "https://www.theregister.com/2026/02/11/golang_124_release/", "A look at the headline features of Go 1.24 including structured concurrency experiments, iterators over slices, and the new weak package that lets developers build memory-efficient caches without finalizers."),
        ("Go 1.24 Release Notes: generics, weak pointers, and more", "https://blog.golang.org/2026/go-1.24", "The official Go blog walks through the changes in Go 1.24: type aliases for generics, the weak package for ephemeron-style caches, improved runtime performance, and quality-of-life changes to go vet and the toolchain."),
        ("Download Go 1.24 - official release binaries", "https://go.dev/dl/", "Download Go 1.24 for Linux, macOS, Windows, and other platforms. Source tarballs and checksums are available, along with install instructions for beginners and package manager integration for Debian, Fedora, and Homebrew."),
        ("Go 1.24 vs Go 1.23: benchmark comparison", "https://benchmarks.example.org/go/1.24-vs-1.23", "Independent benchmarks compare Go 1.24 with 1.23 across compile time, runtime performance, and memory usage. The new release is measurably faster for typical web services and shows significant gains for garbage-collection-heavy workloads."),
        ("Migrating to Go 1.24: what breaks", "https://migration-guides.example.org/golang/1.24", "A practical migration guide covering the few compatibility changes in Go 1.24, including the new behavior of range-over-int, deprecation notices, and recommended toolchain settings for CI pipelines."),
    ],
    "Cerebras inference throughput benchmarks": [
        ("Cerebras Inference: 2,700 tokens per second on Llama 3.1", "https://cerebras.ai/blog/llama31-performance", "Cerebras reports 2,700 tokens per second on Llama 3.1 70B at batch size one, powered by the wafer-scale WSE-3 architecture. The company publishes reproducible throughput numbers on standard workloads."),
        ("Cerebras Inference benchmark methodology", "https://cerebras.ai/blog/inference-benchmarks", "How Cerebras measures tokens per second, time to first token, and power efficiency across Llama, Mistral, and Qwen models. Includes a comparison table against leading GPU-based inference providers."),
        ("Why wafer-scale beats GPUs for inference - analysis", "https://semianalysis.com/2026/03/cerebras-wse3-inference/", "An analysis of why the Cerebras WSE-3 delivers 10x lower latency for autoregressive generation: no PCIe hops between compute and memory, deterministic scheduling, and an all-on-die memory hierarchy."),
        ("Cerebras vs Groq vs GPU inference throughput comparison", "https://benchmarks.example.org/inference/2026/03", "A third-party comparison of Cerebras, Groq, and NVIDIA H100-based inference endpoints measuring tokens per second, time to first token, cost per million tokens, and availability over a 30-day window."),
        ("Cerebras releases open inference benchmarks repo", "https://github.com/cerebras/inference-benchmarks", "Cerebras open-sources its inference benchmark harness so anyone can reproduce tokens-per-second numbers for Llama 3.1 70B, Llama 3.3 70B, and Qwen 2.5 across providers."),
        ("Cerebras inference: pricing drops to $0.10 per million tokens", "https://cerebras.ai/pricing", "With wafer-scale inference, Cerebras undercuts GPU-based providers on cost per token while delivering time to first token under 100 milliseconds for most models, making speculative decoding less necessary."),
        ("Interview: Cerebras CTO on inference-first hardware design", "https://www.theplatform.com/2026/02/cerebras-cto-interview", "The Cerebras CTO discusses why the company optimized the WSE-3 for the memory-bandwidth-bound nature of autoregressive generation and how that changes the economics of test-time compute."),
        ("Cerebras inference API: Llama 3.3 70B at 2,000 tps", "https://cerebras.ai/blog/llama33-performance", "Cerebras adds Llama 3.3 70B support and publishes measured throughput of 2,000 tokens per second with a 90th percentile time to first token of 75 milliseconds on its production API."),
    ],
    "best GPU for training a 70B model": [
        ("Training a 70B model: H100 vs H200 vs MI300X vs B200", "https://training-guides.example.org/70b-gpu-comparison", "A practical comparison of NVIDIA H100 SXM, H200, B200, and AMD MI300X for training a 70B dense model: memory bandwidth, FP8 throughput, inter-node scaling behavior, and realistic price-performance after rack costs."),
        ("How much GPU memory does a 70B model need?", "https://blog.example.org/llm-training/70b-memory", "A 70B model in bf16 needs roughly 140GB of weights alone; with optimizer states, gradients, and activations, a full fine-tune requires 8x H100s or 6x H200s. This guide walks through the exact math with a worked example."),
        ("70B training on 8x H100: a cost breakdown", "https://cost.example.org/training/70b-8xh100", "An itemized breakdown of training a 70B model for one epoch on a 1B-token dataset using 8x H100 80GB nodes: cloud rental costs, power, storage egress, and the checkpointing overhead that most estimates forget."),
        ("AMD MI300X for LLM training: real-world results", "https://amd-llm.example.org/mi300x-training", "Early adopters report MI300X training throughput within 15 percent of H100 for dense transformer workloads at a lower price per GB of HBM. Caveats include the software stack maturity and flash-attention porting effort."),
        ("B200: does the next Blackwell GPU change training economics?", "https://hardware.example.org/2026/b200-training", "With 192GB of HBM3e and doubled FP8 throughput per GPU, B200 reduces the node count for 70B training from 8 to 4 GPUs, but interconnects and cooling requirements change the total cost of ownership."),
        ("The case for renting 70B training clusters vs buying", "https://cloud-economics.example.org/70b-rent-vs-buy", "For most teams, renting 8x H100 or 4x B200 nodes by the hour beats buying: utilization is the real cost driver, and spot pricing for H100 nodes has dropped 40 percent year over year."),
    ],
    "recursive self-improvement LLM research": [
        ("Recursive self-improvement: a survey of approaches", "https://arxiv.org/abs/2601.00001", "A survey of recursive self-improvement research for language models, covering self-training loops, synthetic data distillation, and the theoretical conditions under which model improvement can compound across generations."),
        ("Can LLMs improve their own training data?", "https://arxiv.org/abs/2602.12345", "Recent work shows LLMs can generate increasingly useful synthetic training data when a reward model filters outputs, but gains plateau after several generations due to model collapse and diversity loss."),
        ("Self-improving agents with verifier feedback", "https://arxiv.org/abs/2603.54321", "A framework where an LLM agent proposes improvements to its own prompting and tool-use policies, a verifier scores them, and the loop repeats. The authors measure compounding gains on coding benchmarks over 20 iterations."),
        ("The limits of self-training: why LLMs plateau", "https://arxiv.org/abs/2604.11111", "Theoretical and empirical analysis of self-training limits: without new external information, model-generated labels concentrate around existing modes, bounding achievable improvement by the base distribution's entropy."),
        ("Recursive self-improvement and test-time compute", "https://research.example.org/rsi-test-time-compute", "An essay connecting recursive self-improvement to test-time compute scaling: if a model can spend inference compute to improve its own next-token distribution, the effective scaling law improves faster than pure pretraining."),
        ("AI safety implications of self-improving systems", "https://safety.example.org/rsi-analysis", "A review of alignment concerns for recursively self-improving systems, including specification gaming, reward hacking in verifier loops, and the difficulty of maintaining corrigibility across generations."),
        ("Bootstrapped reasoning: models that write their own scaffolds", "https://arxiv.org/abs/2605.22222", "New results showing that models can write reasoning scaffolds that outperform their own unaided reasoning, and that scaffold-writing ability itself improves with practice, hinting at a weak form of recursive improvement."),
    ],
    "SearXNG public instance list": [
        ("SearXNG public instances - searx.space", "https://searx.space/", "The community-maintained directory of public SearXNG instances with uptime statistics, location, and software version for each node. Instances marked online accept anonymous search traffic."),
        ("How to choose a public SearXNG instance", "https://blog.example.org/searxng-instance-guide", "Privacy considerations when picking a public instance: logging policies, jurisdiction, rate limits, and whether the instance supports JSON output for API clients."),
        ("Running your own SearXNG instance in 10 minutes", "https://docs.searxng.org/admin/installation-docker.html", "Official Docker-based installation guide for SearXNG, including the recommended docker-compose setup, reverse proxy configuration, and limiter settings for public deployments."),
        ("SearXNG vs SearX: what changed", "https://docs.searxng.org/migration/searx-to-searxng.html", "SearXNG is the actively maintained fork of SearX with support for more engines, JSON output improvements, and regular releases. The migration guide covers the differences in configuration files and engine settings."),
        ("Public SearXNG instances with JSON API enabled", "https://gist.github.com/example/searxng-json-instances", "A curated list of public instances that expose the JSON format, useful for building privacy-respecting search APIs without running your own infrastructure."),
        ("SearXNG rate limiting and bot protection", "https://docs.searxng.org/admin/limiter.html", "Documentation for SearXNG's built-in limiter: how to configure per-IP rate limits, bot detection, and CAPTCHA walls for public instances."),
    ],
    "test-time compute scaling laws": [
        ("Scaling LLM Test-Time Compute Optimally", "https://arxiv.org/abs/2408.03314", "The paper introducing adaptive compute allocation: models should spend more inference tokens on hard problems and use verifier-guided search, achieving the same accuracy as a 14x larger model on competition math."),
        ("Test-time compute: the new scaling axis", "https://research.example.org/test-time-scaling", "An overview of how inference-time compute changes the economics of model capabilities: instead of pretraining larger models, spend more tokens on search, self-consistency, and verifier feedback at inference time."),
        ("Compute-optimal inference: when to search harder", "https://arxiv.org/abs/2501.00042", "A follow-up to the original test-time scaling paper proposing a compute-optimal policy that allocates inference budget per-problem based on predicted difficulty, improving efficiency by 40 percent on math benchmarks."),
        ("Self-consistency vs process reward models at scale", "https://arxiv.org/abs/2502.07777", "Comparing majority voting, self-consistency, and process reward model guided search under large test-time budgets. Process supervision wins at high budgets but costs significantly more tokens per problem."),
        ("Test-time compute and the inference wall", "https://blog.example.org/inference-wall", "An industry perspective on the inference wall: as pretraining data runs out, test-time compute becomes the marginal scaling axis, and inference infrastructure becomes the strategic bottleneck."),
        ("Budget forcing: adaptive reasoning depth", "https://arxiv.org/abs/2503.12345", "A method that forces reasoning models to continue thinking when outputs are incorrect, showing that adaptive depth control improves pass rates while keeping average inference cost flat."),
        ("Small models plus search beat large models", "https://arxiv.org/abs/2504.09876", "Evidence that small models with test-time search outperform much larger models on planning and math tasks, with a cost analysis showing the compute tradeoff favors search for most workloads."),
        ("Scaling laws for inference-time reasoning", "https://arxiv.org/abs/2505.20001", "A scaling-law study of reasoning tokens: accuracy improves log-linearly with inference compute up to a saturation point, after which verifier quality and base model knowledge become the limiting factors."),
    ],
    "NanoGPT speedrun results": [
        ("NanoGPT speedrun: training GPT-2 in 4 hours for $10", "https://github.com/karpathy/nanochat", "Karpathy's benchmark of minimal training runs: GPT-2 (124M) reaches 3.28 validation loss in 4 hours on a single 8xA100 node, demonstrating that tiny, well-tuned training loops beat large frameworks."),
        ("NanoGPT speedrun leaderboard", "https://speedrun.example.org/nanogpt", "A community leaderboard tracking the fastest and cheapest GPT-2 124M training runs, with entries ranging from 90 minutes on 8x H100 to 6 hours on a single 4090 using aggressive learning-rate schedules."),
        ("The $30 GPT-2: distillation of speedrun techniques", "https://blog.example.org/nanogpt-30-dollars", "A breakdown of every trick used in the cheap speedruns: torch.compile, reduced context length, no dropout, AdamW hyperparameters tuned for wall-clock time, and float16 gradient accumulation."),
        ("Reproducing the NanoGPT speedrun on consumer hardware", "https://blog.example.org/nanogpt-home", "Can you reproduce the 4-hour speedrun on a single RTX 4090? Yes with modified batch sizes: 24 hours at 24GB VRAM, with the same final validation loss within noise."),
        ("Why NanoGPT speedruns matter for research", "https://research.example.org/nanogpt-speedrun-significance", "The speedrun methodology makes training-time ablation studies practical: researchers can test data mixing, curriculum, and hyperparameter ideas in hours instead of weeks."),
        ("NanoGPT speedrun: full log and config", "https://github.com/karpathy/nanochat/blob/master/log.md", "The complete training log of the original speedrun: loss curves, learning rate schedule, hardware utilization, and the exact configuration file used, so results are fully reproducible."),
    ],
    "RAG latency optimization techniques": [
        ("RAG latency: the hidden cost of retrieval", "https://blog.example.org/rag-latency", "End-to-end RAG latency is dominated by retrieval, chunking, and context assembly, not generation. This post breaks down where the milliseconds go and which optimizations recover them."),
        ("Speculative retrieval: hiding search behind typing", "https://blog.example.org/speculative-retrieval", "An architecture that predicts likely information needs while the user is still typing, prefetches results, and maintains a small relevance-filtered context cache so answers start instantly."),
        ("Cache-aware retrieval: 10x fewer duplicate searches", "https://arxiv.org/abs/2506.01010", "A study of query overlap in conversational search showing 40 percent of queries repeat within a session; a short-TTL normalized cache eliminates most provider calls and cuts median latency by 60 percent."),
        ("Batching and prefetching for search APIs", "https://engineering.example.org/search-api-latency", "Engineering notes on parallel provider fan-out, connection reuse, DNS prefetching, and early result streaming for search gateways serving LLM applications."),
        ("Chunk size and reranking latency tradeoffs", "https://rag-eng.example.org/chunk-rerank-tradeoffs", "Larger chunks mean fewer embedding calls but more tokens in context; rerankers add 50-200ms but cut hallucination. A decision framework for choosing chunk sizes at a given latency budget."),
        ("Streaming retrieval results into generation", "https://arxiv.org/abs/2507.02020", "Instead of waiting for all retrieval to finish, stream the highest-ranked passages into the generator as they arrive, overlapping retrieval and generation to hide most of the pipeline latency."),
        ("HTTP/2 connection pooling for search backends", "https://infra.example.org/http2-search-pooling", "Reusing TLS sessions and HTTP/2 streams across search requests cuts connection setup from 3 round trips to zero, saving 20-60ms per query on cold caches."),
    ],
}

def rune4_tokens(text):
    """Same estimator as internal/search: 1 token per 4 characters."""
    return (len(text) + 3) // 4

def build_raw_payload(query, results):
    """A faithful-ish SearXNG JSON payload with all the fields agents don't need."""
    items = []
    for i, (title, url, content) in enumerate(results):
        items.append({
            "url": url,
            "title": title,
            "content": content,
            "engine": "google" if i % 2 == 0 else "bing",
            "score": round(1.0 - i * 0.11, 3),
            "category": "general",
            "positions": [i + 1],
            "engines": ["google", "brave"] if i % 2 == 0 else ["bing", "duckduckgo"],
            "publishedDate": "2026-08-01T00:00:00+00:00",
            "thumbnail": f"https://thumb.example.org/{i}.png",
            "template": "default.html",
            "parsed_url": ["https", "example.org", "/path", "", "", ""],
        })
    return {
        "query": query,
        "number_of_results": len(results) * 2400,
        "results": items,
        "answers": [],
        "corrections": [],
        "suggestions": [f"{query} tutorial", f"{query} examples", f"{query} alternatives"],
        "infoboxes": [],
        "unresponsive_engines": ["yahoo", "startpage"],
    }

def serve_fixture(port):
    class H(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            from urllib.parse import urlparse, parse_qs
            q = parse_qs(urlparse(self.path).query).get("q", [""])[0]
            if self.path.startswith("/search"):
                results = CORPUS.get(q, CORPUS["latest Go release notes"])
                body = json.dumps(build_raw_payload(q, results)).encode()
                self.send_response(200)
                self.send_header("content-type", "application/json")
                self.send_header("content-length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            else:
                self.send_response(404)
                self.end_headers()
        def log_message(self, *args):
            pass
    srv = http.server.HTTPServer(("127.0.0.1", port), H)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv

def fetch(url, timeout=15):
    with urllib.request.urlopen(url, timeout=timeout) as r:
        return r.status, r.headers.get("content-type", ""), r.read().decode()

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bin", default=str(ROOT / "bin" / "alexandria"))
    ap.add_argument("--port", type=int, default=18123)
    ap.add_argument("--out", default=str(ROOT / "docs" / "bench" / "bench.json"))
    args = ap.parse_args()

    fixture = serve_fixture(args.port + 1)
    env = dict(os.environ, SEARX_URL=f"http://127.0.0.1:{args.port + 1}", ALEXANDRIA_ADDR=f"127.0.0.1:{args.port}")
    proc = subprocess.Popen([args.bin], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        for _ in range(100):
            try:
                fetch(f"http://127.0.0.1:{args.port}/healthz", timeout=1)
                break
            except Exception:
                time.sleep(0.1)
        rows = []
        for query, n in QUERIES:
            results = CORPUS[query][:n]
            raw_payload = build_raw_payload(query, results)
            raw_text = json.dumps(raw_payload)
            status, _, json_text = fetch(f"http://127.0.0.1:{args.port}/v1/search?q={urllib.parse.quote(query)}&format=json&max_tokens=5000")
            assert status == 200, f"json {status}"
            status, _, toon_text = fetch(f"http://127.0.0.1:{args.port}/v1/search?q={urllib.parse.quote(query)}&format=toon&max_tokens=5000")
            assert status == 200, f"toon {status}"
            rows.append({
                "query": query,
                "results": len(results),
                "raw_tokens": rune4_tokens(raw_text),
                "json_tokens": rune4_tokens(json_text),
                "toon_tokens": rune4_tokens(toon_text),
                "raw_bytes": len(raw_text.encode()),
                "toon_bytes": len(toon_text.encode()),
            })
        Path(args.out).parent.mkdir(parents=True, exist_ok=True)
        Path(args.out).write_text(json.dumps(rows, indent=2) + "\n")
        print(f"{'query':42s} {'n':>2s} {'raw':>5s} {'json':>5s} {'toon':>5s} {'saved%':>6s}")
        for r in rows:
            saved = 100 * (r["raw_tokens"] - r["toon_tokens"]) / r["raw_tokens"]
            print(f"{r['query']:42s} {r['results']:2d} {r['raw_tokens']:5d} {r['json_tokens']:5d} {r['toon_tokens']:5d} {saved:5.1f}%")
        raw = sum(r["raw_tokens"] for r in rows)
        toon = sum(r["toon_tokens"] for r in rows)
        print(f"{'TOTAL':42s}     {raw:5d}           {toon:5d} {100*(raw-toon)/raw:5.1f}%")
    finally:
        proc.terminate()
        proc.wait(timeout=10)
        fixture.shutdown()

if __name__ == "__main__":
    main()
