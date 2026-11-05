import assert from 'node:assert/strict';

export async function checkSwitches(call, evaluate, waitFor, origin) {
  await call('Page.navigate', {url: origin + '/owner/switches'});
  await waitFor("document.querySelector('#switch-fields')?.disabled === false");
  assert.equal(await evaluate("document.querySelector('#switch-interval').value"), '86400');
  assert.equal(await evaluate("document.querySelector('#switch-grace').value"), '3600');
  const preview = await evaluate("document.querySelector('#switch-preview').textContent");
  for (const text of ['Deadline UTC:', 'Deadline local:', 'Grace: 3600 seconds.', 'Release boundary UTC:', 'Release boundary local:', 'Preview only']) assert.ok(preview.includes(text), text);
  await evaluate("document.querySelector('#switch-grace').value='0'; document.querySelector('#switch-grace').dispatchEvent(new Event('input')); document.querySelector('button').focus()");
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key: ' ', code: 'Space', windowsVirtualKeyCode: 32});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key: ' ', code: 'Space', windowsVirtualKeyCode: 32});
  await waitFor("document.querySelector('#switch-status').textContent.startsWith('Draft created.')");
  const id = await evaluate("document.querySelector('#switch-list li').dataset.id");
  const record = await evaluate(`fetch('/api/v1/switches/${id}').then(response => response.json())`);
  assert.equal(record.state, 'draft'); assert.equal(record.interval_seconds, 86400); assert.equal(record.grace_seconds, 0);
  assert.equal(record.deadline, undefined); assert.equal(record.release_boundary, undefined);
  await call('Page.reload');
  await waitFor("document.querySelector('#switch-fields')?.disabled === false");
  assert.ok(await evaluate("document.querySelector('#switch-list').textContent.includes('unarmed and unreplicated')"));
  assert.ok(await evaluate("document.querySelector('#switch-list').textContent.includes('Grace: 0 seconds')"));
  await call('Page.navigate', {url: origin + '/owner'});
  await waitFor("document.querySelector('h1')?.textContent === 'Signed in'");
}
