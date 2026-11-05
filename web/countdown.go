package web

// CountdownHTML escapes retained metadata before the owner inspects an event.
const CountdownHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Owner countdowns — epochalypse</title><link rel="stylesheet" href="/owner/style.css"><script src="/owner/countdowns.js" defer></script></head>
<body><main><a href="/owner">Owner dashboard</a><h1>Create a countdown</h1>
<p>Enter an absolute event time in UTC. The service keeps the event time after completion until you delete it.</p>
<form id="countdown-form"><fieldset id="countdown-fields" disabled><legend>Public event</legend>
<p><label for="countdown-title">Title</label> <input id="countdown-title" name="title" required maxlength="128"></p>
<p><label for="countdown-event">Event time (UTC)</label> <input id="countdown-event" name="event_at" required placeholder="2026-12-01T12:00:00Z" aria-describedby="event-format"></p>
<p id="event-format">Use YYYY-MM-DDTHH:MM:SSZ. The event must be in the future and within 2147483647 seconds.</p>
<button id="countdown-create" type="submit">Create countdown</button></fieldset></form>
<p id="countdown-status" role="status" tabindex="-1">Check the owner session.</p>
<section aria-labelledby="retained-heading"><h2 id="retained-heading">Retained countdowns</h2>
<p>Deletion removes local metadata. DNS caches can retain public event data.</p>
<ul id="countdown-list">{{range .}}<li data-id="{{.ID}}"><strong>{{.Title}}</strong><p>Event: <time>{{.EventAt.Format "2006-01-02T15:04:05Z"}}</time></p><p>DNS name: <code>{{.Name}}</code></p><button type="button" data-delete="{{.ID}}">Delete countdown</button></li>{{end}}</ul>
</section></main></body></html>`

// CountdownJS uses session CSRF proof for each bounded mutation.
const CountdownJS = `'use strict';
(() => {
  const form = document.querySelector('#countdown-form');
  const fields = document.querySelector('#countdown-fields');
  const status = document.querySelector('#countdown-status');
  const list = document.querySelector('#countdown-list');
  let busy = false;
  const say = message => { status.textContent = message; };
  async function request(path, options = {}) {
    const response = await fetch(path, {...options, signal: AbortSignal.timeout(10000), cache: 'no-store'});
    if (response.status === 204) return null;
    const body = await response.json();
    if (!response.ok) throw new Error(body.error?.message || 'The countdown request fails.');
    return body;
  }
  function append(record) {
    const item = document.createElement('li');
    item.dataset.id = record.id;
    const title = document.createElement('strong'); title.textContent = record.title;
    const event = document.createElement('p'); event.textContent = 'Event: ' + record.event_at;
    const name = document.createElement('p'); name.textContent = 'DNS name: ' + record.name;
    const button = document.createElement('button'); button.type = 'button'; button.dataset.delete = record.id; button.textContent = 'Delete countdown';
    item.append(title, event, name, button); list.append(item);
  }
  async function mutate(path, method, body) {
    if (busy) return;
    busy = true; fields.disabled = true; say('The countdown request is pending.');
    try {
      const proof = await request('/api/v1/csrf');
      const headers = {'X-CSRF-Token': proof.csrf_token};
      if (body) headers['Content-Type'] = 'application/json';
      const record = await request(path, {method, headers, body: body ? JSON.stringify(body) : undefined});
      if (method === 'POST') { append(record); say('Countdown created. Event: ' + record.event_at + '. DNS name: ' + record.name); }
      else { document.querySelector('[data-id="' + path.split('/').pop() + '"]')?.remove(); say('Countdown deleted. Public DNS caches can retain the event.'); }
    } catch (error) { say(error.message || 'The countdown service is unavailable.'); }
    finally { busy = false; fields.disabled = false; }
  }
  form.addEventListener('submit', event => {
    event.preventDefault();
    mutate('/api/v1/countdowns', 'POST', {title: document.querySelector('#countdown-title').value, event_at: document.querySelector('#countdown-event').value});
  });
  list.addEventListener('click', event => {
    const id = event.target.dataset.delete;
    if (id) mutate('/api/v1/countdowns/' + id, 'DELETE');
  });
  request('/api/v1/csrf').then(() => { fields.disabled = false; say('Enter the public event time.'); }).catch(() => say('The owner session is unavailable. Reload this page.'));
})();`
