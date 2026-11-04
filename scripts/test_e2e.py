import os
import json
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
            self.commit = "a" * 40
            subprocess.run(["go", "build", "-cover", "-coverpkg=github.com/Crank-Git/epochalypse/...", "-ldflags", f"-X github.com/Crank-Git/epochalypse/internal/version.Commit={self.commit}", "-o", binary, "./cmd/epochalypse"], check=True)
            coverdir = Path(directory) / "coverage"
            coverdir.mkdir()
            env = dict(os.environ, GOCOVERDIR=str(coverdir.resolve()))
            for args, code, output in (([], 0, "Usage:"), (["help"], 0, "Usage:"), (["--help"], 0, "Usage:"), (["-h"], 0, "Usage:"), (["bad"], 2, "invalid command"), (["help", "bad"], 2, "invalid command")):
                result = subprocess.run([binary, *args], env=env, capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, code, result.stderr)
                self.assertIn(output, result.stdout if code == 0 else result.stderr)
                self.assertEqual(result.stderr if code == 0 else result.stdout, "")
            self.check_service_processes(binary, env, Path(directory))
            self.check_demo(binary, env, Path(directory))
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
                with socket.socket() as reservation:
                    reservation.bind(("127.0.0.1", 0))
                    dns_port = reservation.getsockname()[1]
                process_env = dict(env, EPOCHALYPSE_CONFIG=str(config), EPOCHALYPSE_HTTP_ADDR=f"127.0.0.1:{port}", EPOCHALYPSE_DNS_ADDR=f"127.0.0.1:{dns_port}", EPOCHALYPSE_STATE_DIR=str(state), EPOCHALYPSE_AGENT_ID="lab-agent", EPOCHALYPSE_BUILD_COMMIT="b" * 40)
                process = subprocess.Popen([binary, mode, "--local-lab"], env=process_env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    self.wait_for_health(process, port)
                    if mode == "dns":
                        self.check_dns_frames(dns_port)
                    with urllib.request.urlopen(f"http://127.0.0.1:{port}/version", timeout=1) as response:
                        self.assertEqual(json.load(response), {"commit": self.commit})
                    for requested, expected in ((self.commit, "success"), ("b" * 40, "failure")):
                        result = subprocess.run([binary, "status", "--endpoints", f"http://127.0.0.1:{port}", requested], env=env, capture_output=True, text=True, timeout=10)
                        self.assertEqual(result.returncode, 0, result.stderr)
                        self.assertEqual(result.stdout, f"{expected} local {self.commit}\n")
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

    def check_dns_frames(self, port):
        for transport, length in ((socket.SOCK_STREAM, 513), (socket.SOCK_STREAM, 65535), (socket.SOCK_DGRAM, 1232), (socket.SOCK_DGRAM, 8192)):
            with self.subTest(transport=transport, length=length):
                query = bytearray(length)
                query[:17] = bytes.fromhex("1234010000010000000000010000010001")
                query[17:28] = bytes.fromhex("00ff000001000000000000")
                query[26:28] = (length - 28).to_bytes(2, "big")
                with socket.socket(type=transport) as connection:
                    connection.settimeout(3)
                    connection.connect(("127.0.0.1", port))
                    if transport == socket.SOCK_STREAM:
                        connection.sendall(length.to_bytes(2, "big"))
                        connection.sendall(query[:512])
                        connection.sendall(query[512:])
                        response = connection.makefile("rb").read(14)
                        self.assertEqual(response[:2], b"\x00\x0c")
                        response = response[2:]
                    else:
                        connection.sendall(query)
                        response = connection.recv(65535)
                    self.assertEqual(response, bytes.fromhex("123481050000000000000000"))
        for frame in (b"\x00", b"\xff\xff" + bytes(12), b"\x00\x01\x00"):
            with socket.create_connection(("127.0.0.1", port), timeout=3) as connection:
                started = time.monotonic()
                connection.sendall(frame)
                self.assertEqual(connection.recv(1), b"")
                self.assertLess(time.monotonic() - started, 2)

    def check_demo(self, binary, env, directory):
        state = directory / "fixtures"
        result = subprocess.run([binary, "demo", "--state-dir", str(state)], env=env, capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        fixtures = json.loads((state / "demo" / "fixtures.json").read_text())
        self.assertTrue(fixtures["synthetic"])
        self.assertEqual({record["kind"] for record in fixtures["records"]}, {"capsule", "switch", "countdown", "window"})
        self.assertTrue(all(record["implemented"] is False for record in fixtures["records"]))
        for namespace, code in (("", 1), ("../", 1), ("demo", 0)):
            result = subprocess.run([binary, "clean-demo", "--state-dir", str(state), "--namespace", namespace], env=env, capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, code, result.stderr)
        self.assertFalse((state / "demo").exists())

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
