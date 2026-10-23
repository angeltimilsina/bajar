#!/usr/bin/env python3
"""Review an allowlisted source snapshot; never walk node state or user-home Git."""
import argparse
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
FILES = ['.gitignore', 'AGENTS.md', 'README.md', 'go.mod', 'go.sum']
TREES = {'cmd': {'.go'}, 'internal': {'.go'}, 'network': {'.json'},
         'docs': {'.md'}, 'scripts': {'.py', '.sh'}, '.vscode': {'.json'},
         '.github/workflows': {'.yml', '.yaml'}}
PATTERNS = [re.compile(r'-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----'),
            re.compile(r'\b(?:AKIA|ASIA)[A-Z0-9]{16}\b'),
            re.compile(r'\bgh[pousr]_[A-Za-z0-9]{30,}\b'),
            re.compile(r'\bgithub_pat_[A-Za-z0-9_]{30,}\b'),
            re.compile(r'(?i)["\'](?:private_key|secret_access_key|api_key|password)["\']\s*:\s*["\'][A-Za-z0-9+/=_-]{24,}["\']')]


def snapshot():
    paths = [ROOT / name for name in FILES]
    for directory, suffixes in TREES.items():
        folder = ROOT / directory
        if not folder.exists():
            continue
        # Do not follow symlinks, including symlinked directories.
        for path in folder.rglob('*'):
            if path.is_symlink():
                raise ValueError(f'Symlink in publication tree: {path.relative_to(ROOT)}')
            if path.is_file() and path.suffix in suffixes and '__pycache__' not in path.parts:
                paths.append(path)
    entries = []
    for path in sorted(paths):
        name = path.relative_to(ROOT).as_posix()
        if path.is_symlink() or not path.resolve().is_relative_to(ROOT.resolve()):
            raise ValueError(f'Unsafe path: {name}')
        content = path.read_text(encoding='utf-8').replace('\r\n', '\n')
        if '\x00' in content or any(pattern.search(content) for pattern in PATTERNS):
            raise ValueError(f'Possible secret or binary data in {name}; review locally')
        raw = content.encode()
        entries.append(dict(path=name, mode='100644', type='blob', content=content,
                            sha256=hashlib.sha256(raw).hexdigest(), bytes=len(raw)))
    return entries


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--snapshot', action='store_true', help='write reviewed text snapshot under ignored .tools/')
    args = parser.parse_args()
    entries = snapshot()
    report = [{k: v for k, v in entry.items() if k != 'content'} for entry in entries]
    if args.snapshot:
        output = ROOT / '.tools/publication'
        output.mkdir(parents=True, exist_ok=True)
        (output / 'source.json').write_text(json.dumps(entries, indent=2), encoding='utf-8')
        (output / 'manifest.json').write_text(json.dumps(report, indent=2), encoding='utf-8')
    print(json.dumps(dict(files=len(entries), bytes=sum(e['bytes'] for e in entries),
                         findings=0, manifest=report), indent=2))


if __name__ == '__main__':
    main()
