import json
import re
import subprocess
import sys


TEST_TIMEOUT_SECONDS = 120
RACE_PREPARATION_TIMEOUT_SECONDS = 120
MODULE_PATH = "github.com/Crank-Git/epochalypse"


def timeout_output(output):
    if isinstance(output, bytes):
        return output.decode("utf-8", errors="replace")
    return output or ""


def run(args):
    if args == ["--prepare-race"]:
        return prepare_race()
    if args == ["--race-groups"]:
        return race_groups()
    try:
        result = subprocess.run(["go", "test", "-json", *args], capture_output=True, text=True, timeout=TEST_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired as error:
        print(timeout_output(error.stdout), end="")
        print(timeout_output(error.stderr), end="", file=sys.stderr)
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
            or event.get("Action") not in ("start", "run", "pause", "cont", "pass", "bench", "fail", "output", "skip")
            or any(not isinstance(event.get(field, ""), str) for field in ("Action", "Test", "Output"))
        ):
            print(f"Go tests: malformed event at line {number}. Inspect the output from go test -json. Require a documented Action and optional Test and Output strings.", file=sys.stderr)
            return 1
        if "Output" in event:
            print(event["Output"], end="")
        if event.get("Test"):
            passed += event["Action"] == "pass"
            skipped += event["Action"] == "skip"
    print(f"Go tests: {passed} passed; {skipped} skipped")
    return int(result.returncode != 0 or passed == 0 or skipped != 0)


def prepare_race():
    print("Go race preparation: build-only; required tests run in the next command.", flush=True)
    command = ["go", "test", "-race", "-run", "^$", "./..."]
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=RACE_PREPARATION_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired as error:
        print(timeout_output(error.stdout), end="")
        print(timeout_output(error.stderr), end="", file=sys.stderr)
        print(f"Go race preparation exceeded {RACE_PREPARATION_TIMEOUT_SECONDS} seconds. Inspect the compilation command: {' '.join(command)}", file=sys.stderr)
        return 1
    print(result.stdout, end="")
    print(result.stderr, end="", file=sys.stderr)
    return int(result.returncode != 0)


def race_groups():
    command = ["go", "list", "./..."]
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=TEST_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired as error:
        print(timeout_output(error.stdout), end="")
        print(timeout_output(error.stderr), end="", file=sys.stderr)
        print(f"Go race inventory exceeded {TEST_TIMEOUT_SECONDS} seconds. Inspect: {' '.join(command)}", file=sys.stderr)
        return 1
    print(result.stderr, end="", file=sys.stderr)
    if result.returncode != 0:
        print(result.stdout, end="")
        print("Go race inventory failed. Inspect: go list ./...", file=sys.stderr)
        return 1
    packages = result.stdout.splitlines()
    client = MODULE_PATH + "/internal/switchclient"
    if (
        len(packages) < 2
        or len(set(packages)) != len(packages)
        or client not in packages
        or any(re.fullmatch(re.escape(MODULE_PATH) + r"(?:/[A-Za-z0-9_-][A-Za-z0-9_.-]*)*", package) is None for package in packages)
    ):
        print("Go race inventory is invalid. Require unique module packages, switchclient, and a nonempty remaining group.", file=sys.stderr)
        return 1
    # Separate the maximum-content HTTPS test from other resource-heavy packages.
    groups = [[client], [package for package in packages if package != client]]
    for group in groups:
        print(f"Go race group: go test -json -race -count=1 {' '.join(group)}", flush=True)
        if run(["-race", "-count=1", *group]) != 0:
            return 1
    return 0


if __name__ == "__main__":
    sys.exit(run(sys.argv[1:]))
