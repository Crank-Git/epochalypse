import os
from pathlib import Path
import subprocess
import tempfile
import unittest


BROWSER_IMAGE = "mcr.microsoft.com/playwright@sha256:2f05f8a734509a7b3f26a912639488ee08f58a7b292d14d62271bcff9038768f"


class OwnerBrowserTests(unittest.TestCase):
    def test_owner_signs_in_and_signs_out_in_chromium(self):
        Path(".test-artifacts").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=".test-artifacts") as temporary:
            directory = Path(temporary).resolve()
            architecture = subprocess.check_output(["docker", "version", "--format", "{{.Server.Arch}}"], text=True, timeout=10).strip()
            self.assertIn(architecture, ("amd64", "arm64"))
            env = dict(os.environ, GOOS="linux", GOARCH=architecture, CGO_ENABLED="0")
            subprocess.run(["go", "build", "-cover", "-coverpkg=github.com/Crank-Git/epochalypse/...", "-o", str(directory / "epochalypse"), "./cmd/epochalypse"], env=env, check=True, timeout=120)
            subprocess.run(["go", "test", "-c", "-o", str(directory / "capsule-fixture"), "./internal/service"], env=env, check=True, timeout=120)
            subprocess.run(["docker", "run", "--rm", "--init", "--network", "none", "--shm-size", "512m", "--mount", f"type=bind,src={Path.cwd()},dst=/work,readonly", "--mount", f"type=bind,src={directory},dst=/state", BROWSER_IMAGE, "sh", "-c", 'node /work/scripts/container_owner.mjs; result=$?; chmod -R a+rwX /state; exit "$result"'], check=True, timeout=120)
            subprocess.run(["go", "tool", "covdata", "textfmt", f"-i={directory / 'coverage'}", "-o=.test-artifacts/browser.out"], check=True, timeout=20)


if __name__ == "__main__":
    unittest.main()
