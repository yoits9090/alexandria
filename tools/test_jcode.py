import json, subprocess, sys, threading, unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if "format=toon" in self.path:
            body=b"query: q\nsources[1]{id,title,url,snippet,source}:\n  1,Title,https://example.test,snippet,searx"
            self.send_response(200); self.send_header("content-type","text/toon; charset=utf-8"); self.send_header("content-length",str(len(body))); self.end_headers(); self.wfile.write(body)
        else: self.send_response(404); self.end_headers()
    def do_POST(self):
        body=json.dumps({"query":"q","results":[{"title":"Title","url":"https://example.test","snippet":"snippet","source":"searx"}]}).encode()
        self.send_response(200); self.send_header("content-type","application/json"); self.send_header("content-length",str(len(body))); self.end_headers(); self.wfile.write(body)
    def log_message(self,*args): pass
class TestHarness(unittest.TestCase):
    def test_toon_mode(self):
        server=HTTPServer(("127.0.0.1",0),Handler); threading.Thread(target=server.serve_forever,daemon=True).start()
        try:
            p=subprocess.run([sys.executable,str(ROOT/"tools/jcode.py"),"q","--search-url",f"http://127.0.0.1:{server.server_port}/v1/search","--toon","--no-llm"],capture_output=True,text=True,check=False)
            self.assertEqual(p.returncode,0);self.assertIn("sources[1]{id,title,url,snippet,source}",p.stdout)
        finally: server.shutdown(); server.server_close()
if __name__=="__main__": unittest.main()
