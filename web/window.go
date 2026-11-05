package web

// WindowHTML keeps public DNS evidence separate from resolver observations.
const WindowHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Public DNS window — epochalypse</title><link rel="stylesheet" href="/windows/style.css"><script src="/windows/results.js" defer></script></head>
<body><a href="#results" class="skip">Skip to results</a><main id="results" data-window="{{.Window.ID}}">
<h1>Public DNS window</h1><aside><h2>Public data</h2><p>A future DNSSEC signature does not keep content secret. Direct authoritative access can expose TXT content before inception.</p><p>SERVFAIL and AD do not establish confidentiality. Resolver results are observations. Acceptance exactly at inception is not guaranteed.</p></aside>
<h2>Immutable window</h2><dl><dt>Name</dt><dd>{{.Window.Name}}</dd><dt>Inception (UTC)</dt><dd>{{.Window.Inception}}</dd><dt>Expiration (UTC)</dt><dd>{{.Window.Expiration}}</dd></dl>
<h3>Public TXT content</h3><pre id="content">{{.Window.Content}}</pre><h3>Stored TXT and RRSIG</h3><pre>{{.Window.TXT}}
{{.Window.RRSIG}}</pre>
<section aria-labelledby="direct-heading"><h2 id="direct-heading">Direct authority</h2><p id="direct-state">State: {{.Direct.State}}</p><p>Check time (UTC): {{.Direct.At}}. Transport: {{if .Direct.Transport}}{{.Direct.Transport}}{{else}}unavailable{{end}}.</p>
{{if eq .Direct.State "observed"}}<p>The main listener returns the stored TXT and RRSIG. This is direct public access, without recursive validation.</p>{{else}}<p>The live query is unavailable. Stored records alone do not prove a live DNS response. Refresh to retry.</p>{{end}}
{{range .Direct.Records}}<pre class="direct-record">{{.}}</pre>{{end}}</section>
<section aria-labelledby="resolver-heading"><h2 id="resolver-heading">Resolver observations</h2><p>A complete run means every resolver returns a response. SERVFAIL can be complete. Partial runs contain unavailable results.</p>
<p><a href="/owner">Owner sign-in</a> is required to start a comparison.</p><button id="compare" type="button">Run resolver comparison</button><button id="refresh" type="button">Refresh observations</button>
<p id="status" role="status" aria-live="polite">Load observations with JavaScript, or read the <a href="/api/v1/windows/{{.Window.ID}}/observations">public JSON</a>.</p><div id="runs"></div></section></main></body></html>`

// WindowCSS provides visible keyboard focus and wraps DNS records on narrow screens.
const WindowCSS = `:root{color-scheme:light dark;font:1rem/1.6 system-ui,sans-serif}body{margin:0;background:Canvas;color:CanvasText}main{max-width:65rem;margin:auto;padding:1.5rem}aside,section{border:1px solid GrayText;padding:1rem;margin-block:1rem}aside{border-inline-start:6px solid Highlight}pre{white-space:pre-wrap;overflow-wrap:anywhere}dd{margin-inline-start:1rem;overflow-wrap:anywhere}button{font:inherit;margin:.3rem;padding:.5rem 1rem}a{color:LinkText}a:focus-visible,button:focus-visible{outline:3px solid Highlight;outline-offset:4px}.skip{display:block;padding:.5rem}article{border-block-start:1px solid GrayText;padding-block:1rem}`

// WindowJS follows bounded observation pages to the accepted opaque run identifier.
const WindowJS = `'use strict';
(() => {
 const root = document.querySelector('[data-window]');
 const endpoint = '/api/v1/windows/' + root.dataset.window + '/observations';
 const status = document.querySelector('#status');
 const runs = document.querySelector('#runs');
 const compare = document.querySelector('#compare');
 const refresh = document.querySelector('#refresh');
 let busy = false;
 const text = (parent, tag, value) => {
  const node = document.createElement(tag);
  node.textContent = value;
  parent.append(node);
 };
 const show = run => {
  const article = document.createElement('article');
  text(article, 'h3', 'Run ' + run.id + ': ' + run.state);
  text(article, 'p', 'Started (UTC): ' + run.started + '; completed (UTC): ' + (run.completed ?? 'unavailable'));
  for (const result of run.results ?? []) {
   const target = result.resolver;
   text(article, 'h4', target.id);
   text(article, 'p', 'Version: ' + target.version + ' (' + target.version_source + '); clock offset (nanoseconds): ' + (target.clock.offset_nanoseconds ?? 'unknown') + ' (' + target.clock.source + ')');
   text(article, 'p', 'Response code: ' + (result.rcode ?? 'unavailable') + (result.rcode === 2 ? ' (SERVFAIL)' : '') + '; transport: ' + (result.transport || 'unavailable') + '; error: ' + (result.error || 'none'));
   text(article, 'p', 'Started (UTC): ' + result.started + '; completed (UTC): ' + result.completed);
   text(article, 'p', result.flags === null ? 'Flags: unavailable' : 'Flags: ' + Object.entries(result.flags).map(([key, value]) => key.toUpperCase() + '=' + value).join(', '));
  }
  runs.append(article);
 };
 const json = async (path, options = {}) => {
  const response = await fetch(path, {...options, credentials: 'same-origin', signal: AbortSignal.timeout(4000)});
  if (!response.ok) throw new Error(response.status === 401 ? 'Sign in as the owner to start a comparison.' : 'Request unavailable (' + response.status + ').');
  return response.json();
 };
 const pages = async accepted => {
  let cursor = '';
  const seen = new Set();
  const collected = [];
  for (let page = 0; page < 10; page++) {
   const data = await json(endpoint + (cursor ? '?after=' + encodeURIComponent(cursor) : ''));
   if (data.version !== 1 || !Array.isArray(data.runs) || data.runs.length > 50) throw new Error('Unsupported observation page.');
   collected.push(...data.runs);
   const found = collected.find(run => run.id === accepted);
   if (found || !data.next_cursor) {
    runs.replaceChildren(); collected.forEach(show);
    return {found, count: collected.length, delay: Math.min(5, Math.max(1, data.poll_after_seconds || 1))};
   }
   if (!/^[a-f0-9]{32}$/.test(data.next_cursor) || seen.has(data.next_cursor)) throw new Error('Invalid observation cursor.');
   cursor = data.next_cursor; seen.add(cursor);
  }
  throw new Error('Observation page limit reached. Read public JSON for more runs.');
 };
 const load = async accepted => {
  for (let attempt = 0; attempt < 20; attempt++) {
   const result = await pages(accepted);
   if (!accepted) { status.textContent = result.count ? 'Retained observations.' : 'No resolver observations yet.'; return; }
   if (result.found && ['complete', 'partial'].includes(result.found.state)) {
    status.textContent = 'Run ' + result.found.state + '. Results are observations, not a validation guarantee.'; return;
   }
   status.textContent = 'Run pending. Waiting for the accepted run.';
   await new Promise(resolve => setTimeout(resolve, result.delay * 1000));
  }
  throw new Error('Polling limit reached. Refresh observations to retry.');
 };
 const act = async start => {
  if (busy) return;
  busy = true; compare.disabled = true; refresh.disabled = true;
  try {
   let accepted;
   if (start) {
    status.textContent = 'Request pending.';
    const proof = await json('/api/v1/csrf');
    const run = await json(endpoint, {method: 'POST', headers: {'X-CSRF-Token': proof.csrf_token}});
    if (run.version !== 1 || !/^[a-f0-9]{32}$/.test(run.id)) throw new Error('Unsupported accepted run.');
    accepted = run.id;
   }
   await load(accepted);
  } catch (error) { status.textContent = error.message; }
  finally { busy = false; compare.disabled = false; refresh.disabled = false; }
 };
 compare.addEventListener('click', () => act(true));
 refresh.addEventListener('click', () => act(false));
 act(false);
})();`
