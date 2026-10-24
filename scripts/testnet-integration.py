#!/usr/bin/env python3
"""Five isolated Linux network namespaces: four validators and one non-signing RPC node.

Requires root and iproute2. No cloud hosts, default routes or public listeners
are created. Test homes/keys are retained under ignored .testnet-integration-*.
"""
import argparse
import base64
from datetime import datetime, timedelta, timezone
import fcntl
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[1]
EXE = ROOT / '.tools/bin/bajard'


def request(payload):
    data = json.dumps(payload).encode()
    req = urllib.request.Request('http://127.0.0.1:26657', data=data,
                                 headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=3) as response:
        result = json.load(response)
    if 'error' in result:
        raise RuntimeError(result['error'])
    return result['result']


def run(protocol=1):
    if os.geteuid() != 0:
        raise SystemExit('Run with sudo (network namespace administration only); build the binary first.')
    if not EXE.is_file():
        raise SystemExit('Build first: bash scripts/build-ledger.sh')
    os.umask(0o077)
    decimals = 6 if protocol == 2 else 9
    fee = 1 if protocol == 2 else 1000
    chain_id = 'bajar-testnet-2' if protocol == 2 else 'bajar-testnet-1'
    client_args = ['--profile', 'testnet', '--chain-id', chain_id, '--rpc', 'http://127.0.0.1:26657']
    tag = uuid.uuid4().hex[:8]
    bridge = 'bjb' + tag
    switch = f'bajar-{tag}-switch'
    names = [f'bajar-{tag}-{i}' for i in range(5)]
    subnet = 1 + int(tag[:2], 16) % 250
    homes = ROOT / f'.testnet-integration-{tag}'
    homes.mkdir(mode=0o700)
    namespaces, links, processes = [], [], []

    def command(args, **kw):
        return subprocess.run(args, check=True, text=True, **kw)

    def ip(*args):
        command(['ip', *args], stdout=subprocess.DEVNULL)

    def cli(index, *args):
        prefix = ['ip', 'netns', 'exec', names[index]] if index is not None else []
        return subprocess.check_output([*prefix, str(EXE), *args], text=True, timeout=40).strip()

    def rpc(index, method, params=None):
        payload = json.dumps(dict(jsonrpc='2.0', id=1, method=method, params=params or {}))
        out = subprocess.check_output(['ip', 'netns', 'exec', names[index], 'python3',
                                       str(Path(__file__).resolve()), '--rpc-call', payload],
                                      text=True, stderr=subprocess.PIPE, timeout=8)
        return json.loads(out)

    def query(index, address):
        r = rpc(index, 'abci_query', {'path': '/account', 'data': address.encode().hex()})['response']
        assert not r.get('code', 0), r
        return json.loads(base64.b64decode(r['value']))

    def height(i):
        return int(rpc(i, 'status')['sync_info']['latest_block_height'])

    def wait_for(check, description, timeout=90):
        end = time.monotonic() + timeout
        last = None
        while time.monotonic() < end:
            try:
                if check():
                    return
            except (OSError, ValueError, RuntimeError, KeyError, subprocess.SubprocessError) as exc:
                last = exc
            time.sleep(.5)
        raise RuntimeError(f'{description}: {last}; inspect {homes}/node*.log')

    def launch(i):
        log = (homes / f'node{i}.log').open('ab')
        try:
            process = subprocess.Popen(['ip', 'netns', 'exec', names[i], str(EXE), 'start',
                                        '--home', str(homes/f'node{i}'), '--profile', 'testnet'],
                                       stdout=log, stderr=log)
        finally:
            log.close()
        processes.append(process)
        return process

    def stop_process(process):
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=30)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=10)
                raise RuntimeError('Node required forced shutdown; inspect retained logs/state')

    try:
        # Put the L2 switch in its own namespace too: host Docker/firewall
        # forwarding policy must not affect the simulation or be changed by it.
        ip('netns', 'add', switch)
        namespaces.append(switch)
        ip('-n', switch, 'link', 'add', bridge, 'type', 'bridge')
        ip('-n', switch, 'link', 'set', bridge, 'up')
        identities = []
        for i, ns in enumerate(names):
            ip('netns', 'add', ns)
            namespaces.append(ns)
            host, peer = f'bjh{tag}{i}', f'bjn{tag}{i}'
            ip('link', 'add', host, 'type', 'veth', 'peer', 'name', peer)
            links.append(host)
            ip('link', 'set', host, 'netns', switch)
            ip('-n', switch, 'link', 'set', host, 'master', bridge)
            ip('-n', switch, 'link', 'set', host, 'up')
            ip('link', 'set', peer, 'netns', ns)
            ip('-n', ns, 'link', 'set', 'lo', 'up')
            address = f'10.233.{subnet}.{i+11}'
            ip('-n', ns, 'addr', 'add', address+'/24', 'dev', peer)
            ip('-n', ns, 'link', 'set', peer, 'up')
            role = 'validator' if i < 4 else 'rpc'
            identities.append(json.loads(cli(i, 'init-testnet', '--home', str(homes/f'node{i}'),
                                              '--role', role, '--name', f'node{i}',
                                              '--advertise', address+':26656', '--chain-id', chain_id)))
        (homes/'validators.json').write_text(json.dumps(identities[:4]))
        alice = cli(None, 'wallet-new', '--key', str(homes/'alice.key'))
        bob = cli(None, 'wallet-new', '--key', str(homes/'bob.key'))
        (homes/'test-allocations.json').write_text(json.dumps({alice: str(1000 * 10**decimals)}))
        genesis = homes/'genesis.json'
        when = (datetime.now(timezone.utc)-timedelta(seconds=1)).isoformat(timespec='seconds').replace('+00:00', 'Z')
        digest = cli(None, 'assemble-genesis', '--identities', str(homes/'validators.json'),
                     '--genesis-time', when, '--test-allocations', str(homes/'test-allocations.json'),
                     '--out', str(genesis))
        assert hashlib.sha256(genesis.read_bytes()).hexdigest() == digest
        key_hashes = []
        for i in range(5):
            home = homes/f'node{i}'
            cli(i, 'install-genesis', '--home', str(home), '--genesis', str(genesis), '--sha256', digest)
            peers = home/'peers.json'
            peers.write_text(json.dumps([p for n, p in enumerate(identities) if n != i]))
            cli(i, 'configure-testnet', '--home', str(home), '--peers', str(peers))
            cli(i, 'check-testnet', '--home', str(home))
            rejected = subprocess.run([str(EXE), 'start', '--home', str(home)], capture_output=True, timeout=10)
            assert rejected.returncode != 0, 'Default local profile accepted public testnet'
            if i < 4:
                key_hashes.append(hashlib.sha256((home/'config/priv_validator_key.json').read_bytes()).hexdigest())
        assert len(set(key_hashes)) == 4
        for i in range(5):
            launch(i)
        wait_for(lambda: all(height(i) > 1 for i in range(5)), 'five-node readiness')
        assert int(rpc(4, 'status')['validator_info']['voting_power']) == 0
        before = [height(i) for i in range(5)]
        wait_for(lambda: all(height(i) >= h+2 for i, h in enumerate(before)), 'five-host block advancement')
        h = str(min(height(i) for i in range(5)))
        commits = [rpc(i, 'commit', {'height': h})['signed_header'] for i in range(5)]
        assert len({c['commit']['block_id']['hash'] for c in commits}) == 1
        assert len({c['header']['app_hash'] for c in commits}) == 1
        cli(4, 'verify-commit', *client_args,
            '--genesis', str(genesis), '--sha256', digest)
        result = json.loads(cli(4, 'transfer', *client_args,
                                '--key', str(homes/'alice.key'), '--to', bob, '--amount', str(10**decimals)))
        wait_for(lambda: all(query(i, bob)['balance'] == str(10**decimals) for i in range(5)), 'RPC transfer replication')
        for i in range(5):
            assert query(i, alice) == dict(balance=str(999 * 10**decimals-fee), nonce=1)
            assert query(i, 'pool:fees')['balance'] == str(fee)
            assert query(i, 'pool:separate_reserve')['balance'] == str(8_000_000_000 * 10**decimals)
            assert query(i, 'pool:former_staking_reserve')['balance'] == str(20_000_000_000 * 10**decimals)
        txs = rpc(4, 'block', {'height': result['height']})['block']['data']['txs']
        tx = next(t for t in txs if hashlib.sha256(base64.b64decode(t)).hexdigest().upper() == result['hash'])
        assert rpc(4, 'check_tx', {'tx': tx})['code'] != 0
        # Restart all nodes; no key, genesis or signing-state regeneration.
        for process in processes:
            stop_process(process)
        previous = [height_value for height_value in before]
        for i in range(5):
            launch(i)
        wait_for(lambda: all(height(i) > previous[i] for i in range(5)), 'restart readiness')
        assert query(4, bob)['balance'] == str(10**decimals)
        assert rpc(4, 'check_tx', {'tx': tx})['code'] != 0
        # One validator outage preserves quorum; two outages stop finalization.
        stop_process(processes[8])  # second launch of validator 3
        start_height = height(4)
        wait_for(lambda: height(4) >= start_height+2, 'three-validator quorum', timeout=40)
        assert sum(s['block_id_flag'] == 2 for s in rpc(4, 'commit')['signed_header']['commit']['signatures']) == 3
        stop_process(processes[7])  # validator 2
        time.sleep(3)  # permit a previously committed block to reach RPC
        stalled = height(4)
        time.sleep(5)
        assert height(4) == stalled, 'Chain finalized without >2/3 quorum'
        launch(2)
        wait_for(lambda: height(4) >= stalled+2, 'quorum recovery', timeout=45)
        cli(4, 'verify-commit', *client_args,
            '--genesis', str(genesis), '--sha256', digest)
        for i in range(4):
            assert hashlib.sha256((homes/f'node{i}/config/priv_validator_key.json').read_bytes()).hexdigest() == key_hashes[i]
        assert not (homes/'node4/config/priv_validator_key.json').exists()
        assert not (homes/'node4/data/priv_validator_state.json').exists()
        print('PASS: 5 isolated network namespaces; independent validators; public-only genesis; '
              'non-signing RPC; transfers; replay; matching roots; restart; outage/quorum loss/recovery')
        print(f'Retained test-only identities, logs and state: {homes}')
    finally:
        for process in processes:
            if process.poll() is None:
                try:
                    stop_process(process)
                except RuntimeError as exc:
                    print(exc)
        for ns in reversed(namespaces):
            subprocess.run(['ip', 'netns', 'del', ns], check=False)
        for link in reversed(links):
            subprocess.run(['ip', 'link', 'del', link], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--rpc-call', help=argparse.SUPPRESS)
    parser.add_argument('--protocol-version', type=int, choices=[1, 2], default=1)
    args = parser.parse_args()
    if args.rpc_call:
        print(json.dumps(request(json.loads(args.rpc_call))))
        return
    with (ROOT/'.tools/testnet-integration.lock').open('w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        run(args.protocol_version)


if __name__ == '__main__':
    main()
