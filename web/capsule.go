package web

// CapsuleHTML places the trust disclosures before any content control.
const CapsuleHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Create capsule — epochalypse</title><link rel="stylesheet" href="/owner/style.css"><script src="/owner/capsules.js" defer></script></head>
<body><main><a href="/owner">Owner dashboard</a><h1>Create capsule</h1>
<section aria-labelledby="trust"><h2 id="trust">Content and trust</h2>
<p>Web preparation passes plaintext through the server in v1. The server operator can read your content during preparation.</p>
<p>Public mode is public data after publication. Its requested date is advisory. Its key becomes available immediately.</p>
<p>Server-held mode trusts the server operator to keep the key private until release. A server outage can delay availability. A lost key has no automatic recovery.</p>
<p>CLI preparation uses a local file on the owner's machine. Local tlock preparation becomes available in a later release. Web tlock preparation remains unavailable.</p>
</section>
<p id="allowance" data-limit="{{.}}">Maximum plaintext: {{.}} bytes. Text must use UTF-8. Files remain opaque bytes.</p>
<noscript>Enable JavaScript to prepare and publish a capsule.</noscript>
<form id="create-capsule"><fieldset disabled id="capsule-fields"><legend>Private draft</legend>
<p><label for="mode">Mode</label> <select id="mode" required><option value="server-held">Server-held</option><option value="public">Public</option><option disabled>tlock — unavailable</option></select></p>
<p><label for="title">Title</label> <input id="title" maxlength="256"></p>
<p><label for="release-at">Requested release time (UTC, RFC 3339)</label> <input id="release-at" placeholder="2099-01-01T00:00:00Z" required></p>
<p><label for="kind">Content kind</label> <select id="kind"><option value="text">UTF-8 text</option><option value="file">Opaque file</option></select></p>
<p id="text-input"><label for="content">Text</label> <textarea id="content" rows="6"></textarea></p>
<p id="file-input" hidden><label for="content-file">File</label> <input id="content-file" type="file"></p>
<button id="create" type="submit">Create draft</button></fieldset></form>
<section id="draft" hidden aria-labelledby="draft-heading"><h2 id="draft-heading">Capsule metadata</h2><pre id="metadata"></pre>
<p>Publication exposes the manifest. Public mode also exposes its key immediately. Public copies remain outside your control.</p>
<button id="publish" type="button" disabled>Publish capsule</button></section>
<p id="capsule-status" role="status" aria-live="polite">Prepare a private draft before publication.</p>
</main></body></html>`

// CapsuleJS sends bounded content through the owner-authorized API.
const CapsuleJS = `'use strict';
const form = document.querySelector('#create-capsule');
const fields = document.querySelector('#capsule-fields');
const publish = document.querySelector('#publish');
const status = document.querySelector('#capsule-status');
const kind = document.querySelector('#kind');
const limit = Number(document.querySelector('#allowance').dataset.limit);
let proof;
let draft;
const showError = error => { status.textContent = error.message; };
async function request(path, options = {}) {
  const response = await fetch(path, {...options, credentials: 'same-origin', signal: AbortSignal.timeout(15000)});
  const body = await response.json();
  if (!response.ok) throw new Error(body.error?.message || 'Capsule access is unavailable.');
  return body;
}
function showMetadata(value) {
  document.querySelector('#draft').hidden = false;
  document.querySelector('#metadata').textContent = JSON.stringify(value, null, 2);
}
kind.addEventListener('change', () => {
  document.querySelector('#text-input').hidden = kind.value !== 'text';
  document.querySelector('#file-input').hidden = kind.value !== 'file';
});
form.addEventListener('submit', async event => {
  event.preventDefault();
  fields.disabled = true;
  const previousPublishDisabled = publish.disabled;
  publish.disabled = true;
  status.textContent = 'Prepare capsule…';
  try {
    const data = new FormData();
    for (const [name, value] of [['mode', document.querySelector('#mode').value], ['kind', kind.value], ['title', document.querySelector('#title').value], ['release_at', document.querySelector('#release-at').value]]) data.append(name, value);
    if (kind.value === 'file') {
      const file = document.querySelector('#content-file').files[0];
      if (!file) throw new Error('Choose a file.');
      if (file.size > limit) throw new Error('The capsule content exceeds its byte limit.');
      data.append('content', file);
    } else {
      const text = document.querySelector('#content').value;
      if (new TextEncoder().encode(text).length > limit) throw new Error('The capsule content exceeds its byte limit.');
      data.append('content', text);
    }
    const value = await request('/api/v1/capsules', {method: 'POST', headers: {'X-CSRF-Token': proof}, body: data});
    draft = value.id;
    showMetadata(value);
    document.querySelector('#content').value = '';
    document.querySelector('#content-file').value = '';
    publish.disabled = false;
    status.textContent = 'Private draft saved. Review the mode before publication.';
  } catch (error) { showError(error); publish.disabled = previousPublishDisabled; }
  finally { fields.disabled = false; }
});
publish.addEventListener('click', async () => {
  publish.disabled = true;
  fields.disabled = true;
  try {
    const value = await request('/api/v1/capsules/' + draft + '/publish', {method: 'POST', headers: {'X-CSRF-Token': proof}});
    showMetadata(value);
    status.textContent = value.mode === 'public' ? 'Published. The public-mode key is available immediately.' : 'Published. The server withholds its key until ' + value.effective_release_at + '. An outage can delay availability.';
  } catch (error) { showError(error); publish.disabled = false; }
  finally { fields.disabled = false; }
});
request('/api/v1/csrf').then(value => { proof = value.csrf_token; fields.disabled = false; }).catch(showError);
`
