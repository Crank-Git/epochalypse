import os
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest


BROWSER_IMAGE = "mcr.microsoft.com/playwright@sha256:2f05f8a734509a7b3f26a912639488ee08f58a7b292d14d62271bcff9038768f"


class SwitchArmExecutableTests(unittest.TestCase):
    def test_cli_arms_three_durable_tls_agents(self):
        Path(".test-artifacts").mkdir(exist_ok=True)
        sources_before = source_hashes()
        help_output = subprocess.run(["docker", "run", "--help"], check=True, capture_output=True, text=True, timeout=10).stdout
        self.assertIn("--mount", help_output)
        self.assertIn("--network", help_output)
        with tempfile.TemporaryDirectory(dir=".test-artifacts") as temporary:
            directory = Path(temporary).resolve()
            architecture = subprocess.check_output(["docker", "version", "--format", "{{.Server.Arch}}"], text=True, timeout=10).strip()
            self.assertIn(architecture, ("amd64", "arm64"))
            env = dict(os.environ, GOOS="linux", GOARCH=architecture, CGO_ENABLED="0")
            executable = directory / "epochalypse"
            fixture = directory / "switch-positive.test"
            clock_fixture = directory / "agent-clock.test"
            coverage = directory / "coverage"
            coverage.mkdir()
            coverpkg = "github.com/Crank-Git/epochalypse/..."
            commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True, timeout=10).strip()
            flags = f"-X github.com/Crank-Git/epochalypse/internal/version.Commit={commit}"
            subprocess.run(["go", "build", "-cover", f"-coverpkg={coverpkg}", "-ldflags", flags, "-o", str(executable), "./cmd/epochalypse"], env=env, check=True, timeout=120)
            subprocess.run(["go", "test", "-c", "-coverpkg=" + coverpkg, "-ldflags", flags, "-o", str(fixture), "./internal/command"], env=env, check=True, timeout=120)
            subprocess.run(["go", "test", "-c", "-coverpkg=" + coverpkg, "-o", str(clock_fixture), "./internal/agent"], env=env, check=True, timeout=120)
            result = subprocess.run([
                "docker", "run", "--rm", "--init", "--network", "none",
                "--mount", f"type=bind,src={Path.cwd()},dst=/work,readonly",
                "--mount", f"type=bind,src={directory},dst=/state",
                "--mount", "type=volume,dst=/agent-storage",
                "--env", "TMPDIR=/agent-storage",
                "--env", f"EPOCHALYPSE_OUTAGE_COMMIT={commit}",
                BROWSER_IMAGE,
                "sh", "-c",
                "mkdir -p /tmp/switch-cover; GOCOVERDIR=/tmp/switch-cover EPOCHALYPSE_SWITCH_ARM_BINARY=/state/epochalypse /state/switch-positive.test -test.v -test.run='^Test(CLIArmsSwitchWithThreeDurableTLSAgents|CLIStatusChecksPinnedInventory|SwitchOutageRecoversAfterMainAndEachAgentStop)$' -test.timeout=90s && /state/agent-clock.test -test.v -test.gocoverdir=/tmp/switch-cover -test.run='^Test(PhysicalReservation|SystemCapture|ProfileClock|ChronyClock|DurationFromTimespec|ValidBootID)' -test.timeout=60s; result=$?; cp -R /tmp/switch-cover/. /state/coverage/; chmod -R a+rwX /state; exit \"$result\"",
            ], capture_output=True, text=True, timeout=150)
            Path(".test-artifacts/switch-outage.log").write_text(result.stdout + result.stderr)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertNotIn("--- SKIP:", result.stdout)
            for index in range(3):
                self.assertIn(f"--- PASS: TestSwitchOutageRecoversAfterMainAndEachAgentStop/agent-{index}", result.stdout)
            self.assertEqual(sources_before, source_hashes(), "outage drill changes repository source")
            evidence = {
                "commit": commit,
                "go": subprocess.check_output(["go", "version"], text=True, timeout=10).strip(),
                "docker": subprocess.check_output(["docker", "version", "--format", "{{.Server.Version}}"], text=True, timeout=10).strip(),
                "architecture": architecture,
                "image": BROWSER_IMAGE,
                "binary_sha256": hashlib.sha256(executable.read_bytes()).hexdigest(),
                "fixture_sha256": hashlib.sha256(fixture.read_bytes()).hexdigest(),
                "source_sha256": sources_before,
                "clock": "test-only stepped UTC; epsilon=2; S=5; W=25; candidate E=34; no host certification",
                "results": [line.strip() for line in result.stdout.splitlines() if "outage=" in line],
            }
            Path(".test-artifacts/switch-outage.json").write_text(json.dumps(evidence, indent=2) + "\n")
            coverage_profile = Path(".test-artifacts/switch-arm.out")
            subprocess.run(["go", "tool", "covdata", "textfmt", f"-i={coverage}", f"-o={coverage_profile}"], check=True, timeout=20)


def source_hashes():
    files = subprocess.check_output(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], timeout=10)
    return {name: hashlib.sha256(Path(name).read_bytes()).hexdigest()
            for name in sorted(set(files.decode().split("\0"))) if name and Path(name).is_file()}


if __name__ == "__main__":
    unittest.main()
