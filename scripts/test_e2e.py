import os
from pathlib import Path
import subprocess
import tempfile
import unittest


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
            Path(".test-artifacts").mkdir(exist_ok=True)
            subprocess.run(["go", "tool", "covdata", "textfmt", f"-i={coverdir}", "-o=.test-artifacts/cli.out"], check=True)


if __name__ == "__main__":
    unittest.main()
