import os
from pathlib import Path
import subprocess
import socket
import tempfile
import time
import unittest
import urllib.error
import urllib.request


class CLITests(unittest.TestCase):
    def test_cli_reports_help_and_errors(self):
        Path(".test-artifacts").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=".test-artifacts") as directory:
            binary = str(Path(directory) / "epochalypse")
            subprocess.run(["go", "build", "-cover", "-coverpkg=./...", "-o", binary, "./cmd/epochalypse"], check=True)
            coverdir = Path(directory) / "coverage"
            coverdir.mkdir()
            env = dict(os.environ, GOCOVERDIR=str(coverdir.resolve()))
            for args, code, output in (([], 0, "Usage:"), (["help"], 0, "Usage:"), (["--help"], 0, "Usage:"), (["-h"], 0, "Usage:"), (["bad"], 2, "invalid command"), (["help", "bad"], 2, "invalid command")):
                result = subprocess.run([binary, *args], env=env, capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, code, result.stderr)
                self.assertIn(output, result.stdout if code == 0 else result.stderr)
                self.assertEqual(result.stderr if code == 0 else result.stdout, "")
            self.check_service_processes(binary, env, Path(directory))
            Path(".test-artifacts").mkdir(exist_ok=True)
            subprocess.run(["go", "tool", "covdata", "textfmt", f"-i={coverdir}", "-o=.test-artifacts/cli.out"], check=True)

    def check_service_processes(self, binary, env, directory):
        config = directory / "local.json"
        config.write_text('{"version":1}')
        for mode in ("server", "dns", "agent"):
            with self.subTest(mode=mode):
                with socket.socket() as reservation:
                    reservation.bind(("127.0.0.1", 0))
                    port = reservation.getsockname()[1]
                state = directory / mode
                process_env = dict(env, EPOCHALYPSE_CONFIG=str(config), EPOCHALYPSE_HTTP_ADDR=f"127.0.0.1:{port}", EPOCHALYPSE_DNS_ADDR="127.0.0.1:0", EPOCHALYPSE_STATE_DIR=str(state), EPOCHALYPSE_AGENT_ID="lab-agent")
                process = subprocess.Popen([binary, mode], env=process_env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    self.wait_for_health(process, port)
                    self.assertTrue((state / "v1").is_dir())
                    result = subprocess.run([binary, mode], env=process_env, capture_output=True, text=True, timeout=5)
                    self.assertEqual(result.returncode, 1, result.stderr)
                    self.assertIn("bind HTTP address", result.stderr)
                finally:
                    process.terminate()
                    stdout, stderr = process.communicate(timeout=5)
                self.assertEqual(process.returncode, 0, stderr)
                self.assertEqual(stdout, "")
                with socket.socket() as connection:
                    self.assertNotEqual(connection.connect_ex(("127.0.0.1", port)), 0)
        result = subprocess.run([binary, "server", "--config", str(directory / "missing.json")], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 1)
        self.assertIn("open local configuration", result.stderr)

    def wait_for_health(self, process, port):
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            self.assertIsNone(process.poll(), "service exits before health check")
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{port}/healthz", timeout=1) as response:
                    self.assertEqual(response.status, 200)
                    self.assertEqual(response.read(16), b"ok\n")
                    return
            except urllib.error.URLError:
                time.sleep(0.02)
        self.fail("service health check times out")


if __name__ == "__main__":
    unittest.main()
