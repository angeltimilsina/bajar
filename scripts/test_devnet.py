import importlib.util
from pathlib import Path
import unittest


class ConfigTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec = importlib.util.spec_from_file_location('devnet', Path(__file__).with_name('devnet.py'))
        cls.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.module)

    def test_sections_and_local_ports(self):
        source = '''proxy_app = "tcp://127.0.0.1:26658"
moniker = "old"
[rpc]
laddr = "tcp://0.0.0.0:26657"
pprof_laddr = "localhost:6060"
[p2p]
laddr = "tcp://0.0.0.0:26656"
external_address = ""
persistent_peers = ""
addr_book_strict = true
allow_duplicate_ip = false
pex = true
[consensus]
timeout_commit = "5s"
[instrumentation]
prometheus = true
'''
        output = self.module.configure(source, 3, 'abc@127.0.0.1:26656')
        self.assertIn('laddr = "tcp://127.0.0.1:26687"', output)
        self.assertIn('laddr = "tcp://127.0.0.1:26686"', output)
        self.assertNotIn('0.0.0.0', output)
        self.assertIn('proxy_app = "persistent_kvstore"', output)
        self.assertIn('pex = false', output)

    def test_unknown_config_fails_closed(self):
        with self.assertRaises(RuntimeError):
            self.module.configure('[rpc]\nladdr = ""', 0, '')

    def test_pinned_module_provenance(self):
        info = ('binary: go1.25.0\n'
                '\tpath\tgithub.com/cometbft/cometbft/cmd/cometbft\n'
                '\tmod\tgithub.com/cometbft/cometbft\tv0.40.0\t'
                'h1:0+zc7FlcnonFfMwzhyDCd5vK/fHZqz0RfRyTCxZsFPU=\n')
        self.module.verify_build(info)
        for invalid in (info.replace('v0.40.0', 'v0.39.0'), info + '\t=>\t../local\n', info.replace('h1:', 'invalid:')):
            with self.assertRaises(RuntimeError):
                self.module.verify_build(invalid)


if __name__ == '__main__':
    unittest.main()
