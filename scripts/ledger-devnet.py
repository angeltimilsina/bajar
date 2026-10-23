#!/usr/bin/env python3
"""Control a disposable, four-node BAJ ledger devnet (Linux/WSL only)."""
import argparse
import base64
import fcntl
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
DATA = ROOT / '.ledger-devnet'
EXE = ROOT / '.tools/bin/bajard'
RECORD = DATA / 'processes.json'


def rpc(index, method, params=None):
    payload = json.dumps(dict(jsonrpc='2.0', id=1, method=method, params=params or {})).encode()
    req = urllib.request.Request(f'http://127.0.0.1:{27657 + index * 10}', data=payload,
                                 headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=30) as response:
        result = json.load(response)
    if 'error' in result:
        raise RuntimeError(result['error'])
    return result['result']


def query(index, path, address=''):
    r = rpc(index, 'abci_query', {'path': path, 'data': address.encode().hex()})['response']
    if r.get('code', 0):
        raise RuntimeError(r['log'])
    return json.loads(base64.b64decode(r['value']))


def owned(pid, index):
    try:
        args = Path(f'/proc/{pid}/cmdline').read_bytes().split(b'\0')
        return str(DATA / f'nodes/node{index}').encode() in args and b'start' in args
    except FileNotFoundError:
        return False


def stop():
    if not RECORD.exists():
        return
    pids = json.loads(RECORD.read_text())
    for index, pid in enumerate(pids):
        if owned(pid, index):
            os.kill(pid, signal.SIGTERM)
    deadline = time.monotonic() + 30
    while any(owned(pid, i) for i, pid in enumerate(pids)):
        if time.monotonic() > deadline:
            raise RuntimeError('Stop timed out; inspect logs. Signing state was preserved.')
        time.sleep(.2)
    RECORD.unlink()
    print('Stopped ledger nodes; wallets, validator signing state and databases preserved.')


def smoke():
    deadline = time.monotonic() + 90
    last = None
    while time.monotonic() < deadline:
        try:
            statuses = [rpc(i, 'status') for i in range(4)]
            before = [int(s['sync_info']['latest_block_height']) for s in statuses]
            chain = {s['node_info']['network'] for s in statuses}
            if len(chain) != 1 or not next(iter(chain)).startswith('bajar-ledger-local-'):
                raise RuntimeError('Unexpected chain')
            if any(int(rpc(i, 'net_info')['n_peers']) != 3 for i in range(4)):
                raise RuntimeError('Expected three peers each')
            time.sleep(3)
            after = [int(rpc(i, 'status')['sync_info']['latest_block_height']) for i in range(4)]
            if not all(b > a > 0 for a, b in zip(before, after)):
                raise RuntimeError('Heights did not advance')
            height = str(min(after))
            commits = [rpc(i, 'commit', {'height': height})['signed_header'] for i in range(4)]
            if len({c['commit']['block_id']['hash'] for c in commits}) != 1:
                raise RuntimeError('Committed blocks differ')
            if len({c['header']['app_hash'] for c in commits}) != 1:
                raise RuntimeError('Application roots differ')
            for i in range(4):
                subprocess.run([str(EXE), 'verify-commit', '--rpc', f'http://127.0.0.1:{27657+i*10}'], check=True)
                assert query(i, '/supply')['base_units'] == '108000000000000000000'
            print(f'PASS: ledger heights {before} -> {after}, common block and app root at {height}')
            return
        except (OSError, RuntimeError, ValueError, KeyError, subprocess.CalledProcessError) as exc:
            last = exc
            time.sleep(1)
    raise RuntimeError(f'Ledger smoke failed: {last}')


def start():
    if not EXE.exists():
        raise RuntimeError('Build first: bash scripts/build-ledger.sh')
    if RECORD.exists():
        raise RuntimeError('Process record exists; stop before restarting')
    if not DATA.exists():
        subprocess.run([str(EXE), 'init-devnet', '--dir', str(DATA)], check=True)
    pids = []
    try:
        for i in range(4):
            with (DATA / f'node{i}.log').open('ab') as log:
                process = subprocess.Popen([str(EXE), 'start', '--home', str(DATA / f'nodes/node{i}')],
                                           stdout=log, stderr=log, start_new_session=True)
            pids.append(process.pid)
            RECORD.write_text(json.dumps(pids))
        smoke()
    except BaseException:
        stop()
        raise


