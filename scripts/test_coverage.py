import unittest

from coverage_gate import measure


class CoverageTests(unittest.TestCase):
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
