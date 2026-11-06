import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


WRAPPER = Path(__file__).resolve().with_name("recovery-drill.sh")
CACHES = {
    "GOCACHE": "var/cache/go-build",
    "GOMODCACHE": "var/cache/go-mod",
    "GOPATH": "var/gopath",
    "GOTMPDIR": "var/tmp",
    "XDG_CACHE_HOME": "var/cache",
    "BUILDX_CONFIG": "var/cache/buildx",
}


class RecoveryDrillTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(WRAPPER.is_file(), "the supported recovery drill command is absent")
        artifacts = WRAPPER.parent.parent / ".test-artifacts"
        artifacts.mkdir(exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=artifacts)
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name).resolve()
        (self.root / "scripts").mkdir()
        self.wrapper = self.root / "scripts/recovery-drill.sh"
        shutil.copyfile(WRAPPER, self.wrapper)
        self.bin = self.root / "fake-bin"
        self.bin.mkdir()
        child = self.bin / "python3"
        child.write_text(
            f"#!{sys.executable}\n"
            "import json, os, pathlib, sys\n"
            "pathlib.Path('child.json').write_text(json.dumps({"
            "'argv': sys.argv[1:], 'cwd': os.getcwd(), 'env': dict(os.environ)}))\n"
            "sys.exit(int(os.environ.get('DRILL_TEST_EXIT', '0')))\n"
        )
        child.chmod(0o700)
        self.env = dict(os.environ, PATH=f"{self.bin}:{os.environ['PATH']}")
        self.env.update({name: "/outside-repository" for name in CACHES})

    def run_wrapper(self, *arguments, status=0):
        return subprocess.run(
            ["sh", str(self.wrapper), *arguments], cwd=self.bin,
            env=dict(self.env, DRILL_TEST_EXIT=str(status)),
            capture_output=True, text=True, timeout=10,
        )

    def test_help_exits_before_state_changes(self):
        result = self.run_wrapper("--help")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("synthetic", result.stdout)
        self.assertIn("Usage:", result.stdout)
        self.assertFalse((self.root / "var").exists())
        self.assertFalse((self.root / "child.json").exists())

    def test_unknown_arguments_exit_before_state_changes(self):
        for arguments in [("--host", "example.invalid"), ("example.invalid",), ("--help", "--remote"), ("--",)]:
            with self.subTest(arguments=arguments):
                result = self.run_wrapper(*arguments)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn("Usage:", result.stderr)
                self.assertFalse((self.root / "var").exists())
                self.assertFalse((self.root / "child.json").exists())

    def test_command_uses_the_existing_runner_from_the_repository(self):
        result = self.run_wrapper()
        self.assertEqual(result.returncode, 0, result.stderr)
        child = json.loads((self.root / "child.json").read_text())
        self.assertEqual(child["argv"], ["scripts/test_switch_arm.py"])
        self.assertEqual(child["cwd"], str(self.root))
        for name, relative in CACHES.items():
            self.assertEqual(child["env"][name], str(self.root / relative))
            self.assertTrue((self.root / relative).is_dir())
        for name in ("HOME", "DOCKER_CONFIG"):
            self.assertEqual(child["env"].get(name), self.env.get(name))

    def test_command_preserves_the_runner_failure(self):
        result = self.run_wrapper(status=23)
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(result.stdout, "")


if __name__ == "__main__":
    unittest.main()
