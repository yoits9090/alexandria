import json, os, subprocess, sys, threading, time
from http.server import BaseHTTPRequestHandler, HTTPServer

class Fixture(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path.startswith('/search'):
            body = json.dumps({"results": [{"title": "Remote fixture", "url": "https://example.test/remote", "content": "Remote Alexandria result", "score": 1.0}]}).encode()
            self.send_response(200); self.send_header("content-type", "application/json"); self.send_header("content-length", str(len(body))); self.end_headers(); self.wfile.write(body)
        else: self.send_response(404); self.end_headers()
    def log_message(self, *args): pass

fixture = HTTPServer(("127.0.0.1", 0), Fixture)
threading.Thread(target=fixture.serve_forever, daemon=True).start()
env = dict(os.environ, SEARX_URL=f"http://127.0.0.1:{fixture.server_port}", ALEXANDRIA_ADDR="127.0.0.1:18080")
server = subprocess.Popen(["/content/alexandria-linux-amd64"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
try:
    time.sleep(0.5)
    result = subprocess.run([sys.executable, "/content/jcode.py", "remote fixture", "--search-url", "http://127.0.0.1:18080/v1/search", "--no-llm"], capture_output=True, text=True, timeout=20)
    print(result.stdout)
    if result.returncode != 0 or "Remote fixture" not in result.stdout: raise SystemExit("remote smoke failed: " + result.stderr)
    print("REMOTE_SMOKE_OK")
finally:
    server.terminate(); server.wait(timeout=5); fixture.shutdown()
