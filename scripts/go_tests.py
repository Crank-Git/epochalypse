import json
import subprocess
import sys


def run(args):
    result = subprocess.run(["go", "test", "-json", *args], capture_output=True, text=True)
    passed = 0
    skipped = 0
    for line in result.stdout.splitlines():
        event = json.loads(line)
        if "Output" in event:
            print(event["Output"], end="")
        if event.get("Test"):
            passed += event["Action"] == "pass"
            skipped += event["Action"] == "skip"
    print(result.stderr, end="", file=sys.stderr)
    print(f"Go tests: {passed} passed; {skipped} skipped")
    return int(result.returncode != 0 or passed == 0 or skipped != 0)


if __name__ == "__main__":
    sys.exit(run(sys.argv[1:]))
