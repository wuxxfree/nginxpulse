#!/usr/bin/env python3
import hashlib
import json
import os
import sys
from pathlib import Path


def load_config():
    raw = os.getenv("CONFIG_JSON")
    if raw:
        return json.loads(raw)
    raw = os.getenv("WEBSITES")
    if raw:
        return {"websites": json.loads(raw)}
    config_path = Path(__file__).resolve().parents[1] / "configs" / "nginxpulse_config.json"
    with config_path.open("r", encoding="utf-8") as f:
        return json.load(f)


def website_id(name: str) -> str:
    digest = hashlib.md5(name.encode("utf-8")).digest()
    return digest[:2].hex()


def main() -> int:
    try:
        cfg = load_config()
    except Exception as exc:
        print(f"failed to load config: {exc}", file=sys.stderr)
        return 1

    websites = cfg.get("websites") or []
    if not websites:
        print("no websites found in config", file=sys.stderr)
        return 1

    for item in websites:
        name = (item.get("name") or "").strip()
        if not name:
            continue
        wid = website_id(name)
        print(f"{name}  ->  {wid}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
