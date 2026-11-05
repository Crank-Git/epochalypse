import assert from 'node:assert/strict';

export async function checkCapsules(call, evaluate, waitFor, origin) {
  await call('Page.navigate', {url: origin + '/owner/capsules'});
  await waitFor("document.querySelector('#capsule-fields')?.disabled === false");
  assert.equal(await evaluate("document.querySelector('#trust').parentElement.textContent.includes('plaintext through the server')"), true);
  assert.equal(await evaluate("document.querySelector('#allowance').textContent.includes('32 bytes')"), true);
  assert.equal(await evaluate("document.querySelector('#mode option:last-child').disabled"), true);
  await evaluate("document.querySelector('#release-at').value = '2099-01-01T00:00:00Z'; document.querySelector('#content').value = 'x'.repeat(33); document.querySelector('#create').click()");
  await waitFor("document.querySelector('#capsule-status').textContent.includes('exceeds its byte limit')");
  assert.equal(await evaluate("document.querySelector('#draft').hidden"), true);
  await evaluate("document.querySelector('#content').value = 'x'.repeat(32); document.querySelector('#title').value = '<script>private</script>'; document.querySelector('#create').click()");
  await waitFor("document.querySelector('#capsule-status').textContent.includes('Private draft saved')");
  const held = await evaluate("JSON.parse(document.querySelector('#metadata').textContent)");
  assert.equal(held.plaintext_bytes, 32);
  assert.equal(held.published_at, null);
  assert.equal(await evaluate("document.querySelector('#metadata script') === null"), true);
  assert.equal(await evaluate("document.querySelector('#content').value"), '');
  const keyStatus = id => evaluate(`fetch('/api/v1/public/capsules/${id}/key').then(response => response.status)`);
  assert.equal(await keyStatus(held.id), 404);
  await evaluate("document.querySelector('#publish').click()");
  await waitFor("document.querySelector('#capsule-status').textContent.includes('server withholds its key')");
  assert.equal(await keyStatus(held.id), 425);
  await evaluate(`{
    document.querySelector('#mode').value = 'public';
    document.querySelector('#kind').value = 'file';
    document.querySelector('#kind').dispatchEvent(new Event('change'));
    const files = new DataTransfer();
    files.items.add(new File([new Uint8Array([0, 255, 128, 1])], 'synthetic.bin'));
    document.querySelector('#content-file').files = files.files;
    document.querySelector('#create').click();
  }`);
  await waitFor("document.querySelector('#capsule-status').textContent.includes('Private draft saved')");
  const publicDraft = await evaluate("JSON.parse(document.querySelector('#metadata').textContent)");
  assert.equal(publicDraft.kind, 'file');
  assert.equal(publicDraft.filename, 'synthetic.bin');
  assert.equal(publicDraft.plaintext_bytes, 4);
  await evaluate("document.querySelector('#publish').click()");
  await waitFor("document.querySelector('#capsule-status').textContent.includes('available immediately')");
  assert.equal(await keyStatus(publicDraft.id), 200);
  await call('Page.navigate', {url: origin + '/owner'});
  await waitFor("document.querySelector('h1')?.textContent === 'Signed in'");
}