def e2e():
    """Transfer, forged signature, replay, conservation, restart and one-node outage."""
    if RECORD.exists():
        raise RuntimeError('Stop existing devnet before e2e')
    try:
        start()
        addresses = json.loads((DATA / 'wallets/addresses.json').read_text())
        alice, bob = addresses['alice'], addresses['bob']
        initial_alice = query(0, '/account', alice)
        initial_bob = query(0, '/account', bob)
        initial_fee = query(0, '/account', 'pool:fees')
        output = subprocess.check_output([str(EXE), 'transfer', '--key', str(DATA/'wallets/alice.key'),
                                          '--to', bob, '--amount', '1234567890'], text=True)
        result = json.loads(output)
        # Block data is available at commit; the asynchronous tx index can lag.
        block_txs = rpc(0, 'block', {'height': result['height']})['block']['data']['txs']
        tx = next(t for t in block_txs if hashlib.sha256(base64.b64decode(t)).hexdigest().upper() == result['hash'])
        # CheckTx bypasses CometBFT's duplicate cache and exercises ledger nonces.
        replay = rpc(0, 'check_tx', {'tx': tx})
        assert replay['code'] != 0, 'Replay accepted'
        forged = json.loads(base64.b64decode(tx))
        forged['nonce'] += 1
        forged['amount'] = '1'
        raw = json.dumps(forged, separators=(',', ':')).encode()
        rejection = rpc(0, 'broadcast_tx_commit', {'tx': base64.b64encode(raw).decode()})
        assert rejection['check_tx']['code'] != 0, 'Forged signature accepted'
        time.sleep(2)
        expected_alice = dict(balance=str(int(initial_alice['balance'])-1234567890-1000), nonce=initial_alice['nonce']+1)
        expected_bob = dict(balance=str(int(initial_bob['balance'])+1234567890), nonce=initial_bob['nonce'])
        for i in range(4):
            assert query(i, '/account', alice) == expected_alice
            assert query(i, '/account', bob) == expected_bob
            assert int(query(i, '/account', 'pool:fees')['balance']) == int(initial_fee['balance'])+1000
            state = query(i, '/state')
            assert sum(int(a['balance']) for a in state['accounts'].values()) == 108000000000000000000
            assert state['accounts']['pool:separate_reserve']['balance'] == '8000000000000000000'
            assert state['accounts']['pool:former_staking_reserve']['balance'] == '20000000000000000000'
        stop()
        start()
        assert query(0, '/account', alice) == expected_alice
        assert query(0, '/account', bob) == expected_bob
        replay = rpc(0, 'check_tx', {'tx': tx})
        assert replay['code'] != 0, 'Replay accepted after restart'
        pids = json.loads(RECORD.read_text())
        if owned(pids[3], 3):
            os.kill(pids[3], signal.SIGTERM)
        deadline = time.monotonic() + 30
        while owned(pids[3], 3):
            if time.monotonic() > deadline:
                raise RuntimeError('Fourth validator did not stop')
            time.sleep(.2)
        height = int(rpc(0, 'status')['sync_info']['latest_block_height'])
        deadline = time.monotonic() + 30
        while int(rpc(0, 'status')['sync_info']['latest_block_height']) < height + 2:
            if time.monotonic() > deadline:
                raise RuntimeError('Three-validator quorum stopped advancing')
            time.sleep(.5)
        commit = rpc(0, 'commit')['signed_header']['commit']
        assert sum(s['block_id_flag'] == 2 for s in commit['signatures']) == 3
        subprocess.run([str(EXE), 'verify-commit'], check=True)
        print('PASS: ML-DSA-65 transfer, forged signature/replay rejection, exact fees/supply, restart and one-validator outage')
    finally:
        stop()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['start', 'stop', 'status', 'smoke', 'e2e'])
    args = parser.parse_args()
    os.umask(0o077)
    # Lock lives outside DATA so initialization can refuse an existing directory.
    with (ROOT / '.tools/ledger-control.lock').open('w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if args.action == 'status':
            print([rpc(i, 'status')['sync_info']['latest_block_height'] for i in range(4)])
        else:
            globals()[args.action]()


if __name__ == '__main__':
    main()
