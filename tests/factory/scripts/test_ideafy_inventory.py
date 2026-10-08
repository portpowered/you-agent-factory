"""Optional inventory component behavior with controlled HTTP and backoff."""

import importlib.util
import io
from http.client import IncompleteRead
from pathlib import Path
import unittest
from urllib.error import HTTPError, URLError


SCRIPT = Path(__file__).resolve().parents[3] / 'factory/scripts/ideafy-read.py'
spec = importlib.util.spec_from_file_location('inventory_read', SCRIPT)
read = importlib.util.module_from_spec(spec)
spec.loader.exec_module(read)
ARGS = ['--server', 'http://localhost:7437', 'session', 'inventory']
BODY = b'{"sessions":[],"warnings":[]}'


class Response(io.BytesIO):
    status = 200


class InventoryTest(unittest.TestCase):
    def invoke(self, outcomes, args=ARGS, jitter=lambda: 0):
        calls, delays, responses = [], [], []
        sequence = iter(outcomes)
        out, err = io.BytesIO(), io.BytesIO()

        def http(request, *, timeout):
            calls.append((request.full_url, request.get_method(), timeout))
            value = next(sequence)
            if isinstance(value, BaseException):
                raise value
            response = Response(value)
            responses.append(response)
            return response

        code = read.main(args, http=http, sleep=delays.append, jitter=jitter,
                         stdout=out, stderr=err,
                         runner=lambda *a, **kw: self.fail('inventory must not invoke CLI'))
        self.assertTrue(all(response.closed for response in responses))
        return code, out.getvalue(), err.getvalue(), calls, delays

    def test_complete_json_success_is_unchanged(self):
        code, out, err, calls, delays = self.invoke([BODY])
        self.assertEqual((code, out, err, delays), (0, BODY, b'', []))
        self.assertEqual(calls, [('http://localhost:7437/factory-sessions?scope=all', 'GET', 10)])

    def test_every_admitted_failure_retries_once_at_sixty_seconds(self):
        failures = [TimeoutError('secret'), URLError('secret'), PermissionError('secret'),
                    IncompleteRead(b'secret'), b'invalid secret', b'{}', b'{"sessions":null}']
        failures += [HTTPError('http://secret', code, 'secret', {}, None)
                     for code in (400, 401, 404, 500, 503)]
        for failure in failures:
            with self.subTest(failure=type(failure)):
                code, out, err, calls, delays = self.invoke([failure, BODY])
                self.assertEqual((code, out, delays), (0, BODY, [1]))
                self.assertEqual([call[2] for call in calls], [10, 60])
                self.assertIn(b'class=transient', err)
                self.assertNotIn(b'secret', err)

    def test_exhaustion_is_gap_without_fabricated_stdout(self):
        code, out, err, calls, delays = self.invoke([TimeoutError(), b'partial'])
        self.assertEqual((code, out, len(calls), delays), (1, b'', 2, [1]))
        self.assertIn(b'optional session inventory gap', err)
        self.assertIn(b'timeout=60s', err)

    def test_invalid_forms_never_read_or_sleep(self):
        for args in (ARGS + ['--all'], ['--server', 'file:///tmp', 'session', 'inventory'],
                     ['--server', 'http://user:secret@host', 'session', 'inventory'],
                     ['--server', 'http://host:bad', 'session', 'inventory'],
                     ['--server', 'http://host', 'work', 'move']):
            code, out, err, calls, delays = self.invoke([], args)
            self.assertEqual((code, out, calls, delays), (2, b'', [], []))

    def test_cancellation_has_no_retry(self):
        with self.assertRaises(KeyboardInterrupt):
            self.invoke([KeyboardInterrupt()])
        calls = []

        def http(*args, **kwargs):
            calls.append(kwargs['timeout'])
            raise TimeoutError()

        def sleep(delay):
            raise KeyboardInterrupt()

        with self.assertRaises(KeyboardInterrupt):
            read.main(ARGS, http=http, sleep=sleep, stdout=io.BytesIO(), stderr=io.BytesIO())
        self.assertEqual(calls, [10])

    def test_jitter_is_bounded(self):
        for value, expected in ((-1, 1), (9, 1.25)):
            self.assertEqual(self.invoke([TimeoutError(), BODY], jitter=lambda: value)[4], [expected])


if __name__ == '__main__':
    unittest.main()
