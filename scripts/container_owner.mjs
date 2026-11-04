import assert from 'node:assert/strict';
import {spawn, spawnSync} from 'node:child_process';
import {writeFile, mkdir} from 'node:fs/promises';
import {setTimeout as pause} from 'node:timers/promises';

const directory = '/state';
await writeFile(`${directory}/password`, 'synthetic-browser', {mode: 0o600});
await writeFile(`${directory}/config.json`, '{"version":1}');
await mkdir(`${directory}/coverage`);
const env = {...process.env, GOCOVERDIR: `${directory}/coverage`};
const init = spawnSync(`${directory}/epochalypse`, ['owner', 'init', '--password-file', `${directory}/password`, '--state-dir', `${directory}/data`], {env, encoding: 'utf8', timeout: 10000});
assert.equal(init.status, 0, init.stderr);
const server = spawn(`${directory}/epochalypse`, ['server', '--config', `${directory}/config.json`, '--state-dir', `${directory}/data`, '--http-addr', '127.0.0.1:8080', '--local-lab'], {env, stdio: ['ignore', 'ignore', 'pipe']});
let diagnostic = '';
server.stderr.on('data', data => { diagnostic = (diagnostic + data).slice(-2000); });
try {
  let ready = false;
  for (let attempt = 0; attempt < 100; attempt++) {
    assert.equal(server.exitCode, null, diagnostic);
    try { ready = (await fetch('http://127.0.0.1:8080/healthz', {signal: AbortSignal.timeout(1000)})).ok; }
    catch { /* The listener starts after the child process. */ }
    if (ready) break;
    await pause(50);
  }
  assert.ok(ready, 'The server starts');
  const executable = spawnSync('find', ['/ms-playwright', '-name', 'chrome', '-type', 'f'], {encoding: 'utf8', timeout: 3000}).stdout.trim().split('\n')[0];
  assert.ok(executable, 'The pinned image contains Chromium');
  const result = spawnSync('node', ['/work/scripts/owner_browser.mjs', executable, `${directory}/chrome`, 'http://127.0.0.1:8080'], {stdio: 'inherit', timeout: 30000});
  assert.equal(result.status, 0, 'The owner browser flow passes');
} finally {
  server.kill('SIGTERM');
  if (server.exitCode === null && server.signalCode === null) {
    await new Promise(resolve => {
      const timer = setTimeout(() => { server.kill('SIGKILL'); resolve(); }, 3000);
      server.once('exit', () => { clearTimeout(timer); resolve(); });
    });
  }
}
