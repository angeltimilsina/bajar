#!/usr/bin/env python3
"""Local Linux/WSL CometBFT example network; no BAJ ledger."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
DATA = ROOT / '.devnet'
CHAIN = 'bajar-consensus-demo-1'


def rpc(i, method):
    with urllib.request.urlopen(f'http://127.0.0.1:{26657 + i * 10}/{method}', timeout=3) as r:
        result = json.load(r)
    if 'error' in result:
        raise RuntimeError(str(result['error']))
    return result['result']


def binary():
    exe = shutil.which('cometbft')
    if not exe:
        raise RuntimeError('Install cometbft v0.40.0; see docs/DEVNET.md')
    # The v0.40.0 module retains a 0.39.0 CLI fallback string. Verify build
    # provenance instead of accepting or rejecting a binary by that label.
    info = subprocess.check_output(['go', 'version', '-m', exe], text=True)
    verify_build(info)
    return exe


def verify_build(info):
    fields = [line.split() for line in info.splitlines()]
    expected = ['mod', 'github.com/cometbft/cometbft', 'v0.40.0',
                'h1:0+zc7FlcnonFfMwzhyDCd5vK/fHZqz0RfRyTCxZsFPU=']
    if expected not in fields or any(row and row[0] == '=>' for row in fields):
        raise RuntimeError('Expected unreplaced CometBFT v0.40.0 module build; install with go install as documented')
    if ['path', 'github.com/cometbft/cometbft/cmd/cometbft'] not in fields:
        raise RuntimeError('Unexpected CometBFT command module')


def run(exe, *args):
    return subprocess.check_output([exe, *args], text=True).strip()


def configure(text, index, peers):
    section = ''
    values = {
        ('', 'proxy_app'): '"persistent_kvstore"',
        ('', 'moniker'): f'"bajar-local-{index}"',
        ('rpc', 'laddr'): f'"tcp://127.0.0.1:{26657 + index * 10}"',
        ('rpc', 'pprof_laddr'): '""',
        ('p2p', 'laddr'): f'"tcp://127.0.0.1:{26656 + index * 10}"',
        ('p2p', 'external_address'): '""',
        ('p2p', 'persistent_peers'): json.dumps(peers),
        ('p2p', 'addr_book_strict'): 'false',
        ('p2p', 'allow_duplicate_ip'): 'true',
        ('p2p', 'pex'): 'false',
        ('consensus', 'timeout_commit'): '"1s"',
        ('instrumentation', 'prometheus'): 'false',
    }
    seen = set()
    output = []
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith('['):
            section = stripped.strip('[]')
        if '=' in stripped and not stripped.startswith('#'):
            key = (section, stripped.split('=', 1)[0].strip())
            if key in values:
                line = f'{key[1]} = {values[key]}'
                seen.add(key)
        output.append(line)
    if seen != set(values):
        raise RuntimeError(f'Unexpected v0.40.0 config: missing {set(values) - seen}')
    return '\n'.join(output) + '\n'


def initialize(exe):
    nodes = DATA / 'nodes'
    if nodes.exists():
        validate_genesis()
        return
    run(exe, 'testnet', '--v', '4', '--o', str(nodes), '--populate-persistent-peers=false')
    ids = [run(exe, 'show-node-id', '--home', str(nodes / f'node{i}')) for i in range(4)]
    genesis = json.loads((nodes / 'node0/config/genesis.json').read_text())
    genesis['chain_id'] = CHAIN
    for i in range(4):
        home = nodes / f'node{i}'
        (home / 'config/genesis.json').write_text(json.dumps(genesis, indent=2) + '\n')
        config = home / 'config/config.toml'
        peers = ','.join(f'{ids[j]}@127.0.0.1:{26656 + j * 10}' for j in range(4) if j != i)
        config.write_text(configure(config.read_text(), i, peers))
    for path in nodes.rglob('*'):
        path.chmod(0o700 if path.is_dir() else 0o600)
    validate_genesis()


def validate_genesis():
    docs = [(DATA / f'nodes/node{i}/config/genesis.json').read_bytes() for i in range(4)]
    if len({hashlib.sha256(d).digest() for d in docs}) != 1:
        raise RuntimeError('Genesis files differ')
    doc = json.loads(docs[0])
    vals = doc['validators']
    if doc['chain_id'] != CHAIN or len(vals) != 4 or len({v['address'] for v in vals}) != 4:
        raise RuntimeError('Expected four distinct validators on demo chain')
    if any(v['pub_key']['type'] != 'tendermint/PubKeyEd25519' or str(v['power']) != '1' for v in vals):
        raise RuntimeError('Expected equal-power Ed25519 demo validators')
    addresses = set()
    for i in range(4):
        key = json.loads((DATA / f'nodes/node{i}/config/priv_validator_key.json').read_text())
        addresses.add(key['address'])
    if addresses != {v['address'] for v in vals}:
        raise RuntimeError('Validator identities do not match genesis')


def owned(pid, i):
    try:
        args = Path(f'/proc/{pid}/cmdline').read_bytes().split(b'\0')
        return str(DATA / f'nodes/node{i}').encode() in args and b'node' in args
    except FileNotFoundError:
        return False


def stop():
    file = DATA / 'processes.json'
    if not file.exists():
        return
    pids = json.loads(file.read_text())
    for i, pid in enumerate(pids):
        if owned(pid, i):
            os.kill(pid, signal.SIGTERM)
    deadline = time.monotonic() + 15
    while any(owned(pid, i) for i, pid in enumerate(pids)):
        if time.monotonic() > deadline:
            raise RuntimeError('Nodes did not stop; inspect logs before restarting')
        time.sleep(.2)
    file.unlink()
    print('Stopped local nodes; keys, signing state and databases preserved.')


def snapshot():
    rows = []
    for i in range(4):
        status = rpc(i, 'status')
        peers = rpc(i, 'net_info')
        validators = rpc(i, 'validators?per_page=100')
        if status['node_info']['network'] != CHAIN or status['sync_info']['catching_up']:
            raise RuntimeError(f'node{i} not ready on expected chain')
        if int(peers['n_peers']) != 3 or int(validators['total']) != 4:
            raise RuntimeError(f'node{i}: expected 3 peers and 4 validators')
        rows.append((int(status['sync_info']['latest_block_height']), status['node_info']['id']))
    if len({row[1] for row in rows}) != 4:
        raise RuntimeError('Duplicate node identities')
    return [row[0] for row in rows]


def smoke(timeout=60):
    validate_genesis()
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            first = snapshot()
            time.sleep(3)
            second = snapshot()
            if all(b > a > 0 for a, b in zip(first, second)):
                height = min(second)
                commits = [rpc(i, f'commit?height={height}')['signed_header'] for i in range(4)]
                hashes = {c['commit']['block_id']['hash'] for c in commits}
                if len(hashes) != 1 or any(sum(s['block_id_flag'] == 2 for s in c['commit']['signatures']) < 3 for c in commits):
                    raise RuntimeError('Missing common finalized commit with >2/3 signatures')
                print(f'PASS: four nodes, three peers each, four validators; heights {first} -> {second}; common commit {height}')
                return
        except (OSError, ValueError, KeyError, RuntimeError) as exc:
            last = exc
        time.sleep(1)
    raise RuntimeError(f'Devnet smoke test timed out: {last}')


def start():
    exe = binary()
    file = DATA / 'processes.json'
    if file.exists():
        raise RuntimeError('Process record exists; run stop before start')
    initialize(exe)
    sockets = []
    try:
        for i in range(4):
            for port in (26656 + i * 10, 26657 + i * 10):
                sock = socket.socket()
                # Match server restart semantics when prior connections are in TIME_WAIT.
                sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                sockets.append(sock)
                sock.bind(('127.0.0.1', port))
    finally:
        for sock in sockets:
            sock.close()
    pids = []
    try:
        for i in range(4):
            with (DATA / f'node{i}.log').open('ab') as log:
                proc = subprocess.Popen([exe, 'node', '--home', str(DATA / f'nodes/node{i}')], stdout=log, stderr=log, start_new_session=True)
            pids.append(proc.pid)
            file.write_text(json.dumps(pids))
        smoke()
    except BaseException:
        stop()
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['start', 'status', 'stop', 'smoke'])
    args = parser.parse_args()
    os.umask(0o077)
    DATA.mkdir(mode=0o700, exist_ok=True)
    with (DATA / 'control.lock').open('w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if args.action == 'start':
            start()
        elif args.action == 'stop':
            stop()
        elif args.action == 'smoke':
            smoke()
        else:
            print('Finalized heights:', snapshot())


if __name__ == '__main__':
    try:
        main()
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        raise SystemExit(str(error))
