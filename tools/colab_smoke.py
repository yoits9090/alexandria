"""Dependency-free Colab smoke check; no secrets are accepted on CLI."""
import os, urllib.request
print("Alexandria Colab smoke node")
print("python ok", __import__("sys").version.split()[0])
print("deepseek configured", bool(os.getenv("DEEPSEEK_API_KEY")))
