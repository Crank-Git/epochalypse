import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import https from 'node:https';
import net from 'node:net';
import dgram from 'node:dgram';

const expected = JSON.parse(await new Promise((resolve, reject) => {
  let data = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { data += chunk; });
  process.stdin.on('end', () => resolve(data));
  process.stdin.on('error', reject);
}));
const certificate = await readFile('/tmp/backup-tls.crt');
const agent = new https.Agent({ca: certificate, servername: 'localhost'});

function request(method, path, body = '', headers = {}) {
  return new Promise((resolve, reject) => {
    const request = https.request({hostname: '127.0.0.1', port: 8080, path, method, agent, headers}, response => {
      const chunks = [];
      response.on('data', chunk => chunks.push(chunk));
      response.on('end', () => resolve({status: response.statusCode, headers: response.headers, body: Buffer.concat(chunks).toString()}));
    });
    request.setTimeout(3000, () => request.destroy(new Error('HTTPS request timed out')));
    request.on('error', reject);
    if (body) request.write(body);
    request.end();
  });
}

function cookie(response) {
  const values = response.headers['set-cookie'] ?? [];
  return values.map(value => value.split(';', 1)[0]).join('; ');
}

let health;
let healthError;
for (let attempt = 0; attempt < 30; attempt += 1) {
  try {
    health = await request('GET', '/healthz');
    if (health.status === 200) break;
  } catch (error) {
    healthError = error;
  }
  await new Promise(resolve => setTimeout(resolve, 250));
}
assert.equal(health?.status, 200, healthError?.message ?? 'The restored service did not become healthy');
const version = await request('GET', '/version');
assert.equal(version.status, 200);
assert.equal(JSON.parse(version.body).commit, expected.commit);
const csrf = await request('GET', '/api/v1/csrf');
assert.equal(csrf.status, 200);
const proof = JSON.parse(csrf.body).csrf_token;
const csrfCookie = cookie(csrf);
const session = await request('POST', '/api/v1/session', JSON.stringify({password: expected.password}), {
  'content-type': 'application/json',
  'cookie': csrfCookie,
  'x-csrf-token': proof,
});
assert.equal(session.status, 204);
const ownerCookie = `${csrfCookie}; ${cookie(session)}`;
const dashboard = await request('GET', '/api/v1/dashboard', '', {cookie: ownerCookie});
assert.equal(dashboard.status, 200);

const windowPage = await request('GET', `/windows/${expected.window.id}`);
assert.equal(windowPage.status, 200);
assert.ok(windowPage.body.includes(expected.window.content));
const countdownPage = await request('GET', `/events/${expected.countdown.id}`);
assert.equal(countdownPage.status, 200);
assert.ok(countdownPage.body.includes('Synthetic recovery event'));
assert.ok(countdownPage.body.includes(expected.countdown.event_at.replace(/\.\d+Z$/, 'Z')));
assert.ok(countdownPage.body.includes(expected.countdown.name));
const countdownAPI = await request('GET', `/api/v1/public/countdowns/${expected.countdown.id}`);
assert.equal(countdownAPI.status, 200);
const retainedCountdown = JSON.parse(countdownAPI.body);
assert.equal(retainedCountdown.id, expected.countdown.id);
assert.equal(retainedCountdown.name, expected.countdown.name);
assert.equal(retainedCountdown.event_at, expected.countdown.event_at);
const capsule = await request('GET', `/api/v1/public/capsules/${expected.capsule_id}/download`);
assert.equal(capsule.status, 200);
assert.equal(capsule.body, expected.capsule_plaintext);

function wireName(name) {
  return Buffer.concat(name.replace(/\.$/, '').split('.').flatMap(label => [Buffer.from([label.length]), Buffer.from(label)]).concat([Buffer.from([0])]));
}

function skipName(packet, offset) {
  for (let steps = 0; steps < 128; steps += 1) {
    assert.ok(offset < packet.length, 'DNS name exceeds the packet');
    const size = packet[offset];
    offset += 1;
    if ((size & 0xc0) === 0xc0) return offset + 1;
    if (size === 0) return offset;
    assert.ok(size <= 63 && offset + size <= packet.length, 'DNS label exceeds the packet');
    offset += size;
  }
  throw new Error('DNS name exceeds its label bound');
}

