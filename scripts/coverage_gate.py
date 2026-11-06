import re
import sys
from pathlib import Path


BLOCK = re.compile(r"(.+\.go):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)")


def measure(profiles):
    blocks = {}
    for profile in profiles:
        lines = profile.splitlines()
        if not lines or lines[0] not in ("mode: set", "mode: count", "mode: atomic") or len(lines) < 2:
            raise ValueError("absent or empty coverage profile")
        for line in lines[1:]:
            match = BLOCK.fullmatch(line)
            if not match:
                raise ValueError("malformed coverage block")
            path, sl, sc, el, ec, statements, count = match.groups()
            start, end = (int(sl), int(sc)), (int(el), int(ec))
            size, hits = int(statements), int(count)
            if min(*start, *end) < 1 or start > end or (lines[0] == "mode: set" and hits > 1):
                raise ValueError("invalid coverage range or count")
            key = (path, start, end)
            previous = blocks.get(key, (size, 0))
            if previous[0] != size:
                raise ValueError("conflicting coverage blocks")
            blocks[key] = (size, previous[1] + hits)
    total = sum(size for size, _ in blocks.values())
    if total == 0:
        raise ValueError("coverage has no statements")
    return sum(size for size, hits in blocks.values() if hits > 0), total


def main(paths):
    try:
        covered, total = measure([Path(path).read_text() for path in paths])
        if covered * 100 < total * 90:
            raise ValueError(f"coverage below 90.00 percent: {covered}/{total} statements")
    except (OSError, ValueError) as error:
        print(f"coverage: {error}", file=sys.stderr)
        return 1
    print(f"coverage: {covered}/{total} statements ({covered * 100 / total:.4f} percent)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
