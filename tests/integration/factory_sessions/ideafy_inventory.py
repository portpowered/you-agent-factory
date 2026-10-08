"""Small prebuilt-artifact witness. Run with --binary <you>; never builds."""

import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import time
from urllib.request import urlopen


def verify(binary):
    helper = Path('factory/scripts/ideafy-read.py').resolve()
    print('artifact sha256=' + hashlib.sha256(binary.read_bytes()).hexdigest())
    with tempfile.TemporaryDirectory(prefix='ideafy-inventory-') as directory:
        root = Path(directory)
        factory = root / 'factory'
        factory.mkdir()
        (factory / 'factory.json').write_text(
            '{"name":"inventory-integration","workTypes":[],"workers":[],"workstations":[]}',
            encoding='utf-8')
        home = root / 'home'
        home.mkdir()
        env = dict(os.environ, HOME=str(home), USERPROFILE=str(home), YOU_NO_BROWSER_OPEN='1')
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            port = listener.getsockname()[1]
        upstream = f'http://127.0.0.1:{port}/factory-sessions?scope=all'
        with (root / 'daemon.log').open('wb') as log:
            daemon = subprocess.Popen([str(binary), 'run', '--dir', str(factory),
                                       '--continuously', '--with-server', '--listen',
                                       f'127.0.0.1:{port}', '--no-record'],
                                      cwd=root, env=env, stdout=log, stderr=log)
            try:
                deadline = time.monotonic() + 30
                while True:
                    try:
                        with urlopen(upstream, timeout=1) as response:
                            response.read()
                        break
                    except OSError:
                        if daemon.poll() is not None or time.monotonic() >= deadline:
                            raise RuntimeError('production server unavailable: ' +
                                               (root / 'daemon.log').read_text(errors='replace'))
                        # Readiness-only ceiling, never used to assert latency.
                        time.sleep(0.05)
                for exhausted in (False, True):
                    requests = []
                    forwarded = []

                    class Fixture(BaseHTTPRequestHandler):
                        def do_GET(self):
                            requests.append(self.path)
                            if len(requests) == 1 or exhausted:
                                self.send_response(503)
                                self.end_headers()
                                self.wfile.write(b'planted-secret')
                            else:
                                with urlopen(upstream, timeout=10) as response:
                                    body = response.read()
                                forwarded.append(body)
                                self.send_response(200)
                                self.end_headers()
                                self.wfile.write(body)

                        def log_message(self, *args):
                            pass

                    fixture = ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
                    thread = threading.Thread(target=fixture.serve_forever)
                    thread.start()
                    try:
                        result = subprocess.run([sys.executable, str(helper), '--server',
                                                 f'http://127.0.0.1:{fixture.server_port}',
                                                 'session', 'inventory'],
                                                capture_output=True, timeout=90)
                    finally:
                        fixture.shutdown()
                        fixture.server_close()
                        thread.join()
                    assert requests == ['/factory-sessions?scope=all'] * 2, requests
                    assert b'planted-secret' not in result.stderr
                    assert result.returncode == int(exhausted), result.stderr
                    if exhausted:
                        assert result.stdout == b''
                        assert b'optional session inventory gap' in result.stderr
                    else:
                        assert result.stdout == forwarded[0]
                        assert isinstance(json.loads(result.stdout)['sessions'], list)
                    print(f'exhausted={exhausted} requests={requests} exit={result.returncode} '
                          f'stdout_bytes={len(result.stdout)} stderr={result.stderr.decode().strip()}')
            finally:
                subprocess.run([str(binary), '--server', f'http://127.0.0.1:{port}',
                                'server', 'stop'], env=env, cwd=root,
                               capture_output=True, timeout=10)
                try:
                    daemon.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    daemon.kill()
                    daemon.wait()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    verify(parser.parse_args().binary.resolve())
