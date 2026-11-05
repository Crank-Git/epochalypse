import os
from pathlib import Path
import subprocess
import tempfile
import unittest


BROWSER_IMAGE = "mcr.microsoft.com/playwright@sha256:2f05f8a734509a7b3f26a912639488ee08f58a7b292d14d62271bcff9038768f"


class SwitchArmExecutableTests(unittest.TestCase):
    def test_cli_arms_three_durable_tls_agents(self):
        Path(".test-artifacts").mkdir(exist_ok=True)
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
            coverage = directory / "coverage"
            coverage.mkdir()
            coverpkg = "github.com/Crank-Git/epochalypse/..."
            subprocess.run(["go", "build", "-cover", f"-coverpkg={coverpkg}", "-o", str(executable), "./cmd/epochalypse"], env=env, check=True, timeout=120)
            subprocess.run(["go", "test", "-c", "-coverpkg=" + coverpkg, "-o", str(fixture), "./internal/command"], env=env, check=True, timeout=120)
            subprocess.run([
                "docker", "run", "--rm", "--init", "--network", "none",
                "--mount", f"type=bind,src={Path.cwd()},dst=/work,readonly",
                "--mount", f"type=bind,src={directory},dst=/state",
                BROWSER_IMAGE,
                "sh", "-c",
                "mkdir -p /tmp/switch-cover; GOCOVERDIR=/tmp/switch-cover EPOCHALYPSE_SWITCH_ARM_BINARY=/state/epochalypse /state/switch-positive.test -test.run=^TestCLIArmsSwitchWithThreeDurableTLSAgents$ -test.timeout=90s; result=$?; cp -R /tmp/switch-cover/. /state/coverage/; chmod -R a+rwX /state; exit \"$result\"",
            ], check=True, timeout=150)
            coverage_profile = Path(".test-artifacts/switch-arm.out")
            subprocess.run(["go", "tool", "covdata", "textfmt", f"-i={coverage}", f"-o={coverage_profile}"], check=True, timeout=20)


if __name__ == "__main__":
    unittest.main()
