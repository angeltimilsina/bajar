#!/usr/bin/env python3
"""Validate the systemd template without installing or starting it."""
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
binary = root / '.tools/bin/bajard'
if not binary.exists():
    raise SystemExit('Build the ledger first')
with tempfile.TemporaryDirectory(prefix='bajar-service-') as temporary:
    unit = Path(temporary)/'bajard.service'
    # Exec paths are checked for existence by systemd-analyze; use our built
    # executable in this verification copy. Neither command is executed.
    unit.write_text((root/'deploy/systemd/bajard.service').read_text().replace(
        '/usr/local/bin/bajard', str(binary)))
    unit.chmod(0o644)
    subprocess.run(['systemd-analyze', 'verify', str(unit)], check=True)
print('PASS: systemd template verified; no service installed or started')
