import json
import subprocess
import sys


TEST_TIMEOUT_SECONDS = 120


def run(args):
    try:
        result = subprocess.run(["go", "test", "-json", *args], capture_output=True, text=True, timeout=TEST_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired:
        print(f"Go tests exceeded {TEST_TIMEOUT_SECONDS} seconds. Inspect the test command for a hung test: go test -json {' '.join(args)}", file=sys.stderr)
        return 1
    print(result.stderr, end="", file=sys.stderr)
    passed = 0
    skipped = 0
    for number, line in enumerate(result.stdout.splitlines(), start=1):
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            event = None
        if (
            not isinstance(event, dict)
            or not event.get("Action")
            or any(not isinstance(event.get(field, ""), str) for field in ("Action", "Test", "Output"))
        ):
            print(f"Go tests: malformed event at line {number}. Inspect the output from go test -json. Require an Action string and optional Test and Output strings.", file=sys.stderr)
            return 1
        if "Output" in event:
            print(event["Output"], end="")
        if event.get("Test"):
            passed += event["Action"] == "pass"
            skipped += event["Action"] == "skip"
    print(f"Go tests: {passed} passed; {skipped} skipped")
    return int(result.returncode != 0 or passed == 0 or skipped != 0)


if __name__ == "__main__":
    sys.exit(run(sys.argv[1:]))
