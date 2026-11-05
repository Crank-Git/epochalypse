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
            result = subprocess.run([binary, "lab", "verify", "--zone", "windows.example."], env=env, capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 1, result.stderr)
            self.assertEqual(json.loads(result.stdout)["status"], "indeterminate")
            self.assertEqual(json.loads(result.stdout)["trust_source"], "none")
            self.check_tokens(binary, env, Path(directory))
            self.check_capsules(binary, env, Path(directory))
            self.check_switches(binary, env, Path(directory))
            Path(".test-artifacts").mkdir(exist_ok=True)
            subprocess.run(["go", "tool", "covdata", "textfmt", f"-i={coverdir}", "-o=.test-artifacts/cli.out"], check=True)

    def check_switches(self, binary, env, directory):
        state = directory / "switches"
        args = [binary, "switch", "create", "--state-dir", str(state), "--interval-seconds", "3600", "--grace-seconds", "0"]
        created = subprocess.run(args, env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(created.returncode, 0, created.stderr)
        draft = json.loads(created.stdout)
        self.assertEqual(draft["state"], "draft")
        self.assertEqual(draft["interval_seconds"], 3600)
        self.assertEqual(draft["grace_seconds"], 0)
        self.assertNotIn("deadline", draft)
        status = subprocess.run([binary, "switch", "status", "--state-dir", str(state), "--id", draft["id"]], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(status.returncode, 0, status.stderr)
        self.assertEqual(json.loads(status.stdout), draft)
        args[-1] = "-1"
        invalid = subprocess.run(args, env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(invalid.returncode, 1)
        self.assertEqual(invalid.stdout, "")

    def check_capsules(self, binary, env, directory):
        state = directory / "capsules"
        content = directory / "synthetic.bin"
        content.write_bytes(bytes([0, 255, 128, 1]))
        args = [binary, "capsule", "create", "--state-dir", str(state), "--mode", "server-held", "--kind", "file", "--content-file", str(content), "--release-at", "2099-01-01T00:00:00Z", "--plaintext-limit", "4"]
        created = subprocess.run(args, env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(created.returncode, 0, created.stderr)
        draft = json.loads(created.stdout)
        self.assertIsNone(draft["published_at"])
        self.assertEqual(draft["plaintext_bytes"], 4)
        self.assertNotIn("object", draft)
        self.assertNotIn("key", draft)
        published = subprocess.run([binary, "capsule", "publish", "--state-dir", str(state), "--id", draft["id"]], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(published.returncode, 0, published.stderr)
        self.assertEqual(json.loads(published.stdout)["effective_release_at"], "2099-01-01T00:00:00Z")
        bundle = directory / "held-bundle.json"
        exported = subprocess.run([binary, "capsule", "export", "--state-dir", str(state), "--id", draft["id"], "--output", str(bundle)], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(exported.returncode, 0, exported.stderr)
        self.assertIn("ciphertext-only", exported.stdout)
        self.assertNotIn("released_key", json.loads(bundle.read_text()))
        canceled = subprocess.run([binary, "capsule", "delete", "--state-dir", str(state), "--id", draft["id"], "--confirm", "yes"], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(canceled.returncode, 2)
        deleted = subprocess.run([binary, "capsule", "delete", "--state-dir", str(state), "--id", draft["id"], "--confirm", draft["id"]], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(deleted.returncode, 0, deleted.stderr)
        self.assertIn("Public copies", deleted.stdout)
        output = directory / "unreleased.bin"
        opened = subprocess.run([binary, "capsule", "open", "--bundle", str(bundle), "--output", str(output)], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(opened.returncode, 1)
        self.assertFalse(output.exists())
        public_args = list(args)
        public_args[public_args.index("server-held")] = "public"
        public_created = subprocess.run(public_args, env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(public_created.returncode, 0, public_created.stderr)
        public_id = json.loads(public_created.stdout)["id"]
        for action in ("publish", "export", "delete"):
            command = [binary, "capsule", action, "--state-dir", str(state), "--id", public_id]
            if action == "export":
                command += ["--output", str(directory / "public-bundle.json")]
            if action == "delete":
                command += ["--confirm", public_id]
            result = subprocess.run(command, env=env, capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 0, result.stderr)
        recovered = directory / "recovered.bin"
        opened = subprocess.run([binary, "capsule", "open", "--bundle", str(directory / "public-bundle.json"), "--output", str(recovered)], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(opened.returncode, 0, opened.stderr)
        self.assertEqual(recovered.read_bytes(), bytes([0, 255, 128, 1]))
        content.write_bytes(b"12345")
        rejected = subprocess.run(args, env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(rejected.returncode, 1)
        self.assertEqual(rejected.stdout, "")
        self.assertIn("exceeds its limit", rejected.stderr)

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

    def check_tokens(self, binary, env, directory):
        state = directory / "token-state"
        password = directory / "token-password"
        password.write_text("synthetic-token-password\n")
        password.chmod(0o600)
        result = subprocess.run([binary, "owner", "init", "--state-dir", str(state), "--password-file", str(password)], env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        args = [binary, "token", "create", "--state-dir", str(state), "--name", "automation", "--scopes", "read,switch:checkin", "--expires-at", "2099-01-01T00:00:00Z"]
        result = subprocess.run(args, env=env, capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        grant = json.loads(result.stdout)
        self.assertEqual(len(grant["id"]), 32)
        self.assertEqual(len(grant["token"]), 43)
        self.assertEqual(result.stderr, "")
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            port = reservation.getsockname()[1]
        process = subprocess.Popen([binary, "server", "--config", str(directory / "local.json"), "--state-dir", str(state), "--http-addr", f"127.0.0.1:{port}", "--local-lab"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            self.wait_for_health(process, port)
            origin = f"http://127.0.0.1:{port}"
            result = subprocess.run(args, env=env, capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 1)
            self.assertNotIn(grant["token"], result.stdout + result.stderr)
            with urllib.request.urlopen(origin + "/api/v1/csrf", timeout=2) as response:
                csrf_cookie = response.headers["Set-Cookie"].split(";", 1)[0]
                csrf_token = json.load(response)["csrf_token"]
            headers = {"Content-Type": "application/json", "Cookie": csrf_cookie, "X-CSRF-Token": csrf_token}
            request = urllib.request.Request(origin + "/api/v1/session", data=json.dumps({"password": "synthetic-token-password"}).encode(), headers=headers)
            with urllib.request.urlopen(request, timeout=2) as response:
                self.assertEqual(response.status, 204)
                cookie = csrf_cookie + "; " + response.headers["Set-Cookie"].split(";", 1)[0]
            body = json.dumps({"name": "api", "scopes": ["read"], "expires_at": "2099-01-01T00:00:00Z"}).encode()
            request = urllib.request.Request(origin + "/api/v1/tokens", data=body, headers={"Content-Type": "application/json", "Cookie": cookie, "X-CSRF-Token": csrf_token})
            with urllib.request.urlopen(request, timeout=2) as response:
                self.assertEqual(response.status, 201)
                created = json.load(response)
                self.assertEqual(response.headers["Cache-Control"], "no-store")
            for item in (grant, created):
                request = urllib.request.Request(origin + "/api/v1/tokens", data=body, headers={"Content-Type": "application/json", "Authorization": "Bearer " + item["token"]})
                with self.assertRaises(urllib.error.HTTPError) as rejected:
                    urllib.request.urlopen(request, timeout=2)
                with rejected.exception as response:
                    self.assertEqual(response.code, 401)
                    self.assertNotIn(item["token"], response.read().decode())
                request = urllib.request.Request(origin + "/api/v1/tokens/" + item["id"], method="DELETE", headers={"Cookie": cookie, "X-CSRF-Token": csrf_token})
                with urllib.request.urlopen(request, timeout=2) as response:
                    self.assertEqual(response.status, 204)
                    self.assertEqual(response.read(), b"")
        finally:
            process.terminate()
            stdout, stderr = process.communicate(timeout=5)
        self.assertEqual(process.returncode, 0, stderr)
        self.assertEqual(stdout, "")

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
