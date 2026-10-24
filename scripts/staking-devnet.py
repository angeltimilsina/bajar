#!/usr/bin/env python3
"""Exercise the isolated six-decimal staking chain; never reuse a legacy home."""
import importlib.util
import json
from pathlib import Path
import subprocess
import time

spec = importlib.util.spec_from_file_location('ledger_devnet', Path(__file__).with_name('ledger-devnet.py'))
net = importlib.util.module_from_spec(spec)
spec.loader.exec_module(net)
net.DATA = net.ROOT / f'.staking-devnet-{time.time_ns()}'
net.RECORD = net.DATA / 'processes.json'
net.INIT_COMMAND = 'init-staking-devnet'
net.CHAIN_PREFIX = 'bajar-staking-local-'
net.EXPECTED_SUPPLY = '108000000000000000'


def run():
    if net.RECORD.exists():
        raise RuntimeError('Stop existing staking devnet first')
    try:
        net.start()
        addresses = json.loads((net.DATA/'wallets/addresses.json').read_text())
        alice, bob = addresses['alice'], addresses['bob']
        key = str(net.DATA/'wallets/alice.key')

        def cli(kind, *args):
            return json.loads(subprocess.check_output([str(net.EXE), kind, '--key', key, *args], text=True))

        def stake():
            return net.query(0, '/staking', alice)

        before = net.query(0, '/account', alice)
        cli('stake', '--amount', '1000000')  # 1 BAJ, consensus power stays independent
        cli('transfer', '--to', bob, '--amount', '1000000')
        assert int(stake()['claimable']) >= 1
        cli('claim')
        cli('unstake', '--amount', '1000000')
        view = stake()
        release = view['position']['unbonding'][-1]['release_height']
        rejected = subprocess.run([str(net.EXE), 'withdraw', '--key', key], capture_output=True)
        assert rejected.returncode != 0, 'Early unstake withdrawal accepted'
        deadline = time.monotonic() + 180
        while int(net.rpc(0, 'status')['sync_info']['latest_block_height']) < release:
            if time.monotonic() > deadline:
                raise RuntimeError('Staking network stopped before unbonding matured')
            time.sleep(1)
        cli('withdraw')
        for i in range(4):
            assert net.query(i, '/account', bob)['balance'] == '1000000'
            state = net.query(i, '/state')
            assert state['total_bonded'] == '0'
            assert state['accounts']['pool:staking']['balance'] == '0'
            assert sum(int(a['balance']) for a in state['accounts'].values()) == 108000000000000000
            assert state['accounts']['pool:separate_reserve']['balance'] == '8000000000000000'
            assert state['accounts']['pool:former_staking_reserve']['balance'] == '20000000000000000'
        after = net.query(0, '/account', alice)
        assert int(after['balance']) < int(before['balance'])  # transfer plus net fees; no mint
        net.stop()
        net.start()
        assert net.query(0, '/account', alice) == after
        assert stake()['position']['bonded'] == '0'
        assert stake()['position']['unbonding'] == []
        print('PASS: six decimals, optional staking, fee-only rewards, claim, unbonding lock, withdrawal, supply and restart')
    finally:
        net.stop()


if __name__ == '__main__':
    run()
