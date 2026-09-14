#!/usr/bin/env python3
"""Load local DB configuration without evaluating shell code, then run a Go entry point."""
import os
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[1]
if len(sys.argv) < 2 or sys.argv[1] not in {'api', 'worker', 'dev-token', 'seed-demo'}:
    raise SystemExit('Usage: python3 scripts/run.py api|worker|dev-token|seed-demo [subject]')
environment = dict(os.environ)
path = root / '.env'
if path.exists():
    for line in path.read_text().splitlines():
        if line and not line.startswith('#') and '=' in line:
            key, value = line.split('=', 1)
            environment.setdefault(key, value.strip().strip('"').strip("'"))
os.chdir(root)
os.execvpe('go', ['go', 'run', './cmd/' + sys.argv[1], *sys.argv[2:]], environment)
