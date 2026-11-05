import assert from 'node:assert/strict';
import {generateKeyPairSync, X509Certificate} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {mkdir, mkdtemp, lstat, readFile, writeFile, chmod, chown, rename, rm} from 'node:fs/promises';

const directory = '/state/lab';
const files = ['window.private', 'window.key', 'tls.key', 'tls.crt', 'password', 'metadata.json'];

function openssl(args) {
  const result = spawnSync('/usr/bin/openssl', args, {encoding: 'utf8', timeout: 10000, maxBuffer: 16384});
  assert.equal(result.status, 0, 'The pinned OpenSSL command completes');
  assert.equal(result.error, undefined, 'The pinned OpenSSL command stays within its bounds');
  return result.stdout.trim();
}

async function verifyExisting() {
  const info = await lstat(directory);
  assert.ok(info.isDirectory() && !info.isSymbolicLink(), 'The lab namespace is a regular directory');
  assert.equal(info.mode & 0o777, 0o700, 'The lab directory has owner-only access');
  assert.equal(info.uid, 65534, 'The main process owns the lab directory');
  for (const name of files) {
    const file = await lstat(`${directory}/${name}`);
    assert.ok(file.isFile() && !file.isSymbolicLink() && file.size <= 16384, 'Each lab file is a bounded regular file');
    assert.equal(file.mode & 0o777, 0o600, 'Each lab file has owner-only access');
    assert.equal(file.uid, 65534, 'The main process owns each lab file');
  }
  const metadata = JSON.parse(await readFile(`${directory}/metadata.json`, 'utf8'));
  assert.equal(metadata.version, 1);
  assert.equal(metadata.synthetic, true);
  assert.equal(metadata.zone, 'windows.lab.invalid.');
  assert.ok((await readFile(`${directory}/window.private`, 'utf8')).includes('Comment: epochalypse-lab-only'));
  const certificate = new X509Certificate(await readFile(`${directory}/tls.crt`));
  assert.ok(certificate.validToDate > new Date(), 'The lab certificate remains valid');
}

await mkdir('/state', {recursive: true, mode: 0o700});
await chmod('/state', 0o700);
await chown('/state', 65534, 65534);
let existing = false;
try { await lstat(directory); existing = true; }
catch (error) { if (error.code !== 'ENOENT') throw error; }
if (existing) {
  await verifyExisting();
  console.log('Synthetic lab material remains unchanged');
} else {
  const staging = await mkdtemp('/state/.lab-setup-');
  try {
    assert.equal(process.version, 'v26.13.0', 'The pinned setup image supplies the verified Node version');
    const version = openssl(['version']);
    assert.ok(version.startsWith('OpenSSL 3.0.13 '), 'The pinned setup image supplies the verified OpenSSL version');
    const key = generateKeyPairSync('ec', {namedCurve: 'prime256v1'}).privateKey.export({format: 'jwk'});
    const publicKey = Buffer.concat([Buffer.from(key.x, 'base64url'), Buffer.from(key.y, 'base64url')]).toString('base64');
    await writeFile(`${staging}/window.private`, `Private-key-format: v1.3\nAlgorithm: 13 (ECDSAP256SHA256)\nComment: epochalypse-lab-only\nPrivateKey: ${Buffer.from(key.d, 'base64url').toString('base64')}\n`, {mode: 0o600, flag: 'wx'});
    await writeFile(`${staging}/window.key`, `windows.lab.invalid. 60 IN DNSKEY 257 3 13 ${publicKey}\n`, {mode: 0o600, flag: 'wx'});
    openssl(['req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:prime256v1', '-noenc', '-batch', '-days', '30', '-subj', '/CN=localhost', '-addext', 'subjectAltName=DNS:localhost,DNS:web,IP:127.0.0.1', '-keyout', `${staging}/tls.key`, '-out', `${staging}/tls.crt`]);
    await writeFile(`${staging}/password`, 'synthetic-lab-password\n', {mode: 0o600, flag: 'wx'});
    await writeFile(`${staging}/metadata.json`, JSON.stringify({version: 1, synthetic: true, zone: 'windows.lab.invalid.', node: process.version, openssl: version}) + '\n', {mode: 0o600, flag: 'wx'});
    for (const name of files) { await chmod(`${staging}/${name}`, 0o600); await chown(`${staging}/${name}`, 65534, 65534); }
    await chmod(staging, 0o700);
    await chown(staging, 65534, 65534);
    await rename(staging, directory);
    await verifyExisting();
    console.log('Synthetic DNSSEC keys and self-signed lab TLS are ready');
  } finally {
    // The script removes only the temporary directory that this invocation creates.
    await rm(staging, {recursive: true, force: true});
  }
}
