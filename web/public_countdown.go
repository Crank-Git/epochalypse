package web

// PublicCountdownHTML preserves the absolute event when time synchronization fails.
const PublicCountdownHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Public countdown — epochalypse</title><link rel="stylesheet" href="/events/style.css"><script src="/events/countdown.js" defer></script></head>
<body><a class="skip" href="#event">Skip to countdown</a><main id="event" tabindex="-1" data-id="{{.ID}}"><h1>{{.Title}}</h1>
<section aria-labelledby="event-heading"><h2 id="event-heading">Public countdown</h2>
<p>Event time (UTC): <time id="event-at" datetime="{{.EventAt.Format "2006-01-02T15:04:05Z"}}">{{.EventAt.Format "2006-01-02T15:04:05Z"}}</time></p>
<p id="local-event"></p><p>DNS name: <code id="dns-name">{{.Name}}</code></p>
<p id="event-state" role="status">{{.State}} at the server page check.</p><p id="remaining">Time unavailable. Synchronize with the server.</p>
<p id="time-status" role="status">Time unavailable. Synchronize with the server.</p><button id="sync-time" type="button">Synchronize time</button>
<noscript><p>Enable JavaScript for a live countdown. Reload for the server event state.</p></noscript></section>
<section aria-labelledby="dns-heading"><h2 id="dns-heading">Advisory DNS TTL</h2>
<p>Recursive TTL values are advisory. A recursive resolver can cap TTL, prefetch, or serve stale data.</p>
<p>The countdown uses the absolute event time and a server clock sample. DNS TTL is not a security boundary.</p>
<p>Completion describes the event time. It does not promise exact recursive TTL values.</p></section></main></body></html>`

// PublicCountdownJS invalidates clock samples after suspension or a failed request.
const PublicCountdownJS = `'use strict';
(() => {
 const root = document.querySelector('#event');
 const eventTime = Date.parse(document.querySelector('#event-at').dateTime);
 const remaining = document.querySelector('#remaining');
 const state = document.querySelector('#event-state');
 const status = document.querySelector('#time-status');
 const button = document.querySelector('#sync-time');
 document.querySelector('#local-event').textContent = 'Event time (local): ' + new Date(eventTime).toLocaleString();
 let sample = null;
 let busy = false;
 let generation = 0;
 let retry = false;
 let previous = performance.now();
 let wall = Date.now();
 function unavailable(message) {
  sample = null;
  state.textContent = 'Event state unavailable.';
  remaining.textContent = 'Time unavailable.';
  status.textContent = message;
 }
 function render() {
  if (!sample) return;
  const delta = eventTime - (sample.server + performance.now() - sample.local);
  const label = delta <= 0 ? 'Completed' : 'Active';
  if (state.textContent !== label) state.textContent = label;
  remaining.textContent = Math.max(0, Math.floor(delta / 1000)) + ' whole seconds remain.';
 }
 async function sync() {
  const current = ++generation;
  unavailable('Synchronize time with the server.');
  if (document.hidden) return;
  if (busy) { retry = true; return; }
  busy = true; button.disabled = true;
  try {
   const response = await fetch('/api/v1/public/countdowns/' + root.dataset.id, {cache:'no-store', signal:AbortSignal.timeout(10000)});
   if (!response.ok) throw new Error('Time unavailable. Retry synchronization.');
   const text = await response.text();
   if (text.length > 4096) throw new Error('Invalid server time. Retry synchronization.');
   const record = JSON.parse(text);
   if (current !== generation) return;
   const server = Date.parse(record.server_now);
   if (record.id !== root.dataset.id || Date.parse(record.event_at) !== eventTime || !Number.isFinite(server)) throw new Error('Invalid server time. Retry synchronization.');
   sample = {server, local:performance.now()};
   previous = performance.now(); wall = Date.now();
   status.textContent = 'Time synchronized with the server. Network delay can affect this advisory display.';
   render();
  } catch { if (current === generation) unavailable('Time unavailable. Retry synchronization.'); }
  finally {
   busy = false; button.disabled = false;
   if (retry) { retry = false; sync(); }
  }
 }
 button.addEventListener('click', sync);
 document.addEventListener('visibilitychange', () => {
  generation++;
  unavailable('Time unavailable after page suspension.');
  if (!document.hidden) sync();
 });
 window.addEventListener('pageshow', () => sync());
 window.addEventListener('focus', () => sync());
 setInterval(() => {
  const now = performance.now(); const date = Date.now();
  // Some browsers pause the monotonic clock during operating-system sleep.
  const suspended = now - previous > 2000 || Math.abs((date - wall) - (now - previous)) > 2000;
  previous = now; wall = date;
  if (suspended) { sync(); return; }
  render();
 }, 100);
 sync();
})();`
