import assert from 'node:assert/strict';

export async function checkPublicWindow({call, evaluate, waitFor, origin, record, observed}) {
  await call('Page.navigate', {url: origin + '/windows/' + record.id});
  await waitFor("document.querySelector('#status')?.textContent === 'Retained observations.'");
  assert.equal(await evaluate("document.querySelector('h1').textContent"), 'Public DNS window');
  assert.ok(await evaluate("document.body.textContent.includes('Public data') && document.body.textContent.includes('does not keep content secret')"));
  assert.equal(await evaluate("document.querySelector('#direct-state').textContent"), 'State: observed');
  assert.ok(Date.now() < Date.parse(record.inception), 'The public page exposes the records before inception');
  const records = await evaluate("Array.from(document.querySelectorAll('.direct-record'), node => node.textContent)");
  assert.deepEqual(records, [record.txt, record.rrsig]);
  assert.equal(await evaluate("document.querySelector('#content').textContent"), record.content);
  assert.equal(await evaluate("document.querySelector('#content img')"), null);
  assert.equal(await evaluate('globalThis.publicWindowXSS'), undefined);
  assert.ok(await evaluate("document.querySelector('#runs').textContent.includes('partial') && document.querySelector('#runs').textContent.includes('Flags: unavailable') && document.querySelector('#runs').textContent.includes('unknown')"));
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9});
  assert.equal(await evaluate('document.activeElement.textContent'), 'Skip to results');
  assert.equal(await evaluate('getComputedStyle(document.activeElement).outlineStyle'), 'solid');
  for (let i = 0; i < 2; i++) {
    await call('Input.dispatchKeyEvent', {type: 'keyDown', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9});
    await call('Input.dispatchKeyEvent', {type: 'keyUp', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9});
  }
  assert.equal(await evaluate('document.activeElement.id'), 'compare');
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13, text: '\r'});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13});
  try { await waitFor("document.querySelector('#status').textContent.startsWith('Run partial.')"); }
  catch (error) { throw new Error(await evaluate("document.querySelector('#status').textContent"), {cause:error}); }
  // Controlled responses exercise states that the unavailable resolver cannot produce.
  await evaluate(`globalThis.savedFetch = fetch; globalThis.fixture = ${JSON.stringify(observed)};
    fixture.id = 'ffffffffffffffffffffffffffffffff'; fixture.state = 'pending'; fixture.results = [];
    globalThis.pageReads = 0;
    globalThis.fetch = async (path, options = {}) => {
      if (!String(path).includes('/observations')) return savedFetch(path, options);
      if (options.method === 'POST') return Response.json(fixture, {status:202});
      pageReads++;
      if (!String(path).includes('?after=')) return Response.json({version:1,runs:[],next_cursor:'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',poll_after_seconds:1});
      return Response.json({version:1,runs:[fixture],next_cursor:'',poll_after_seconds:1});
    }; document.querySelector('#compare').click();`);
  await waitFor("document.querySelector('#status').textContent.includes('Run pending.')");
  await evaluate(`fixture.state = 'complete'; fixture.results = ${JSON.stringify(observed.results)};
    fixture.results[0].rcode = 2; fixture.results[0].error = '';
    fixture.results[0].resolver.id = '<img src=x onerror="globalThis.publicWindowXSS=1">';
    fixture.results[0].flags = {ad:false,cd:false,aa:false,ra:true,rd:true,tc:false,do:true};`);
  await waitFor("document.querySelector('#status').textContent.startsWith('Run complete.')");
  assert.ok(await evaluate("pageReads >= 4 && document.querySelector('#runs').textContent.includes('SERVFAIL') && document.querySelector('#runs').textContent.includes('AD=false')"));
  assert.equal(await evaluate("document.querySelector('#runs img')"), null);
  assert.equal(await evaluate('globalThis.publicWindowXSS'), undefined);
  await evaluate("globalThis.fetch = async () => new Response('', {status:503}); document.querySelector('#refresh').click()");
  await waitFor("document.querySelector('#status').textContent.includes('503')");
  await evaluate("globalThis.fetch = savedFetch");
  await call('Page.navigate', {url: origin + '/owner'});
  await waitFor("document.querySelector('h1')?.textContent === 'Signed in'");
}
