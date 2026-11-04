import assert from 'node:assert/strict';
import {spawn, spawnSync} from 'node:child_process';
import {writeFile, readFile, mkdir} from 'node:fs/promises';
import {setTimeout as pause} from 'node:timers/promises';
import {generateKeyPairSync} from 'node:crypto';

const directory = '/state';
await writeFile(`${directory}/password`, 'synthetic-browser', {mode: 0o600});
await writeFile(`${directory}/config.json`, '{"version":1}');
await mkdir(`${directory}/coverage`);
// Each browser run uses a separate synthetic DNSSEC key.
const key = generateKeyPairSync('ec', {namedCurve: 'prime256v1'}).privateKey.export({format: 'jwk'});
const publicKey = Buffer.concat([Buffer.from(key.x, 'base64url'), Buffer.from(key.y, 'base64url')]).toString('base64');
await writeFile(`${directory}/synthetic.private`, `Private-key-format: v1.3\nAlgorithm: 13 (ECDSAP256SHA256)\nPrivateKey: ${Buffer.from(key.d, 'base64url').toString('base64')}\n`, {mode: 0o600});
await writeFile(`${directory}/synthetic.key`, `windows.example. 60 IN DNSKEY 257 3 13 ${publicKey}`, {mode: 0o600});
const env = {...process.env, GOCOVERDIR: `${directory}/coverage`};
const init = spawnSync(`${directory}/epochalypse`, ['owner', 'init', '--password-file', `${directory}/password`, '--state-dir', `${directory}/data`], {env, encoding: 'utf8', timeout: 10000});
assert.equal(init.status, 0, init.stderr);
const server = spawn(`${directory}/epochalypse`, ['server', '--config', `${directory}/config.json`, '--state-dir', `${directory}/data`, '--http-addr', '127.0.0.1:8080', '--local-lab', '--zone', 'windows.example.', '--key-file', `${directory}/synthetic.private`], {env, stdio: ['ignore', 'ignore', 'pipe']});
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
  const result = spawnSync('node', ['/work/scripts/owner_browser.mjs', executable, `${directory}/chrome`, 'http://127.0.0.1:8080', `${directory}/window-record.json`], {stdio: 'inherit', timeout: 30000});
  assert.equal(result.status, 0, 'The owner browser flow passes');
  const record = JSON.parse(await readFile(`${directory}/window-record.json`, 'utf8'));
  const locked = spawnSync(`${directory}/epochalypse`, ['window', 'inspect', '--id', record.id, '--state-dir', `${directory}/data`, '--zone', 'windows.example.', '--key-file', `${directory}/synthetic.private`], {env, encoding: 'utf8', timeout: 5000});
  assert.equal(locked.status, 1, 'The live database rejects an offline CLI reader');
  assert.match(locked.stderr, /open window database/);
  assert.equal(locked.stdout, '');
} finally {
  server.kill('SIGTERM');
  if (server.exitCode === null && server.signalCode === null) {
    await new Promise(resolve => {
      const timer = setTimeout(() => { server.kill('SIGKILL'); resolve(); }, 3000);
      server.once('exit', () => { clearTimeout(timer); resolve(); });
    });
  }
}
const record = JSON.parse(await readFile(`${directory}/window-record.json`, 'utf8'));
const windowFlags = ['--state-dir', `${directory}/data`, '--zone', 'windows.example.', '--key-file', `${directory}/synthetic.private`];
const inspect = spawnSync(`${directory}/epochalypse`, ['window', 'inspect', '--id', record.id, ...windowFlags], {env, encoding: 'utf8', timeout: 5000});
assert.equal(inspect.status, 0, inspect.stderr);
assert.deepEqual(JSON.parse(inspect.stdout), record, 'Offline inspection preserves the browser-created signature');
const quota = spawnSync(`${directory}/epochalypse`, ['window', 'create', '--content', 'public CLI experiment', '--inception', record.inception, '--expiration', record.expiration, '--max-objects', '1', ...windowFlags], {env, encoding: 'utf8', timeout: 5000});
assert.equal(quota.status, 1, 'The CLI enforces the retained-object quota');
assert.match(quota.stderr, /quota exceeded/);
const created = spawnSync(`${directory}/epochalypse`, ['window', 'create', '--content', 'public CLI experiment', '--inception', record.inception, '--expiration', record.expiration, ...windowFlags], {env, encoding: 'utf8', timeout: 5000});
assert.equal(created.status, 0, created.stderr);
assert.equal(JSON.parse(created.stdout).content, 'public CLI experiment');
