import datetime
import http.cookiejar
import json
import os
from pathlib import Path
import socket
import ssl
import subprocess
import tempfile
import unittest
import urllib.request
import uuid

from dns_wire import assert_authority, assert_fallback


def free_port(dns=False):
    with socket.socket() as tcp:
        tcp.bind(("127.0.0.1", 0))
        port = tcp.getsockname()[1]
        if dns:
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as udp:
                udp.bind(("127.0.0.1", port))
        return port


class LabTests(unittest.TestCase):
    def test_lab_preserves_signed_windows_after_process_restart(self):
        project = "epochalypse-worker8-" + uuid.uuid4().hex[:12]
        env = dict(os.environ, LAB_HTTP_PORT=str(free_port()), LAB_DNS_PORT=str(free_port(True)), LAB_IMAGE_TAG=project)
        base = ["docker", "compose", "-f", "deploy/lab/compose.yaml", "-p", project]

        def compose(*args, timeout=90):
            result = subprocess.run([*base, *args], env=env, capture_output=True, text=True, timeout=timeout)
            self.assertEqual(result.returncode, 0, result.stdout[-3000:] + result.stderr[-3000:])
            return result.stdout

        Path(".test-artifacts").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=".test-artifacts") as temporary:
            directory = Path(temporary)
            try:
                compose("build")
                compose("up", "--no-build", "--pull", "never", "--detach", "--wait", "--wait-timeout", "60")
                compose("cp", "web:/state/lab/tls.crt", str(directory / "tls.crt"), timeout=15)
                context = ssl.create_default_context(cafile=str(directory / "tls.crt"))
                client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), urllib.request.HTTPSHandler(context=context))
                origin = "https://127.0.0.1:" + env["LAB_HTTP_PORT"]
                with client.open(origin + "/api/v1/csrf", timeout=3) as response:
                    proof = json.load(response)["csrf_token"]

                def post(path, body):
                    request = urllib.request.Request(origin + path, data=json.dumps(body).encode(), headers={"Content-Type": "application/json", "X-CSRF-Token": proof}, method="POST")
                    return client.open(request, timeout=3)

                with post("/api/v1/session", {"password": "synthetic-lab-password"}) as response:
                    self.assertEqual(response.status, 204)
                inception = datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0) + datetime.timedelta(hours=1)
                with post("/api/v1/windows", {"content": "x" * 512, "inception": inception.isoformat().replace("+00:00", "Z"), "expiration": (inception + datetime.timedelta(hours=1)).isoformat().replace("+00:00", "Z")}) as response:
                    self.assertEqual(response.status, 201)
                    record = json.load(response)
                port = int(env["LAB_DNS_PORT"])
                assert_authority(record, port=port)
                assert_fallback(record, port=port)
                verified = json.loads(compose("exec", "-T", "web", "epochalypse", "lab", "verify", "--zone", record["zone"], "--address", "127.0.0.1:1053", "--anchor-file", "/state/lab/window.key"))
                self.assertEqual(verified["status"], "verified")
                self.assertEqual(verified["trust_source"], "explicit-local-anchor")
                self.assertIn("experimental TXT validity remain separate", verified["detail"])
                services = [json.loads(line) for line in compose("ps", "--all", "--format", "json").splitlines()]
                running = {item["Service"]: item for item in services if item["State"] == "running"}
                self.assertEqual(set(running), {"web", "agent-1", "agent-2", "agent-3"})
                self.assertTrue(all(item["Health"] == "healthy" for item in running.values()))
                for service in running:
                    self.assertEqual(compose("exec", "-T", service, "id", "-u").strip(), "65534")

                def inspect(container, expression):
                    return subprocess.check_output(["docker", "inspect", "--format", expression, container], env=env, text=True, timeout=10).strip()

                state_volumes = set()
                for item in running.values():
                    mounts = json.loads(inspect(item["ID"], "{{json .Mounts}}"))
                    volumes = [mount["Name"] for mount in mounts if mount["Destination"] == "/state"]
                    self.assertEqual(len(volumes), 1)
                    state_volumes.add(volumes[0])
                self.assertEqual(len(state_volumes), 4, "The main process and agents have independent volumes")
                jobs = {item["Service"]: item for item in services if item["Service"] in {"lab-material", "lab-owner"}}
                self.assertEqual(set(jobs), {"lab-material", "lab-owner"})
                self.assertTrue(all(item["State"] == "exited" and item["ExitCode"] == 0 for item in jobs.values()))
                self.assertEqual(inspect(jobs["lab-material"]["ID"], "{{.HostConfig.NetworkMode}}"), "none")
                original_pid = inspect(running["web"]["ID"], "{{.State.Pid}}")
                compose("restart", "web", timeout=20)
                compose("up", "--no-build", "--pull", "never", "--detach", "--wait", "--wait-timeout", "60")
                self.assertNotEqual(inspect(running["web"]["ID"], "{{.State.Pid}}"), original_pid, "The lab starts a new executable process")
                assert_authority(record, port=port)
                assert_fallback(record, port=port)
                compose("cp", "web:/state/lab/window.key", str(directory / "window.key"), timeout=15)
                original_key = (directory / "window.key").read_bytes()
                compose("exec", "-T", "web", "chmod", "0644", "/state/lab/window.private")
                try:
                    unsafe = subprocess.run([*base, "run", "--rm", "--no-deps", "lab-material"], env=env, capture_output=True, text=True, timeout=20)
                    self.assertNotEqual(unsafe.returncode, 0, "The setup rejects unsafe private-key access")
                    self.assertIn("owner-only access", unsafe.stderr)
                finally:
                    compose("exec", "-T", "web", "chmod", "0600", "/state/lab/window.private")
                self.assertIn("remains unchanged", compose("run", "--rm", "--no-deps", "lab-material", timeout=20))
                compose("cp", "web:/state/lab/window.key", str(directory / "window.key"), timeout=15)
                self.assertEqual((directory / "window.key").read_bytes(), original_key, "Repeated setup preserves the DNSKEY")
                print("Lab: TLS creation, cohost UDP/TCP, fallback, a new process, four independent volumes, and three healthy agents pass")
            finally:
                compose("down", "--volumes", "--timeout", "3", timeout=30)
