from contextlib import redirect_stdout, redirect_stderr
import io
import json
import os
from pathlib import Path
import subprocess
from subprocess import CompletedProcess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

from go_tests import run


class GoTestTests(unittest.TestCase):
    def test_rejects_a_hung_subprocess(self):
        execute = subprocess.run

        def hung_command(command, **kwargs):
            return execute([sys.executable, "-c", "import time; time.sleep(2)"], **kwargs)

        errors = io.StringIO()
        start = time.monotonic()
        with patch("go_tests.TEST_TIMEOUT_SECONDS", 0.1, create=True), patch("go_tests.subprocess.run", side_effect=hung_command), redirect_stdout(io.StringIO()), redirect_stderr(errors):
            self.assertEqual(run(["./..."]), 1)
        self.assertLess(time.monotonic() - start, 1)
        self.assertIn("exceeded 0.1 seconds", errors.getvalue())
        self.assertIn("Inspect", errors.getvalue())

    def test_rejects_malformed_events_through_the_cli(self):
        for event in ("not json", "[]", '{"Test":"TestBehavior"}', '{"Action":"pass","Output":1}', '{"Action":1}', '{"Action":"pass","Test":1}', '{"Action":"unexpected"}'):
            with self.subTest(event=event), tempfile.TemporaryDirectory() as directory:
                go = Path(directory) / "go"
                go.write_text(f"#!{sys.executable}\nprint('{{\"Test\":\"TestBehavior\",\"Action\":\"pass\"}}')\nprint({event!r})\n")
                go.chmod(0o700)
                result = subprocess.run([sys.executable, "scripts/go_tests.py", "./..."], env=dict(os.environ, PATH=directory), capture_output=True, text=True, timeout=5)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("Traceback", result.stderr)
                self.assertIn("malformed event at line 2", result.stderr)
                self.assertIn("go test -json", result.stderr)

    def test_requires_executed_tests(self):
        for actions, code, expected in (([], 0, 1), (["pass"], 0, 0), (["pass"], 1, 1), (["pass", "skip"], 0, 1), (["fail"], 1, 1)):
            with self.subTest(actions=actions):
                output = "\n".join(json.dumps({"Test": "TestBehavior", "Action": action}) for action in actions)
                with patch("go_tests.subprocess.run", return_value=CompletedProcess([], code, output, "")) as execute, redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
                    self.assertEqual(run(["./..."]), expected)
                    execute.assert_called_once_with(["go", "test", "-json", "./..."], capture_output=True, text=True, timeout=120)


if __name__ == "__main__":
    unittest.main()
