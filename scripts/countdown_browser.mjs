import assert from 'node:assert/strict';
import {checkPublicCountdown} from './public_countdown_browser.mjs';

export async function checkCountdowns(call, evaluate, waitFor, origin) {
  await call('Page.navigate', {url: origin + '/owner/countdowns'});
  await waitFor("document.querySelector('#countdown-fields')?.disabled === false");
  assert.equal(await evaluate("document.querySelector('label[for=countdown-event]').textContent"), 'Event time (UTC)');
  await evaluate("document.querySelector('#countdown-title').value = '<script>public event</script>'; document.querySelector('#countdown-event').value = '2000-01-01T00:00:00Z'; document.querySelector('#countdown-create').click()");
  await waitFor("document.querySelector('#countdown-status').textContent.includes('future event_at')");
  assert.equal(await evaluate("document.querySelector('#countdown-list').children.length"), 0);
  const event = new Date(Date.now() + 3600000).toISOString().replace(/\.\d{3}Z$/, 'Z');
  await evaluate(`document.querySelector('#countdown-event').value = ${JSON.stringify(event)}; document.querySelector('#countdown-create').focus()`);
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key: ' ', code: 'Space', windowsVirtualKeyCode: 32});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key: ' ', code: 'Space', windowsVirtualKeyCode: 32});
  try { await waitFor("document.querySelector('#countdown-status').textContent.startsWith('Countdown created.')"); }
  catch (error) { throw new Error(error.message + ': ' + await evaluate("JSON.stringify({status: document.querySelector('#countdown-status').textContent, focus: document.activeElement.id, event: document.querySelector('#countdown-event').value, valid: document.querySelector('#countdown-form').checkValidity()})")); }
  const id = await evaluate("document.querySelector('#countdown-list li').dataset.id");
  const record = await evaluate(`fetch('/api/v1/countdowns/${id}').then(response => response.json())`);
  assert.equal(await evaluate("document.querySelector('#countdown-list a').getAttribute('href')"), '/events/' + id);
  assert.equal(record.event_at, event);
  assert.equal(record.name, 'c' + id + '.countdown.example.');
  assert.equal(await evaluate("document.querySelector('#countdown-list script') === null"), true);
  assert.equal(await evaluate("document.querySelector('#countdown-list strong').textContent"), '<script>public event</script>');
  await checkPublicCountdown(call, evaluate, waitFor, origin, record);
  await call('Page.navigate', {url: origin + '/owner/countdowns'});
  await waitFor("document.querySelector('#countdown-fields')?.disabled === false");
  await call('Page.reload');
  await waitFor("document.querySelector('#countdown-fields')?.disabled === false");
  assert.equal(await evaluate("document.querySelector('#countdown-list strong').textContent"), '<script>public event</script>');
  await evaluate("document.querySelector('[data-delete]').click()");
  await waitFor("document.querySelector('#countdown-status').textContent.startsWith('Countdown deleted.')");
  assert.equal(await evaluate(`fetch('/api/v1/countdowns/${id}').then(response => response.status)`), 404);
  await evaluate(`document.querySelector('#countdown-title').value = 'Retained browser event'; document.querySelector('#countdown-event').value = ${JSON.stringify(event)}; document.querySelector('#countdown-create').click()`);
  await waitFor("document.querySelector('#countdown-status').textContent.startsWith('Countdown created.')");
  const retained = await evaluate("fetch('/api/v1/countdowns/' + document.querySelector('#countdown-list li').dataset.id).then(response => response.json())");
  await call('Page.navigate', {url: origin + '/owner'});
  await waitFor("document.querySelector('h1')?.textContent === 'Signed in'");
  return retained;
}
