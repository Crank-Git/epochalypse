import unittest
from pathlib import Path
import subprocess
import sys
import tempfile

from coverage_gate import measure


class CoverageTests(unittest.TestCase):
    def test_cli_enforces_coverage_boundary(self):
        Path(".test-artifacts").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=".test-artifacts") as directory:
            profile = Path(directory) / "coverage.out"
            cases = (
                (None, 1, "coverage:"),
                ("", 1, "empty coverage profile"),
                ("mode: atomic\nbad", 1, "malformed coverage block"),
                ("mode: atomic\na.go:1.1,2.1 94999 1\na.go:3.1,4.1 5001 0", 1, "below 95.00 percent"),
                ("mode: atomic\na.go:1.1,2.1 95 1\na.go:3.1,4.1 5 0", 0, "95.0000 percent"),
            )
            for content, code, message in cases:
                with self.subTest(content=content):
                    if content is not None:
                        profile.write_text(content)
                    result = subprocess.run([sys.executable, "scripts/coverage_gate.py", str(profile)], capture_output=True, text=True, timeout=10)
                    self.assertEqual(result.returncode, code, result.stderr)
                    self.assertIn(message, result.stderr if code else result.stdout)

    def test_accepts_exact_boundary(self):
        self.assertEqual(measure(["mode: atomic\na.go:1.1,2.1 95 1\na.go:3.1,4.1 5 0\n"]), (95, 100))

    def test_rejects_invalid_profiles(self):
        for profile in ("", "mode: atomic\n", "mode: bad\na.go:1.1,2.1 1 1", "mode: set\nbad", "mode: set\na.go:1.1,2.1 -1 1", "mode: set\na.go:2.1,1.1 1 1"):
            with self.subTest(profile=profile), self.assertRaises(ValueError):
                measure([profile])

    def test_preserves_unrounded_total(self):
        covered, total = measure(["mode: count\na.go:1.1,2.1 94999 1\na.go:3.1,4.1 5001 0"])
        self.assertLess(covered * 100, total * 95)

    def test_combines_instrumented_runs(self):
        self.assertEqual(measure(["mode: atomic\na.go:1.1,2.1 1 0", "mode: set\na.go:1.1,2.1 1 1"]), (1, 1))

    def test_rejects_conflicting_blocks(self):
        with self.assertRaises(ValueError):
            measure(["mode: set\na.go:1.1,2.1 1 0\na.go:1.1,2.1 2 1"])


if __name__ == "__main__":
    unittest.main()
