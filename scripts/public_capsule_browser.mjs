import assert from 'node:assert/strict';

export async function checkPublicCapsules(call, evaluate, waitFor, origin) {
  await call('Page.navigate', {url: origin + '/fixture'});
  await waitFor("document.body?.textContent.includes('server-held')");
  const ids = await evaluate('JSON.parse(document.body.textContent)');
  const visit = async name => {
    await call('Page.navigate', {url: origin + '/c/' + ids[name]});
    await waitFor("document.querySelector('#availability-state') !== null");
  };
  const state = () => evaluate("document.querySelector('#availability-state').textContent");
  const control = name => evaluate(`fetch('/fixture/${name}', {method: 'POST'}).then(r => r.status)`);
  const download = name => evaluate(`fetch('/api/v1/public/capsules/${ids[name]}/download').then(async r => ({status:r.status, type:r.headers.get('Content-Type'), disposition:r.headers.get('Content-Disposition'), body:await r.text()}))`);
  for (const mode of ['public', 'server-held', 'tlock']) {
    await visit(mode);
    assert.match(await evaluate("document.querySelector('#trust').textContent"), new RegExp(mode));
    assert.equal(await evaluate("document.querySelector('h1').textContent"), '<script>fixture</script>');
    assert.equal(await evaluate('globalThis.uploadRan'), undefined);
    assert.equal(await evaluate("document.querySelector('script')"), null);
    assert.match(await evaluate('document.body.textContent'), /Requested release.*Effective release/s);
    if (mode === 'public') {
      assert.match(await state(), /Verified and available/);
      assert.match(await evaluate("document.querySelector('#preview').textContent"), /<script>/);
    } else {
      assert.match(await state(), /Sealed/);
      assert.equal(await evaluate("document.querySelector('#download')"), null);
      assert.equal((await download(mode)).status, 425);
    }
  }
  assert.equal(await control('due'), 204);
  for (const mode of ['server-held', 'tlock']) {
    await visit(mode);
    assert.match(await state(), /Verified and available/);
    assert.match(await evaluate("document.querySelector('#release-state').textContent"), /Release condition is due/);
    assert.equal((await download(mode)).status, 200);
  }
  assert.equal(await control('delayed'), 204);
  await visit('tlock');
  assert.match(await state(), /Delayed availability/);
  assert.match(await evaluate("document.querySelector('#release-state').textContent"), /Release condition is due/);
  assert.equal(await evaluate("document.querySelector('#download')"), null);
  assert.equal((await download('tlock')).status, 503);
  assert.equal(await control('invalid-beacon'), 204);
  await visit('tlock');
  assert.match(await state(), /Integrity error/);
  assert.equal((await download('tlock')).body.includes('uploadRan'), false);
  for (const name of ['corrupt', 'lost']) {
    assert.equal(await control(name), 204);
    await visit(name);
    assert.match(await state(), name === 'corrupt' ? /Integrity error/ : /Delayed availability/);
    assert.equal(await evaluate("document.querySelector('#download')"), null);
    const failed = await download(name);
    assert.equal(failed.status, 503);
    assert.equal(failed.body.includes('uploadRan'), false);
  }
  await visit('file');
  assert.equal(await evaluate("document.querySelector('#preview')"), null);
  const file = await download('file');
  assert.equal(file.type, 'application/octet-stream');
  assert.match(file.disposition, /attachment; filename=active.html/);
  assert.equal(file.body, '<script>globalThis.uploadRan=true</script>');
  await evaluate("document.querySelector('#download').focus()");
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9});
  assert.equal(await evaluate('document.activeElement.id'), 'bundle');
  assert.equal(await evaluate('getComputedStyle(document.activeElement).outlineStyle'), 'solid');
  assert.equal(await evaluate("document.querySelector('#availability-state').getAttribute('role')"), 'status');
  await call('Emulation.setDeviceMetricsOverride', {width: 360, height: 740, deviceScaleFactor: 1, mobile: true});
  assert.equal(await evaluate('document.documentElement.scrollWidth <= innerWidth'), true);
  await call('Emulation.clearDeviceMetricsOverride');
  await visit('preview');
  assert.equal(await evaluate("new TextEncoder().encode(document.querySelector('#preview').textContent).length"), 4095);
  const headers = await evaluate("fetch(location.href).then(r => ({csp:r.headers.get('Content-Security-Policy'),cache:r.headers.get('Cache-Control'),nosniff:r.headers.get('X-Content-Type-Options')}))");
  assert.match(headers.csp, /default-src 'none'/);
  assert.match(headers.csp, /style-src 'self'/);
  assert.equal(headers.cache, 'no-store');
  assert.equal(headers.nosniff, 'nosniff');
}
