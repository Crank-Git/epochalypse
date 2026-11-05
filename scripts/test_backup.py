import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import uuid


PLAYWRIGHT_IMAGE = "mcr.microsoft.com/playwright@sha256:2f05f8a734509a7b3f26a912639488ee08f58a7b292d14d62271bcff9038768f"
GO_COVERAGE = "github.com/Crank-Git/epochalypse/internal/version.Commit"


class BackupRecoveryTests(unittest.TestCase):
    def test_linux_restore_preserves_owner_and_public_records(self):
        Path(".test-artifacts").mkdir(exist_ok=True)
        project = "epochalypse-backup-" + uuid.uuid4().hex[:10]
        source_volume = project + "-source"
        restored_volume = project + "-restored"
        coverage_volume = project + "-coverage"
        image = project + ":local"
        builder_image = project + ":builder"
        container_names = [project + "-fixture", project + "-instrumented", project + "-restore", project + "-main", project + "-client"]
        volumes = [source_volume, restored_volume, coverage_volume]
        commit = subprocess.run(["git", "rev-parse", "HEAD"], check=True, capture_output=True, text=True, timeout=10).stdout.strip()
        self.assertEqual(len(commit), 40, "The recovery image uses a full source commit")

        def docker(*args, timeout=60, check=True, input=None):
            result = subprocess.run(["docker", *args], input=input, capture_output=True, text=True, timeout=timeout)
            if check:
                self.assertEqual(result.returncode, 0, result.stdout[-3000:] + result.stderr[-3000:])
            return result

        with tempfile.TemporaryDirectory(dir=".test-artifacts") as temporary:
            directory = Path(temporary).resolve()
            fence = directory / "go.mod"
            fence.write_text("module epochalypse-local-state\n\ngo 1.27.3\n")
            go_environment = ["--env", "XDG_CACHE_HOME=/src/var/cache/xdg", "--mount", f"type=bind,source={fence},target=/src/var/go.mod,readonly"]
            archive = directory / "recovery.age"
            identity = directory / "identity"
            recipient = directory / "recipient"
            expected_path = directory / "expected.json"
            instrumented = directory / "epochalypse"
            certificate = directory / "fullchain.pem"
            coverage_output = Path(".test-artifacts/backup.out").resolve()
            try:
                coverage_output.unlink(missing_ok=True)
                docker("pull", PLAYWRIGHT_IMAGE, timeout=180)
                docker("volume", "create", source_volume)
                docker("volume", "create", restored_volume)
                docker("volume", "create", coverage_volume)
                docker("build", "--target", "build", "--tag", builder_image, "--build-arg", f"EPOCHALYPSE_BUILD_COMMIT={commit}", "--file", "deploy/lab/Dockerfile", ".", timeout=600)
                docker("build", "--tag", image, "--build-arg", f"EPOCHALYPSE_BUILD_COMMIT={commit}", "--file", "deploy/lab/Dockerfile", ".", timeout=600)

                fixture_name = container_names[0]
                docker("create", "--name", fixture_name, "--network", "none", *go_environment, "--mount", f"type=volume,source={source_volume},target=/fixture", "--env", "EPOCHALYPSE_BACKUP_FIXTURE_DIR=/fixture", "--env", f"EPOCHALYPSE_BACKUP_FIXTURE_COMMIT={commit}", "--entrypoint", "/bin/sh", builder_image, "-c", "mkdir -p /src/tests/backup && sleep 3600")
                docker("start", fixture_name)
                docker("cp", "tests/backup/fixture_test.go", f"{fixture_name}:/src/tests/backup/fixture_test.go")
                fixture_run = docker("exec", fixture_name, "go", "test", "./tests/backup", "-run", "^TestWritesSyntheticRecoveryInstallation$", "-count=1", timeout=180)
                self.assertIn("ok", fixture_run.stdout)
                docker("cp", f"{fixture_name}:/fixture/credentials/identity", str(identity))
                docker("cp", f"{fixture_name}:/fixture/credentials/recipient", str(recipient))
                docker("cp", f"{fixture_name}:/fixture/expected.json", str(expected_path))
                os.chmod(identity, 0o600)
                os.chmod(recipient, 0o600)
                expected = json.loads(expected_path.read_text())
                self.assertEqual(expected["commit"], commit)
                source_hashes = self.source_hashes(docker, builder_image, source_volume)
                self.assertTrue(source_hashes, "The Linux fixture contains source state")

                instrumented_name = container_names[1]
                docker("create", "--name", instrumented_name, "--network", "none", *go_environment, "--mount", f"type=volume,source={source_volume},target=/fixture", "--env", "GOCOVERDIR=/coverage", "--mount", f"type=volume,source={coverage_volume},target=/coverage", "--entrypoint", "/bin/sh", builder_image, "-c", f"mkdir -p /coverage && chown 65534:65534 /coverage && chmod 0700 /coverage && CGO_ENABLED=0 go build -cover -coverpkg=github.com/Crank-Git/epochalypse/... -ldflags '-X {GO_COVERAGE}={commit}' -o /fixture/epochalypse ./cmd/epochalypse")
                docker("start", "--attach", instrumented_name, timeout=240)
                docker("cp", f"{instrumented_name}:/fixture/epochalypse", str(instrumented))
                os.chmod(instrumented, 0o755)

                create = docker("run", "--rm", "--network", "none", "--mount", f"type=volume,source={source_volume},target=/fixture", "--mount", f"type=volume,source={coverage_volume},target=/coverage", "--mount", f"type=bind,source={directory},target=/artifacts", "--mount", f"type=bind,source={recipient},target=/recipient,readonly", "--mount", f"type=bind,source={instrumented},target=/instrumented,readonly", "--env", "GOCOVERDIR=/coverage", "--entrypoint", "/instrumented", image, "backup", "create", "--source", "/fixture/installation", "--output", "/artifacts/recovery.age", "--recipient-file", "/recipient", "--commit", commit, "--max-objects", "100", "--quota-bytes", "1073741824", timeout=60)
                self.assertEqual(json.loads(create.stdout), {"commit": commit, "max_objects": 100, "quota_bytes": 1073741824})
                self.assertTrue(archive.is_file())

                restore = docker("run", "--rm", "--network", "none", "--read-only", "--tmpfs", "/tmp", "--mount", f"type=volume,source={restored_volume},target=/restore-parent", "--mount", f"type=volume,source={coverage_volume},target=/coverage", "--mount", f"type=bind,source={archive},target=/archive.age,readonly", "--mount", f"type=bind,source={identity},target=/identity,readonly", "--mount", f"type=bind,source={instrumented},target=/instrumented,readonly", "--env", "GOCOVERDIR=/coverage", "--entrypoint", "/instrumented", image, "backup", "restore", "--input", "/archive.age", "--destination", "/restore-parent/installation", "--identity-file", "/identity", "--commit", commit, "--uid", "65534", "--gid", "65534", timeout=60)
                restored = json.loads(restore.stdout)
                self.assertEqual(restored, {"commit": commit, "max_objects": 100, "quota_bytes": 1073741824})
                inventory = self.restored_inventory(docker, builder_image, restored_volume)
                expected_files = {
                    "/restore-parent/installation/state/v1/owner.db",
                    "/restore-parent/installation/state/v1/windows.db",
                    "/restore-parent/installation/config/main.json",
                    "/restore-parent/installation/keys/window.private",
                    "/restore-parent/installation/keys/window.key",
                    "/restore-parent/installation/keys/countdown.private",
                    "/restore-parent/installation/keys/countdown.key",
                    "/restore-parent/installation/tls/fullchain.pem",
                    "/restore-parent/installation/tls/private.pem",
                }
                self.assertEqual(set(inventory["files"]), expected_files, "Restore publishes only the nine canonical files")
                self.assertTrue(all(owner == "65534:65534" and mode == "600" for owner, mode in inventory["files"].values()))
                expected_directories = {
                    "/restore-parent/installation",
                    "/restore-parent/installation/state",
                    "/restore-parent/installation/state/v1",
                    "/restore-parent/installation/config",
                    "/restore-parent/installation/keys",
                    "/restore-parent/installation/tls",
                }
                self.assertEqual(set(inventory["directories"]), expected_directories, "Restore omits the setup marker and agent state")
                self.assertTrue(all(owner == "65534:65534" and mode == "700" for owner, mode in inventory["directories"].values()))

                main_name = container_names[3]
                docker("run", "--detach", "--name", main_name, "--network", "none", "--user", "65534:65534", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--tmpfs", "/tmp", "--mount", f"type=volume,source={restored_volume},target=/restore-parent", "--mount", f"type=volume,source={coverage_volume},target=/coverage", "--mount", f"type=bind,source={instrumented},target=/instrumented,readonly", "--env", "GOCOVERDIR=/coverage", "--entrypoint", "/instrumented", image, "server", "--environment", "lab", "--state-dir", "/restore-parent/installation/state", "--config", "/restore-parent/installation/config/main.json", "--http-addr", "0.0.0.0:8080", "--dns-addr", "0.0.0.0:1053", "--key-file", "/restore-parent/installation/keys/window.private", "--countdown-key-file", "/restore-parent/installation/keys/countdown.private", "--tls-cert", "/restore-parent/installation/tls/fullchain.pem", "--tls-key", "/restore-parent/installation/tls/private.pem", "--max-objects", str(restored["max_objects"]), "--quota-bytes", str(restored["quota_bytes"]))
                inspected = json.loads(docker("inspect", main_name).stdout)[0]
                self.assertEqual(inspected["HostConfig"]["NetworkMode"], "none")
                self.assertFalse(inspected["HostConfig"].get("PortBindings"))
                mounts = inspected["Mounts"]
                self.assertEqual({mount["Destination"] for mount in mounts}, {"/restore-parent", "/coverage", "/instrumented"})
                self.assertNotIn(source_volume, {mount.get("Name") for mount in mounts})

                docker("cp", f"{main_name}:/restore-parent/installation/tls/fullchain.pem", str(certificate))
                client_name = container_names[4]
                docker("create", "--name", client_name, "--network", f"container:{main_name}", "--entrypoint", "node", PLAYWRIGHT_IMAGE, "-e", "setInterval(() => {}, 60000)")
                docker("cp", "scripts/backup_recovery.mjs", f"{client_name}:/tmp/backup_recovery.mjs")
                docker("cp", str(certificate), f"{client_name}:/tmp/backup-tls.crt")
                docker("start", client_name)
                client_result = docker("exec", "--interactive", client_name, "node", "/tmp/backup_recovery.mjs", timeout=30, input=expected_path.read_text())
                self.assertIn("authenticated owner read", client_result.stdout)
                self.assertEqual(self.source_hashes(docker, builder_image, source_volume), source_hashes, "Backup and restore leave source files unchanged")

                docker("stop", "--timeout", "10", main_name, timeout=20)
                coverage_output.parent.mkdir(exist_ok=True)
                docker("run", "--rm", "--network", "none", "--mount", f"type=volume,source={coverage_volume},target=/coverage,readonly", "--mount", f"type=bind,source={coverage_output.parent},target=/artifacts", *go_environment, builder_image, "go", "tool", "covdata", "textfmt", "-i=/coverage", "-o=/artifacts/backup.out", timeout=60)
                self.assertTrue(coverage_output.is_file(), "The isolated CLI and service contribute to first-party coverage")
            finally:
                for name in container_names:
                    docker("rm", "--force", "--volumes", name, check=False)
                for name in volumes:
                    docker("volume", "rm", name, check=False)
                docker("image", "rm", image, builder_image, check=False)

    def source_hashes(self, docker, image, volume):
        result = docker("run", "--rm", "--network", "none", "--mount", f"type=volume,source={volume},target=/fixture,readonly", "--entrypoint", "/bin/sh", image, "-c", "find /fixture/installation -type f -exec sha256sum '{}' ';' | sort")
        return result.stdout

    def restored_inventory(self, docker, image, volume):
        def entries(kind):
            result = docker("run", "--rm", "--network", "none", "--mount", f"type=volume,source={volume},target=/restore-parent,readonly", "--entrypoint", "/bin/sh", image, "-c", f"find /restore-parent/installation -type {kind} -exec stat -c '%n %u:%g %a' '{{}}' ';' | sort")
            return {
                fields[0]: (fields[1], fields[2])
                for line in result.stdout.splitlines()
                if (fields := line.split())
            }

        return {"files": entries("f"), "directories": entries("d")}


if __name__ == "__main__":
    unittest.main()
