import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest


STAGES = ("check-tools", "test-unit", "test-integration", "test-e2e", "test-race", "coverage", "fuzz-smoke", "lint", "build")


class CheckTests(unittest.TestCase):
    def run_check(self, failure):
        Path(".test-artifacts").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=".test-artifacts") as directory:
            root = Path(directory)
            log = root / "calls"
            runner = root / "stage.py"
            runner.write_text("import os, pathlib, sys\nwith pathlib.Path(os.environ['CHECK_LOG']).open('a') as log:\n    log.write(sys.argv[1] + '\\n')\nsys.exit(int(sys.argv[1] == os.environ['CHECK_FAIL']))\n")
            fixture = root / "targets.mk"
            fixture.write_text("\n".join(f"{stage}:\n\t@python3 {shlex.quote(str(runner))} {stage}" for stage in STAGES))
            make = f"make -f Makefile -f {shlex.quote(str(fixture))}"
            result = subprocess.run(["make", "-f", "Makefile", "-f", str(fixture), "check", f"MAKE={make}"], env=dict(os.environ, CHECK_LOG=str(log), CHECK_FAIL=failure), capture_output=True, text=True, timeout=30)
            return result, log.read_text().splitlines()

    def test_check_runs_every_current_stage(self):
        result, calls = self.run_check("")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, list(STAGES[:-2]) + ["check-tools", "lint", "build"])

    def test_check_rejects_each_failed_stage(self):
        for stage in STAGES:
            with self.subTest(stage=stage):
                result, calls = self.run_check(stage)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls[-1], stage)

    def test_future_suites_report_failure(self):
        for target in ("test-resolvers",):
            result = subprocess.run(["make", target], capture_output=True, text=True, timeout=30)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("unavailable", result.stderr)


if __name__ == "__main__":
    unittest.main()
