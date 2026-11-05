import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {readFile, writeFile} from 'node:fs/promises';
import {setTimeout as pause} from 'node:timers/promises';
import {checkPublicWindow} from './public_window_browser.mjs';
import {checkCapsules} from './capsule_browser.mjs';

const [chrome, profile, origin, windowResult, preparedPath] = process.argv.slice(2);
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
    const reply = await call('Runtime.evaluate', {expression, returnByValue: true, awaitPromise: true});
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
  assert.equal(await evaluate('document.cookie'), '');
  assert.equal(await evaluate("fetch('/api/v1/dashboard').then(response => response.status)"), 401);
  for (const proof of ['missing', 'wrong']) {
    await evaluate(`document.querySelector('[name=csrf_token]').value = ${JSON.stringify(proof === 'missing' ? '' : 'wrong')}; document.querySelector('#password').value = 'synthetic-browser'; document.querySelector('button').click()`);
    await waitFor("document.body.textContent.includes('invalid_csrf')");
    await call('Page.navigate', {url: `${origin}/owner`});
    await waitFor("document.querySelector('h1')?.textContent === 'Owner sign-in'");
  }
  const proof = await evaluate("document.querySelector('[name=csrf_token]').value");
  const foreignOrigin = origin.replace('127.0.0.1', 'localhost');
  await call('Page.navigate', {url: `${foreignOrigin}/healthz`});
  await waitFor(`location.origin === ${JSON.stringify(foreignOrigin)} && document.readyState === 'complete'`);
  await evaluate(`{
    const form = document.createElement('form');
    form.method = 'POST'; form.action = ${JSON.stringify(`${origin}/owner/sign-in`)};
    for (const [name, value] of Object.entries({password: 'synthetic-browser', csrf_token: ${JSON.stringify(proof)}})) {
      const input = document.createElement('input'); input.name = name; input.value = value; form.append(input);
    }
    document.body.append(form); form.submit();
  }`);
  await waitFor("document.body.textContent.includes('invalid_csrf')");
  await call('Page.navigate', {url: `${origin}/owner`});
  await waitFor("document.querySelector('h1')?.textContent === 'Owner sign-in'");
  await evaluate("document.querySelector('#password').value = 'wrong'; document.querySelector('button').click()");
  await waitFor("document.querySelector('[role=alert]') !== null");
  assert.equal(await evaluate("document.querySelector('#password').value"), '');
  await evaluate("document.querySelector('#password').value = 'synthetic-browser'; document.querySelector('button').click()");
  await waitFor("document.querySelector('h1')?.textContent === 'Signed in'");
  assert.equal(await evaluate('document.cookie'), '');
  await evaluate("document.body.dataset.beforeReload = 'true'");
  await call('Page.reload');
  await waitFor("document.body.dataset.beforeReload !== 'true' && document.querySelector('h1')?.textContent === 'Signed in'");
  assert.deepEqual(await evaluate("Array.from(document.querySelectorAll('section h2'), heading => heading.textContent)"), ['Capsules', 'Switches', 'Countdowns', 'Agent health']);
  assert.equal(await evaluate("document.querySelector('#switches').textContent.includes('production use remains blocked')"), true);
  assert.equal(await evaluate("document.querySelector('#agents').textContent.includes('no verified agent observations')"), true);
  assert.equal(await evaluate("getComputedStyle(document.querySelector('.sections')).display"), 'grid');
  assert.equal(await evaluate("document.querySelector('nav').getAttribute('aria-label')"), 'Dashboard sections');
  await checkCapsules(call, evaluate, waitFor, origin, await readFile(preparedPath, 'utf8'));
  await evaluate("document.querySelector('nav a').focus()");
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9});
  assert.equal(await evaluate("document.activeElement.getAttribute('href')"), '#switches');
  assert.equal(await evaluate("getComputedStyle(document.activeElement).outlineStyle"), 'solid');
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13});
  await waitFor("location.hash === '#switches'");
  const request = async (method, path, body, token) => evaluate(`(async () => {
    const response = await fetch(${JSON.stringify(path)}, {method: ${JSON.stringify(method)},
      headers: {'Content-Type': 'application/json', ${token ? `'X-CSRF-Token': ${JSON.stringify(token)}` : ''}},
      ${body ? `body: ${JSON.stringify(body)},` : ''}
      credentials: 'same-origin'});
    return {status: response.status, body: await response.text()};
  })()`);
  const csrf = await evaluate("document.querySelector('[name=csrf_token]').value");
  const inception = new Date(Date.now() + 3600000).toISOString().replace(/\.\d{3}Z$/, 'Z');
  const expiration = new Date(Date.now() + 7200000).toISOString().replace(/\.\d{3}Z$/, 'Z');
  const publicContent = 'public browser experiment <img src=x onerror="globalThis.publicWindowXSS=1">';
  const windowBody = JSON.stringify({content: publicContent, inception, expiration});
  assert.equal((await request('POST', '/api/v1/windows', windowBody)).status, 403);
  assert.equal((await request('POST', '/api/v1/windows', windowBody, 'wrong')).status, 403);
  const signedWindow = await request('POST', '/api/v1/windows', windowBody, csrf);
  assert.equal(signedWindow.status, 201, signedWindow.body);
  const windowRecord = JSON.parse(signedWindow.body);
  assert.equal(windowRecord.content, publicContent);
  assert.equal(windowRecord.inception, inception);
  assert.equal(windowRecord.expiration, expiration);
  assert.match(windowRecord.rrsig, /RRSIG\s+TXT 13 /);
  const observationPath = `/api/v1/windows/${windowRecord.id}/observations`;
  assert.equal((await request('POST', observationPath, '{}')).status, 403);
  assert.equal((await request('POST', observationPath, '{}', 'wrong')).status, 403);
  assert.equal((await request('POST', observationPath, JSON.stringify({resolver_url: 'http://169.254.169.254/'}), csrf)).status, 400);
  const acceptedRun = await request('POST', observationPath, '{}', csrf);
  assert.equal(acceptedRun.status, 202, acceptedRun.body);
  const runID = JSON.parse(acceptedRun.body).id;
  let observed;
  for (let attempt = 0; attempt < 60; attempt++) {
    const result = await request('GET', observationPath);
    assert.equal(result.status, 200, result.body);
    const page = JSON.parse(result.body);
    assert.equal(page.version, 1);
    assert.equal(page.poll_after_seconds, 1);
    assert.equal(page.next_cursor, '');
    assert.equal(page.runs.length, 1);
    observed = page.runs.find(run => run.id === runID);
    if (observed?.state !== 'pending') break;
    await pause(50);
  }
  assert.equal(observed.state, 'partial');
  assert.equal(observed.results[0].error, 'resolver_unavailable');
  assert.equal(observed.results[0].resolver.address, undefined);
  assert.equal(observed.results[0].resolver.version, 'unknown');
  assert.equal(observed.results[0].resolver.clock.offset_nanoseconds, null);
  assert.equal(observed.results[0].rcode, null);
  assert.equal(observed.results[0].flags, null);
  await checkPublicWindow({call, evaluate, waitFor, origin, record: windowRecord, observed});
  await writeFile(windowResult, JSON.stringify(windowRecord), {mode: 0o600});
  const invalidWindow = await request('POST', '/api/v1/windows', JSON.stringify({content: 'public', inception, expiration: inception}), csrf);
  assert.equal(invalidWindow.status, 400);
  assert.ok(JSON.parse(invalidWindow.body).error.fields.expiration);
  const dashboard = await request('GET', '/api/v1/dashboard?limit=1');
  assert.equal(dashboard.status, 200);
  const summary = JSON.parse(dashboard.body);
  for (const section of ['capsules', 'switches', 'countdowns', 'agents']) assert.deepEqual(summary[section].items, []);
  assert.equal(summary.switches.state, 'blocked');
  assert.equal(summary.agents.state, 'unavailable');
  assert.equal((await request('GET', '/api/v1/dashboard?limit=101')).status, 400);
  const tokenBody = JSON.stringify({name: 'browser', scopes: ['read'], expires_at: '2099-01-01T00:00:00Z'});
  for (const token of [undefined, 'wrong']) {
    assert.equal((await request('DELETE', '/api/v1/session', undefined, token)).status, 403);
    assert.equal((await request('POST', '/api/v1/tokens', tokenBody, token)).status, 403);
  }
  const created = await request('POST', '/api/v1/tokens', tokenBody, csrf);
  assert.equal(created.status, 201);
  const tokenID = JSON.parse(created.body).id;
  const readToken = JSON.parse(created.body).token;
  assert.equal(await evaluate(`fetch('/api/v1/windows', {method: 'POST', headers: {'Content-Type': 'application/json', Authorization: ${JSON.stringify(`Bearer ${readToken}`)}, 'X-CSRF-Token': ${JSON.stringify(csrf)}}, body: ${JSON.stringify(windowBody)}}).then(response => response.status)`), 401);
  const bearerDashboard = () => evaluate(`fetch('/api/v1/dashboard', {headers: {Authorization: ${JSON.stringify(`Bearer ${readToken}`)}}}).then(response => response.status)`);
  assert.equal(await bearerDashboard(), 200);
  assert.equal((await request('DELETE', `/api/v1/tokens/${tokenID}`)).status, 403);
  assert.equal((await request('DELETE', `/api/v1/tokens/${tokenID}`, undefined, csrf)).status, 204);
  assert.equal(await bearerDashboard(), 401);
  await evaluate("document.querySelector('[name=csrf_token]').remove(); document.querySelector('button').click()");
  await waitFor("document.body.textContent.includes('invalid_csrf')");
  await call('Page.navigate', {url: `${origin}/owner`});
  await waitFor("document.querySelector('h1')?.textContent === 'Signed in'");
  await evaluate("document.querySelector('button').click()");
  await waitFor("document.querySelector('h1')?.textContent === 'Owner sign-in'");
  assert.equal((await request('GET', '/api/v1/dashboard')).status, 401);
  assert.equal((await request('POST', '/api/v1/windows', windowBody, csrf)).status, 401);
  assert.equal((await request('POST', observationPath, '{}', csrf)).status, 401);
  assert.equal((await request('GET', observationPath)).status, 200);
  await call('Page.navigate', {url: origin + '/windows/' + windowRecord.id});
  await waitFor("document.querySelector('#status')?.textContent === 'Retained observations.'");
  assert.equal(await evaluate("document.querySelector('#direct-state').textContent"), 'State: observed');
  await evaluate("document.querySelector('#compare').click()");
  await waitFor("document.querySelector('#status').textContent.includes('Sign in as the owner')");
  await call('Page.navigate', {url: origin + '/owner'});
  await waitFor("document.querySelector('h1')?.textContent === 'Owner sign-in'");
  await evaluate("document.body.dataset.beforeReload = 'true'");
  await call('Page.reload');
  await waitFor("document.body.dataset.beforeReload !== 'true' && document.querySelector('h1')?.textContent === 'Owner sign-in'");
  const bootstrap = await evaluate("fetch('/api/v1/csrf').then(response => response.json())");
  for (let attempt = 0; attempt < 4; attempt++) {
    assert.equal((await request('POST', '/api/v1/session', JSON.stringify({password: 'wrong'}), bootstrap.csrf_token)).status, 401);
  }
  const cooldown = await request('POST', '/api/v1/session', JSON.stringify({password: 'synthetic-browser'}), bootstrap.csrf_token);
  assert.equal(cooldown.status, 429);
  assert.ok(!cooldown.body.includes('synthetic-browser'));
  console.log('Browser: signed windows, resolver observations, public reads, dashboard states, keyboard navigation, scoped summary access, CSRF proof, token administration, sign-in, sign-out, and cooldown pass');
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
