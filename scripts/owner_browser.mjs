import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {readFile} from 'node:fs/promises';
import {setTimeout as pause} from 'node:timers/promises';

const [chrome, profile, origin] = process.argv.slice(2);
const sandbox = process.getuid?.() === 0 ? ['--no-sandbox'] : [];
const browser = spawn(chrome, ['--headless', '--no-first-run', '--no-default-browser-check', '--disable-breakpad', '--disable-crash-reporter',
  ...sandbox, `--user-data-dir=${profile}`, '--remote-debugging-port=0', 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
let diagnostics = '';
browser.stderr.on('data', data => { diagnostics = (diagnostics + data).slice(-4000); });
browser.on('error', error => { diagnostics = error.message; });
let socket;
try {
  let port;
  for (let attempt = 0; attempt < 100; attempt++) {
    if (browser.exitCode !== null || browser.signalCode !== null) throw new Error(`Chrome exits before its debug endpoint starts (${browser.exitCode}, ${browser.signalCode}): ${diagnostics}`);
    try { port = (await readFile(`${profile}/DevToolsActivePort`, 'utf8')).split('\n')[0]; break; }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
    await pause(50);
  }
  assert.ok(port, 'Chrome starts its debug endpoint');
  const tabs = await (await fetch(`http://127.0.0.1:${port}/json/list`, {signal: AbortSignal.timeout(3000)})).json();
  socket = new WebSocket(tabs.find(tab => tab.type === 'page').webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
  let sequence = 0;
  const pending = new Map();
  socket.onmessage = event => {
    const reply = JSON.parse(event.data);
    if (!pending.has(reply.id)) return;
    const {resolve, reject, timer} = pending.get(reply.id);
    clearTimeout(timer);
    pending.delete(reply.id);
    if (reply.error) reject(new Error(JSON.stringify(reply.error))); else resolve(reply.result);
  };
  const call = (method, params = {}) => new Promise((resolve, reject) => {
    const id = ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error(`CDP timeout: ${method}`)); }, 5000);
    pending.set(id, {resolve, reject, timer});
    socket.send(JSON.stringify({id, method, params}));
  });
  const evaluate = async expression => {
    const reply = await call('Runtime.evaluate', {expression, returnByValue: true});
    if (reply.exceptionDetails) throw new Error('Browser expression fails');
    return reply.result.value;
  };
  const waitFor = async expression => {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (await evaluate(expression)) return;
      await pause(50);
    }
    throw new Error(`Page assertion times out: ${expression}`);
  };
  await call('Page.navigate', {url: `${origin}/owner`});
  await waitFor("document.querySelector('h1')?.textContent === 'Owner sign-in'");
  assert.equal(await evaluate("document.querySelector('label').htmlFor"), 'password');
  assert.equal(await evaluate("document.querySelector('#password').type"), 'password');
  await evaluate("document.querySelector('#password').value = 'wrong'; document.querySelector('button').click()");
  await waitFor("document.querySelector('[role=alert]') !== null");
  assert.equal(await evaluate("document.querySelector('#password').value"), '');
  await evaluate("document.querySelector('#password').value = 'synthetic-browser'; document.querySelector('button').click()");
  await waitFor("document.querySelector('h1')?.textContent === 'Signed in'");
  assert.equal(await evaluate('document.cookie'), '');
  await evaluate("document.body.dataset.beforeReload = 'true'");
  await call('Page.reload');
  await waitFor("document.body.dataset.beforeReload !== 'true' && document.querySelector('h1')?.textContent === 'Signed in'");
  await evaluate("document.querySelector('button').click()");
  await waitFor("document.querySelector('h1')?.textContent === 'Owner sign-in'");
  await evaluate("document.body.dataset.beforeReload = 'true'");
  await call('Page.reload');
  await waitFor("document.body.dataset.beforeReload !== 'true' && document.querySelector('h1')?.textContent === 'Owner sign-in'");
  console.log('Browser: credential error, sign-in, HttpOnly cookie, reload, and sign-out pass');
} finally {
  socket?.close();
  browser.kill('SIGTERM');
  if (browser.exitCode === null && browser.signalCode === null) {
    await new Promise(resolve => {
      const timer = setTimeout(() => { browser.kill('SIGKILL'); resolve(); }, 3000);
      browser.once('exit', () => { clearTimeout(timer); resolve(); });
    });
  }
}
