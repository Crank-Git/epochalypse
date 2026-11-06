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
    module = "github.com/Crank-Git/epochalypse"
    client = module + "/internal/switchclient"

    def test_executes_each_race_package_once(self):
        packages = [self.module + "/internal/agent", self.client, self.module + "/tests/integration"]
        output = json.dumps({"Test": "TestBehavior", "Action": "pass"})
        results = [CompletedProcess([], 0, "\n".join(packages) + "\n", ""),
                   CompletedProcess([], 0, output, ""), CompletedProcess([], 0, output, "")]
        with patch("go_tests.subprocess.run", side_effect=results) as execute, redirect_stdout(io.StringIO()):
            self.assertEqual(run(["--race-groups"]), 0)
        self.assertEqual([call.args[0] for call in execute.call_args_list], [
            ["go", "list", "./..."],
            ["go", "test", "-json", "-race", "-count=1", self.client],
            ["go", "test", "-json", "-race", "-count=1", packages[0], packages[2]],
        ])
        for call in execute.call_args_list:
            self.assertEqual(call.kwargs, dict(capture_output=True, text=True, timeout=120))

    def test_rejects_invalid_race_inventory(self):
        for inventory in ("", self.client, self.module + "/internal/agent", self.client + "\n" + self.client,
                          self.client + "\nother.example/package", self.client + "\n\n" + self.module,
                          self.client + "\n" + self.module + "/../outside", self.client + "\n " + self.module):
            with self.subTest(inventory=inventory), patch("go_tests.subprocess.run", return_value=CompletedProcess([], 0, inventory, "")) as execute, redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()) as errors:
                self.assertEqual(run(["--race-groups"]), 1)
                execute.assert_called_once()
                self.assertIn("inventory", errors.getvalue())

    def test_retains_race_inventory_failure_diagnostics(self):
        for result in (CompletedProcess([], 1, "partial inventory\n", "list error\n"),
                       subprocess.TimeoutExpired([], 120, output=b"partial inventory\n", stderr=b"list error\n")):
            with self.subTest(result=result), redirect_stdout(io.StringIO()) as output, redirect_stderr(io.StringIO()) as errors:
                with patch("go_tests.subprocess.run", **({"side_effect": result} if isinstance(result, Exception) else {"return_value": result})) as execute:
                    self.assertEqual(run(["--race-groups"]), 1)
                execute.assert_called_once_with(["go", "list", "./..."], capture_output=True, text=True, timeout=120)
                self.assertIn("partial inventory", output.getvalue())
                self.assertIn("list error", errors.getvalue())
                self.assertIn("go list ./...", errors.getvalue())

    def test_stops_after_a_race_group_fails(self):
        inventory = self.client + "\n" + self.module + "/internal/agent"
        for output, code in (("", 0), (json.dumps({"Test": "TestBehavior", "Action": "skip"}), 0),
                             (json.dumps({"Test": "TestBehavior", "Action": "fail"}), 1), ("not json", 0)):
            with self.subTest(output=output), patch("go_tests.subprocess.run", side_effect=[CompletedProcess([], 0, inventory, ""), CompletedProcess([], code, output, "")]) as execute, redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
                self.assertEqual(run(["--race-groups"]), 1)
                self.assertEqual(execute.call_count, 2)

    def test_rejects_failure_in_the_remaining_race_group(self):
        inventory = self.client + "\n" + self.module + "/internal/agent"
        passed = json.dumps({"Test": "TestBehavior", "Action": "pass"})
        with patch("go_tests.subprocess.run", side_effect=[CompletedProcess([], 0, inventory, ""), CompletedProcess([], 0, passed, ""), CompletedProcess([], 1, passed, "")]) as execute, redirect_stdout(io.StringIO()):
            self.assertEqual(run(["--race-groups"]), 1)
            self.assertEqual(execute.call_count, 3)

    def test_retains_race_group_timeout_diagnostics(self):
        inventory = self.client + "\n" + self.module + "/internal/agent"
        timeout = subprocess.TimeoutExpired([], 120, output=b"last race output\n", stderr=b"last race error\n")
        with patch("go_tests.subprocess.run", side_effect=[CompletedProcess([], 0, inventory, ""), timeout]) as execute, redirect_stdout(io.StringIO()) as output, redirect_stderr(io.StringIO()) as errors:
            self.assertEqual(run(["--race-groups"]), 1)
        self.assertEqual(execute.call_count, 2)
        self.assertIn("last race output", output.getvalue())
        self.assertIn("last race error", errors.getvalue())
        self.assertIn("exceeded 120 seconds", errors.getvalue())
        self.assertIn("go test -json -race -count=1 " + self.client, errors.getvalue())

    def test_executes_complete_groups_through_the_cli(self):
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / "commands"
            go = Path(directory) / "go"
            go.write_text(f"#!{sys.executable}\nimport json, sys\nfrom pathlib import Path\nwith Path({str(log)!r}).open('a') as output:\n    output.write(json.dumps(sys.argv[1:]) + '\\n')\nif sys.argv[1] == 'list':\n    print({self.client!r})\n    print({self.module + '/internal/agent'!r})\nelse:\n    print(json.dumps(dict(Test='TestBehavior', Action='pass')))\n")
            go.chmod(0o700)
            result = subprocess.run([sys.executable, "scripts/go_tests.py", "--race-groups"], env=dict(os.environ, PATH=directory), capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual([json.loads(line) for line in log.read_text().splitlines()], [
                ["list", "./..."],
                ["test", "-json", "-race", "-count=1", self.client],
                ["test", "-json", "-race", "-count=1", self.module + "/internal/agent"],
            ])
            self.assertEqual(result.stdout.count("Go tests: 1 passed; 0 skipped"), 2)

    def test_prepares_race_without_counting_tests(self):
        stdout = io.StringIO()
        with patch("go_tests.subprocess.run", return_value=CompletedProcess([], 0, "ok package [no tests to run]\n", "")) as execute, redirect_stdout(stdout):
            self.assertEqual(run(["--prepare-race"]), 0)
        execute.assert_called_once_with(["go", "test", "-race", "-run", "^$", "./..."], capture_output=True, text=True, timeout=120)
        self.assertIn("build-only", stdout.getvalue())
        self.assertNotIn("passed", stdout.getvalue())

    def test_rejects_race_compilation_failure(self):
        stdout = io.StringIO()
        stderr = io.StringIO()
        with patch("go_tests.subprocess.run", return_value=CompletedProcess([], 1, "build context\n", "compile error\n")), redirect_stdout(stdout), redirect_stderr(stderr):
            self.assertEqual(run(["--prepare-race"]), 1)
        self.assertIn("build context", stdout.getvalue())
        self.assertIn("compile error", stderr.getvalue())
        self.assertNotIn("passed", stdout.getvalue())

    def test_retains_race_compilation_timeout_output(self):
        stdout = io.StringIO()
        stderr = io.StringIO()
        timeout = subprocess.TimeoutExpired([], 120, output=b"compiler output\n\xff", stderr=b"compiler error\n\xe2\x82")
        with patch("go_tests.subprocess.run", side_effect=timeout), redirect_stdout(stdout), redirect_stderr(stderr):
            self.assertEqual(run(["--prepare-race"]), 1)
        self.assertIn("compiler output\n\ufffd", stdout.getvalue())
        self.assertIn("compiler error\n\ufffd", stderr.getvalue())
        self.assertIn("exceeded 120 seconds", stderr.getvalue())
        self.assertIn("go test -race -run ^$ ./...", stderr.getvalue())
        self.assertNotIn("passed", stdout.getvalue())

    def test_preserves_the_required_race_command(self):
        output = json.dumps({"Test": "TestBehavior", "Action": "pass"})
        with patch("go_tests.subprocess.run", return_value=CompletedProcess([], 0, output, "")) as execute, redirect_stdout(io.StringIO()):
            self.assertEqual(run(["-race", "-count=1", "./..."]), 0)
        execute.assert_called_once_with(["go", "test", "-json", "-race", "-count=1", "./..."], capture_output=True, text=True, timeout=120)

    def test_prepares_race_before_required_execution(self):
        for makelevel in ("0", "1"):
            with self.subTest(makelevel=makelevel):
                result = subprocess.run(["make", "--no-print-directory", "-n", "test-race"], env=dict(os.environ, MAKELEVEL=makelevel), capture_output=True, text=True, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.splitlines(), [
                    "python3 scripts/go_tests.py --prepare-race",
                    "python3 scripts/go_tests.py --race-groups",
                ])

    def test_stops_the_target_after_compilation_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / "commands"
            python = Path(directory) / "python3"
            python.write_text(f"#!{sys.executable}\nfrom pathlib import Path\nimport sys\nwith Path({str(log)!r}).open('a') as output:\n    output.write(' '.join(sys.argv[1:]) + '\\n')\nsys.exit(1)\n")
            python.chmod(0o700)
            for makelevel in ("0", "1"):
                with self.subTest(makelevel=makelevel):
                    log.write_text("")
                    result = subprocess.run(["make", "--no-print-directory", "test-race"], env=dict(os.environ, MAKELEVEL=makelevel, PATH=directory + os.pathsep + os.environ["PATH"]), capture_output=True, text=True, timeout=5)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(log.read_text().splitlines(), ["scripts/go_tests.py --prepare-race"])

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

    def test_retains_timeout_output(self):
        for output, errors, expected_output, expected_errors in (
            (b'last test output\n\xe2\x82', b'last error\n\xff', 'last test output\n\ufffd', 'last error\n\ufffd'),
            ('last test output\n', 'last error\n', 'last test output\n', 'last error\n'),
            (None, None, '', ''),
            ('{"Action":"output","Output":"last test', 'partial error', '{"Action":"output","Output":"last test', 'partial error'),
        ):
            with self.subTest(output=output, errors=errors):
                stdout = io.StringIO()
                stderr = io.StringIO()
                timeout = subprocess.TimeoutExpired(['go', 'test'], 120, output=output, stderr=errors)
                with patch('go_tests.subprocess.run', side_effect=timeout) as execute, redirect_stdout(stdout), redirect_stderr(stderr):
                    self.assertEqual(run(['-race', './internal/agent']), 1)
                self.assertEqual(stdout.getvalue(), expected_output)
                self.assertIn(expected_errors, stderr.getvalue())
                self.assertIn('exceeded 120 seconds', stderr.getvalue())
                self.assertIn('go test -json -race ./internal/agent', stderr.getvalue())
                self.assertNotIn('Traceback', stderr.getvalue())
                execute.assert_called_once_with(['go', 'test', '-json', '-race', './internal/agent'], capture_output=True, text=True, timeout=120)

    def test_rejects_timeout_after_passing_events(self):
        output = json.dumps({'Test': 'TestBehavior', 'Action': 'pass'}) + '\n'
        stdout = io.StringIO()
        stderr = io.StringIO()
        timeout = subprocess.TimeoutExpired(['go', 'test'], 120, output=output)
        with patch('go_tests.subprocess.run', side_effect=timeout), redirect_stdout(stdout), redirect_stderr(stderr):
            self.assertEqual(run(['./...']), 1)
        self.assertEqual(stdout.getvalue(), output)
        self.assertNotIn('Go tests: 1 passed', stdout.getvalue())
        self.assertIn('exceeded 120 seconds', stderr.getvalue())

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
