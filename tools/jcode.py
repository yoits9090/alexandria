#!/usr/bin/env python3
"""Small search + DeepSeek-compatible smoke harness; keys only come from env."""
import argparse, json, os, sys
from urllib.request import Request, urlopen

def post_json(url, payload, headers=None, timeout=30):
    body=json.dumps(payload).encode(); h={"content-type":"application/json", **(headers or {})}
    req=Request(url, data=body, headers=h, method="POST")
    with urlopen(req, timeout=timeout) as r: return json.load(r)

def main():
    ap=argparse.ArgumentParser(); ap.add_argument("query"); ap.add_argument("--search-url", default=os.getenv("ALEXANDRIA_SEARCH_URL","http://localhost:8080/v1/search")); ap.add_argument("--max-tokens",type=int,default=800); ap.add_argument("--no-llm",action="store_true"); ap.add_argument("--model",default=os.getenv("DEEPSEEK_MODEL","deepseek-v4-flash")); args=ap.parse_args()
    search=post_json(args.search_url,{"query":args.query,"max_results":5,"max_tokens":args.max_tokens})
    print(json.dumps(search,indent=2,ensure_ascii=False))
    key=os.getenv("DEEPSEEK_API_KEY")
    if args.no_llm or not key: return 0
    base=os.getenv("DEEPSEEK_BASE_URL","https://api.deepseek.com").rstrip("/")
    sources="\n".join(f"[{i+1}] {r['title']}\n{r['url']}\n{r.get('snippet','')}" for i,r in enumerate(search.get("results",[])))
    prompt=f"Answer the query using only these sources. Cite source numbers. If insufficient, say so.\nQuery: {args.query}\nSources:\n{sources}"
    out=post_json(base+"/chat/completions",{"model":args.model,"temperature":0,"max_tokens":400,"messages":[{"role":"system","content":"You are a concise research assistant."},{"role":"user","content":prompt}]},{"authorization":"Bearer "+key})
    print("\n--- DeepSeek answer ---\n"+out["choices"][0]["message"]["content"]); return 0
if __name__=="__main__": sys.exit(main())
