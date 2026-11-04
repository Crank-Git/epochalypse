from contextlib import redirect_stdout, redirect_stderr
import io
import json
from subprocess import CompletedProcess
import unittest
from unittest.mock import patch

from go_tests import run


class GoTestTests(unittest.TestCase):
    def test_requires_executed_tests(self):
        for actions, code, expected in (([], 0, 1), (["pass"], 0, 0), (["pass", "skip"], 0, 1), (["fail"], 1, 1)):
            with self.subTest(actions=actions):
                output = "\n".join(json.dumps({"Test": "TestBehavior", "Action": action}) for action in actions)
                with patch("go_tests.subprocess.run", return_value=CompletedProcess([], code, output, "")), redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
                    self.assertEqual(run(["./..."]), expected)


if __name__ == "__main__":
    unittest.main()
