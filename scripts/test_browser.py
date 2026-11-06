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

    def test_countdown_restores_page_focus_in_chromium(self):
        self.run_browser(delay_styles=False, background_countdown=True)

    def run_browser(self, *, delay_styles, background_countdown=False):
        Path(".test-artifacts").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=".test-artifacts") as temporary:
            directory = Path(temporary).resolve()
            architecture = subprocess.check_output(["docker", "version", "--format", "{{.Server.Arch}}"], text=True, timeout=10).strip()
            self.assertIn(architecture, ("amd64", "arm64"))
            env = dict(os.environ, GOOS="linux", GOARCH=architecture, CGO_ENABLED="0")
            subprocess.run(["go", "build", "-cover", "-coverpkg=github.com/Crank-Git/epochalypse/...", "-o", str(directory / "epochalypse"), "./cmd/epochalypse"], env=env, check=True, timeout=120)
            subprocess.run(["go", "test", "-c", "-o", str(directory / "capsule-fixture"), "./internal/service"], env=env, check=True, timeout=120)
            mounts = self.delayed_style_mount(directory) if delay_styles else []
            if background_countdown:
                mounts.extend(self.background_countdown_mount(directory))
            subprocess.run(["docker", "run", "--rm", "--init", "--network", "none", "--shm-size", "512m", "--mount", f"type=bind,src={Path.cwd()},dst=/work,readonly", "--mount", f"type=bind,src={directory},dst=/state", *mounts, BROWSER_IMAGE, "sh", "-c", 'node /work/scripts/container_owner.mjs; result=$?; chmod -R a+rwX /state; exit "$result"'], check=True, timeout=120)
            subprocess.run(["go", "tool", "covdata", "textfmt", f"-i={directory / 'coverage'}", "-o=.test-artifacts/browser.out"], check=True, timeout=20)

    def background_countdown_mount(self, directory):
        (directory / "public_countdown_original.mjs").write_text(Path("scripts/public_countdown_browser.mjs").read_text())
        script = directory / "public_countdown_browser.mjs"
        script.write_text("""import assert from 'node:assert/strict';
import {checkPublicCountdown as original} from '/state/public_countdown_original.mjs';

export async function checkPublicCountdown(call, evaluate, waitFor, origin, record) {
  let targetId;
  let unavailable = false;
  const backgroundAfterLoad = async expression => {
    await waitFor(expression);
    const remaining = await evaluate("document.querySelector('#remaining').textContent");
    if (remaining === 'Time unavailable.') unavailable = true;
    if (targetId || !unavailable || remaining !== '10 whole seconds remain.') return;
    await waitFor("document.readyState === 'complete' && document.querySelector('#sync-time').disabled === false");
    ({targetId} = await call('Target.createTarget', {url: 'about:blank', newWindow: true}));
    await call('Target.activateTarget', {targetId});
    await waitFor('document.hasFocus() === false');
    assert.equal(await evaluate('document.visibilityState'), 'visible');
    await evaluate("document.querySelector('#sync-time').focus()");
    assert.equal(await evaluate("document.activeElement.id"), 'sync-time');
    assert.equal(await evaluate("document.querySelector('#sync-time').matches(':focus')"), false);
    assert.equal(await evaluate("getComputedStyle(document.querySelector('#sync-time')).outlineStyle"), 'none');
    console.log('COUNTDOWN_BACKGROUND_FOCUS', 'activeElement alone does not establish page focus');
  };
  const keyboardCall = async (method, params) => {
    if (method === 'Input.dispatchKeyEvent') {
      assert.equal(await evaluate('document.hasFocus()'), true, 'Keyboard input requires page focus');
      assert.equal(await evaluate("document.querySelector('#sync-time').disabled"), false);
    }
    return call(method, params);
  };
  try {
    await original(keyboardCall, evaluate, backgroundAfterLoad, origin, record);
    assert.ok(targetId, 'The regression activates a second window before the keyboard check');
  } finally {
    if (targetId) await call('Target.closeTarget', {targetId});
  }
}
""")
        return ["--mount", f"type=bind,src={script},dst=/work/scripts/public_countdown_browser.mjs,readonly"]

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
