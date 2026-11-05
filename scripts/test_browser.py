import os
from pathlib import Path
import subprocess
import tempfile
import unittest


BROWSER_IMAGE = "mcr.microsoft.com/playwright@sha256:2f05f8a734509a7b3f26a912639488ee08f58a7b292d14d62271bcff9038768f"


class OwnerBrowserTests(unittest.TestCase):
    def test_owner_signs_in_and_signs_out_in_chromium(self):
        self.run_browser(delay_styles=False)

    def test_dashboard_waits_for_delayed_styles_in_chromium(self):
        self.run_browser(delay_styles=True)

    def run_browser(self, *, delay_styles):
        Path(".test-artifacts").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=".test-artifacts") as temporary:
            directory = Path(temporary).resolve()
            architecture = subprocess.check_output(["docker", "version", "--format", "{{.Server.Arch}}"], text=True, timeout=10).strip()
            self.assertIn(architecture, ("amd64", "arm64"))
            env = dict(os.environ, GOOS="linux", GOARCH=architecture, CGO_ENABLED="0")
            subprocess.run(["go", "build", "-cover", "-coverpkg=github.com/Crank-Git/epochalypse/...", "-o", str(directory / "epochalypse"), "./cmd/epochalypse"], env=env, check=True, timeout=120)
            subprocess.run(["go", "test", "-c", "-o", str(directory / "capsule-fixture"), "./internal/service"], env=env, check=True, timeout=120)
            mounts = self.delayed_style_mount(directory) if delay_styles else []
            subprocess.run(["docker", "run", "--rm", "--init", "--network", "none", "--shm-size", "512m", "--mount", f"type=bind,src={Path.cwd()},dst=/work,readonly", "--mount", f"type=bind,src={directory},dst=/state", *mounts, BROWSER_IMAGE, "sh", "-c", 'node /work/scripts/container_owner.mjs; result=$?; chmod -R a+rwX /state; exit "$result"'], check=True, timeout=120)
            subprocess.run(["go", "tool", "covdata", "textfmt", f"-i={directory / 'coverage'}", "-o=.test-artifacts/browser.out"], check=True, timeout=20)

    def delayed_style_mount(self, directory):
        source = Path("scripts/owner_browser.mjs").read_text()
        changes = [
            ("  socket.onmessage = event => {", "  let delayedStyles = 0;\n  socket.onmessage = async event => {"),
            ("    if (!pending.has(reply.id)) return;", """    if (reply.method === 'Fetch.requestPaused') {
      delayedStyles++;
      await pause(1500);
      await call('Fetch.continueRequest', {requestId: reply.params.requestId});
      return;
    }
    if (!pending.has(reply.id)) return;"""),
            ("  await call('Page.reload');", """  const protocol = await (await fetch(`http://127.0.0.1:${port}/json/protocol`)).json();
  const fetchSchema = protocol.domains.find(domain => domain.domain === 'Fetch');
  assert.ok(fetchSchema.commands.find(command => command.name === 'enable').parameters.find(parameter => parameter.name === 'patterns'));
  assert.ok(fetchSchema.commands.find(command => command.name === 'continueRequest').parameters.find(parameter => parameter.name === 'requestId'));
  const networkSchema = protocol.domains.find(domain => domain.domain === 'Network');
  assert.ok(networkSchema.commands.find(command => command.name === 'setCacheDisabled').parameters.find(parameter => parameter.name === 'cacheDisabled'));
  await call('Network.setCacheDisabled', {cacheDisabled: true});
  await call('Fetch.enable', {patterns: [{urlPattern: '*/owner/style.css', requestStage: 'Request'}]});
  await call('Page.reload');"""),
            ("  assert.equal(await evaluate(\"getComputedStyle(document.querySelector('.sections')).display\"), 'grid');", """  assert.equal(delayedStyles, 1, 'The reload requests the delayed stylesheet');
  console.log('DELAYED_STYLES', JSON.stringify(await evaluate("({ready:document.readyState,styles:[...document.styleSheets].map(style=>style.href),display:getComputedStyle(document.querySelector('.sections')).display})")));
  assert.equal(await evaluate("getComputedStyle(document.querySelector('.sections')).display"), 'grid');
  await call('Fetch.disable');"""),
        ]
        for old, new in changes:
            self.assertIn(old, source, "The delay fixture matches the browser flow")
            source = source.replace(old, new, 1)
        script = directory / "owner_browser.mjs"
        script.write_text(source)
        return ["--mount", f"type=bind,src={script},dst=/work/scripts/owner_browser.mjs,readonly"]


if __name__ == "__main__":
    unittest.main()
