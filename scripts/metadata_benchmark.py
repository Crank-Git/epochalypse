"""Measure the local metadata target in isolated Linux containers."""

import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import uuid


CLIENT_IMAGE = "mcr.microsoft.com/playwright@sha256:2f05f8a734509a7b3f26a912639488ee08f58a7b292d14d62271bcff9038768f"


def command(*args, timeout=60, check=True, input=None):
    result = subprocess.run(args, input=input, capture_output=True, text=True, timeout=timeout)
    if check and result.returncode:
        raise RuntimeError(f"{args[0]} {args[1]} fails; command output is suppressed")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    os.chdir(root)
    build_cache = root / "var/cache/buildx"
    build_cache.mkdir(parents=True, exist_ok=True)
    os.environ["BUILDX_CONFIG"] = str(build_cache)
    command("git", "diff", "--exit-code", "HEAD", "--", "cmd", "internal", "web", "config", "third_party",
            "deploy/lab/Dockerfile", "go.mod", "go.sum", "tests/backup/fixture_test.go")
    commit = command("git", "rev-parse", "HEAD").stdout.strip()
    project = "epochalypse-metadata-" + uuid.uuid4().hex[:10]
    builder, image = project + ":build", project + ":main"
    fixture, server, client = [project + suffix for suffix in ("-fixture", "-main", "-client")]
    volume = project + "-state"

    def docker(*parts, **kwargs):
        return command("docker", *parts, **kwargs)

    with tempfile.TemporaryDirectory(prefix="metadata-") as temporary:
        directory = Path(temporary)
        os.chmod(directory, 0o700)
        fence = directory / "go.mod"
        fence.write_text("module epochalypse-local-state\n\ngo 1.27.3\n")
        try:
            for target, tag in (("build", builder), (None, image)):
                options = ["--target", target] if target else []
                docker("build", *options, "--tag", tag, "--build-arg", f"EPOCHALYPSE_BUILD_COMMIT={commit}",
                       "--file", "deploy/lab/Dockerfile", ".", timeout=600)
            docker("volume", "create", volume)
            docker("run", "--detach", "--name", fixture, "--network", "none",
                   "--mount", f"type=volume,source={volume},target=/fixture",
                   "--mount", f"type=bind,source={fence},target=/src/var/go.mod,readonly",
                   "--env", "XDG_CACHE_HOME=/src/var/cache/xdg",
                   "--env", "EPOCHALYPSE_BACKUP_FIXTURE_DIR=/fixture",
                   "--env", f"EPOCHALYPSE_BACKUP_FIXTURE_COMMIT={commit}",
                   "--entrypoint", "/bin/sh", builder, "-c", "mkdir -p /src/tests/backup && sleep 3600")
            docker("cp", "tests/backup/fixture_test.go", f"{fixture}:/src/tests/backup/fixture_test.go")
            docker("exec", fixture, "go", "test", "./tests/backup", "-run", "^TestWritesSyntheticRecoveryInstallation$", "-count=1", timeout=180)
            docker("cp", f"{fixture}:/fixture/expected.json", str(directory / "expected.json"))
            os.chmod(directory / "expected.json", 0o600)
            docker("cp", f"{fixture}:/fixture/installation/tls/fullchain.pem", str(directory / "tls.crt"))
            docker("exec", fixture, "chown", "-R", "65534:65534", "/fixture/installation")
            storage = docker("exec", fixture, "du", "-sk", "/fixture/installation").stdout.split()[0]
            docker("run", "--detach", "--name", server, "--network", "none", "--cpus", "2",
                   "--memory", "256m", "--memory-swap", "256m", "--user", "65534:65534", "--read-only",
                   "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--tmpfs", "/tmp",
                   "--mount", f"type=volume,source={volume},target=/fixture", image, "server",
                   "--environment", "lab", "--state-dir", "/fixture/installation/state",
                   "--config", "/fixture/installation/config/main.json", "--http-addr", "0.0.0.0:8080",
                   "--dns-addr", "0.0.0.0:1053", "--key-file", "/fixture/installation/keys/window.private",
                   "--countdown-key-file", "/fixture/installation/keys/countdown.private",
                   "--tls-cert", "/fixture/installation/tls/fullchain.pem", "--tls-key", "/fixture/installation/tls/private.pem")
            inspected = json.loads(docker("inspect", server).stdout)[0]
            limits = inspected["HostConfig"]
            assert limits["NanoCpus"] == 2000000000
            assert limits["Memory"] == 268435456
            assert limits["NetworkMode"] == "none" and not limits.get("PortBindings")
            docker("create", "--name", client, "--network", f"container:{server}", "--entrypoint", "node",
                   CLIENT_IMAGE, "-e", "setInterval(() => {}, 60000)")
            docker("cp", "tests/performance/metadata.mjs", f"{client}:/tmp/metadata.mjs")
            docker("cp", str(directory / "tls.crt"), f"{client}:/tmp/metadata.crt")
            docker("start", client)
            before = json.loads(docker("stats", "--no-stream", "--format", "json", server).stdout)
            result = docker("exec", "--interactive", client, "node", "/tmp/metadata.mjs",
                            input=(directory / "expected.json").read_text(), timeout=40, check=False)
            if not result.stdout.strip():
                raise RuntimeError("metadata client produces no report; setup fails")
            report = json.loads(result.stdout)
            report["resources"] = {"nano_cpus": limits["NanoCpus"], "memory_bytes": limits["Memory"],
                                   "memory_swap_bytes": limits["MemorySwap"], "storage_before_kib": int(storage),
                                   "stats_before": before,
                                   "stats_after": json.loads(docker("stats", "--no-stream", "--format", "json", server).stdout)}
            info = json.loads(docker("info", "--format", "{{json .}}").stdout)
            report["host"] = {key: info[key] for key in ("OSType", "Architecture", "NCPU", "MemTotal", "KernelVersion", "OperatingSystem")}
            report["docker_server"] = json.loads(docker("version", "--format", "{{json .Server}}").stdout)
            report["client_image"] = CLIENT_IMAGE
            report["server_image_id"] = inspected["Image"]
            report["measured_at_utc"] = command("date", "-u", "+%Y-%m-%dT%H:%M:%SZ").stdout.strip()
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(json.dumps(report, indent=2) + "\n")
            print(json.dumps({key: report[key] for key in ("source_commit", "requests", "errors", "p50_ms", "p95_ms", "max_ms", "pass")}))
            return 0 if report["pass"] and result.returncode == 0 else 1
        finally:
            for name in (client, server, fixture):
                docker("rm", "--force", "--volumes", name, check=False)
            docker("volume", "rm", volume, check=False)
            docker("image", "rm", image, builder, check=False)


if __name__ == "__main__":
    raise SystemExit(main())
