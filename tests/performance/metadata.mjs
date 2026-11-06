import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import https from 'node:https';
import {performance} from 'node:perf_hooks';
import {setTimeout as sleep} from 'node:timers/promises';

let input = '';
for await (const chunk of process.stdin) input += chunk;
const expected = JSON.parse(input);
const agent = new https.Agent({ca: readFileSync('/tmp/metadata.crt'), servername: 'localhost', keepAlive: true, maxSockets: 16});

function request(path, method = 'GET', body = '', headers = {}, timing = {}) {
  return new Promise((resolve, reject) => {
    const req = https.request({hostname: '127.0.0.1', port: 8080, path, method, agent, headers}, res => {
      let data = '';
      res.on('data', chunk => {
        data += chunk;
        if (data.length > 1048576) req.destroy(new Error('response_limit'));
      });
      res.on('error', reject);
      res.on('end', () => resolve({status: res.statusCode, headers: res.headers, body: data}));
    });
    req.setTimeout(3000, () => req.destroy(new Error('request_timeout')));
    req.on('socket', () => { timing.socket_assigned_at = performance.now(); });
    req.on('finish', () => { timing.dispatched_at = performance.now(); });
    req.on('error', reject);
    req.end(body);
  });
}

function cookie(response) {
  return (response.headers['set-cookie'] ?? []).map(value => value.split(';', 1)[0]).join('; ');
}

try {
  let ready = false;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try { ready = (await request('/healthz')).status === 200; } catch { /* Startup can precede the listener. */ }
    if (ready) break;
    await sleep(250);
  }
  assert.ok(ready, 'service_not_ready');
  const version = await request('/version');
  assert.equal(version.status, 200);
  assert.equal(JSON.parse(version.body).commit, expected.commit);
  const csrf = await request('/api/v1/csrf');
  assert.equal(csrf.status, 200);
  const login = await request('/api/v1/session', 'POST', JSON.stringify({password: expected.password}), {
    'content-type': 'application/json', cookie: cookie(csrf), 'x-csrf-token': JSON.parse(csrf.body).csrf_token,
  });
  assert.equal(login.status, 204);
  const ownerHeaders = {cookie: `${cookie(csrf)}; ${cookie(login)}`};
  const paths = [
    [`/windows/${expected.window.id}`, {}, body => body.includes(expected.window.content)],
    [`/api/v1/capsules/${expected.capsule_id}`, ownerHeaders, body => JSON.parse(body).id === expected.capsule_id],
    [`/api/v1/public/countdowns/${expected.countdown.id}`, {}, body => JSON.parse(body).id === expected.countdown.id],
    ['/api/v1/dashboard', ownerHeaders, body => ['capsules', 'switches', 'countdowns', 'agents'].every(key => Array.isArray(JSON.parse(body)[key]?.items))],
  ];
  for (const [path, headers, valid] of paths) {
    const response = await request(path, 'GET', '', headers);
    assert.equal(response.status, 200);
    assert.ok(valid(response.body));
  }
  const samples = [];
  const start = performance.now();
  const pending = [];
  // Absolute start times preserve the offered rate when a response becomes slow.
  for (let index = 0; index < 200; index += 1) {
    const scheduled = index * 50;
    await sleep(Math.max(0, start + scheduled - performance.now()));
    const [path, headers, valid] = paths[index % paths.length];
    const sent = performance.now();
    const timing = {};
    pending.push((async () => {
      let status = 0;
      let error = null;
      try {
        const response = await request(path, 'GET', '', headers, timing);
        status = response.status;
        if (status !== 200) error = 'unexpected_status';
        else if (!valid(response.body)) error = 'unexpected_body';
      } catch { error = 'transport_or_body_error'; }
      if (!timing.dispatched_at && !error) error = 'missing_dispatch_time';
      samples.push({index, path, status, error, scheduled_ms: scheduled, launched_ms: sent - start,
        dispatched_ms: timing.dispatched_at ? timing.dispatched_at - start : null,
        socket_wait_ms: timing.socket_assigned_at ? timing.socket_assigned_at - sent : null,
        elapsed_ms: performance.now() - sent, scheduled_elapsed_ms: performance.now() - start - scheduled});
    })());
  }
  await Promise.all(pending);
  const elapsed = performance.now() - start;
  const sorted = samples.map(sample => sample.elapsed_ms).sort((a, b) => a - b);
  const p95 = sorted[Math.ceil(sorted.length * 0.95) - 1];
  const errors = samples.filter(sample => sample.error).length;
  const maxLag = Math.max(...samples.map(sample => sample.dispatched_ms === null ? Infinity : sample.dispatched_ms - sample.scheduled_ms));
  const maxSocketWait = Math.max(...samples.map(sample => sample.socket_wait_ms ?? 0));
  const statusCounts = {};
  for (const sample of samples) {
    statusCounts[sample.status] = (statusCounts[sample.status] ?? 0) + 1;
  }
  const result = {source_commit: expected.commit, node: process.version, tls: 'fixture certificate CA; localhost hostname verification',
    requests: samples.length, errors, status_counts: statusCounts, expected_status: 200, offered_requests_per_second: 20, schedule_seconds: 10,
    elapsed_ms: elapsed, completed_requests_per_second: samples.length / (elapsed / 1000), max_dispatch_lag_ms: Number.isFinite(maxLag) ? maxLag : null,
    max_socket_wait_ms: maxSocketWait,
    p50_ms: sorted[99], p95_ms: p95, max_ms: sorted.at(-1), target_ms: 250,
    pass: samples.length === 200 && errors === 0 && p95 < 250 && maxLag < 50, samples: samples.sort((a, b) => a.index - b.index)};
  console.log(JSON.stringify(result));
  if (!result.pass) process.exitCode = 1;
} catch {
  console.error('Metadata benchmark setup fails; credentials and response bodies are suppressed.');
  process.exitCode = 1;
} finally { agent.destroy(); }