function decodeDNS(packet, record, expectedContent, expectedSignature = null, expectedKeyTag = null) {
  const identifier = packet.readUInt16BE(0);
  const flags = packet.readUInt16BE(2);
  const questions = packet.readUInt16BE(4);
  const answers = packet.readUInt16BE(6);
  assert.equal(identifier, 0x1948);
  assert.equal(questions, 1);
  assert.equal(flags & 0x8000, 0x8000);
  assert.equal(flags & 0x0400, 0x0400);
  assert.equal(flags & 0x0200, 0);
  assert.equal(flags & 0x000f, 0);
  assert.equal(answers, 2);
  let offset = skipName(packet, 12) + 4;
  let content;
  let signature;
  let keyTag;
  let expiration;
  let inception;
  for (let index = 0; index < answers; index += 1) {
    offset = skipName(packet, offset);
    const type = packet.readUInt16BE(offset);
    const klass = packet.readUInt16BE(offset + 2);
    const length = packet.readUInt16BE(offset + 8);
    offset += 10;
    const end = offset + length;
    assert.equal(klass, 1);
    if (type === 16) {
      const chunks = [];
      while (offset < end) {
        const size = packet[offset++];
        chunks.push(packet.subarray(offset, offset + size));
        offset += size;
      }
      content = Buffer.concat(chunks).toString();
    } else if (type === 46) {
      assert.equal(packet.readUInt16BE(offset), 16);
      assert.equal(packet[offset + 2], 13);
      expiration = packet.readUInt32BE(offset + 8);
      inception = packet.readUInt32BE(offset + 12);
      keyTag = packet.readUInt16BE(offset + 16);
      if (record.expiration && record.inception) {
        assert.equal(expiration, Math.floor(new Date(record.expiration).getTime() / 1000));
        assert.equal(inception, Math.floor(new Date(record.inception).getTime() / 1000));
      }
      const signerEnd = skipName(packet, offset + 18);
      signature = packet.subarray(signerEnd, end);
    }
    offset = end;
  }
  assert.equal(content, expectedContent);
  assert.ok(signature, 'DNS response omits its signature');
  if (expectedSignature) {
    assert.deepEqual(signature, Buffer.from(expectedSignature.trim().split(/\s+/).at(-1), 'base64'));
  }
  if (expectedKeyTag !== null) {
    assert.equal(keyTag, expectedKeyTag);
    const now = Math.floor(Date.now() / 1000);
    assert.ok(inception <= now && expiration > now, 'Countdown signature is outside its current window');
  }
}

function query(name, transport) {
  const question = Buffer.concat([wireName(name), Buffer.from([0, 16, 0, 1])]);
  // The DO bit requests the retained signature with the TXT answer.
  const opt = Buffer.alloc(11);
  opt.writeUInt16BE(41, 1);
  opt.writeUInt16BE(1232, 3);
  opt.writeUInt32BE(0x8000, 5);
  const packet = Buffer.alloc(12 + question.length + opt.length);
  packet.writeUInt16BE(0x1948, 0);
  packet.writeUInt16BE(0x0100, 2);
  packet.writeUInt16BE(1, 4);
  packet.writeUInt16BE(1, 10);
  question.copy(packet, 12);
  opt.copy(packet, 12 + question.length);
  if (transport === 'udp') {
    return new Promise((resolve, reject) => {
      const socket = dgram.createSocket('udp4');
      let timer;
      socket.once('error', error => { clearTimeout(timer); socket.close(); reject(error); });
      socket.once('message', response => { clearTimeout(timer); socket.close(); resolve(response); });
      socket.send(packet, 1053, '127.0.0.1', error => { if (error) reject(error); });
      timer = setTimeout(() => { socket.close(); reject(new Error('DNS UDP request timed out')); }, 3000);
    });
  }
  return new Promise((resolve, reject) => {
    const socket = net.connect(1053, '127.0.0.1');
    const frame = Buffer.alloc(packet.length + 2);
    frame.writeUInt16BE(packet.length, 0);
    packet.copy(frame, 2);
    let response = Buffer.alloc(0);
    socket.setTimeout(3000, () => socket.destroy(new Error('DNS TCP request timed out')));
    socket.on('connect', () => socket.write(frame));
    socket.on('data', chunk => {
      response = Buffer.concat([response, chunk]);
      if (response.length >= 2 && response.length >= response.readUInt16BE(0) + 2) {
        socket.end();
        resolve(response.subarray(2, response.readUInt16BE(0) + 2));
      }
    });
    socket.on('error', reject);
  });
}

for (const transport of ['udp', 'tcp']) {
  decodeDNS(await query(expected.window.name, transport), expected.window, expected.window.content, expected.window.rrsig);
  const countdownContent = `v=epochalypse1 event=${expected.countdown.event_at.replace(/\.\d+Z$/, 'Z')}`;
  const countdownPacket = await query(expected.countdown.name, transport);
  decodeDNS(countdownPacket, expected.countdown, countdownContent, null, expected.countdown_key_tag);
}
console.log('Linux restore: authenticated owner read, signed window, released capsule, and countdown UDP/TCP records pass');
