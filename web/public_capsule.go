package web

// PublicCapsuleHTML separates release eligibility from authenticated retrieval.
const PublicCapsuleHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Public capsule — epochalypse</title><link rel="stylesheet" href="/c/style.css"></head>
<body><a class="skip" href="#capsule">Skip to capsule</a><main id="capsule"><h1>{{if .Manifest.Title}}{{.Manifest.Title}}{{else}}Public capsule{{end}}</h1>
<section aria-labelledby="trust"><h2 id="trust">Selected mode: {{.Manifest.Mode}}</h2><p>{{.Trust}}</p>
<p>Public copies remain outside the owner's control. Local deletion cannot recall published copies.</p></section>
<section aria-labelledby="release"><h2 id="release">Release time</h2><dl><dt>Requested release (UTC)</dt><dd>{{.Requested}}</dd><dt>Effective release (UTC)</dt><dd>{{.Effective}}</dd><dt>Server check (UTC)</dt><dd>{{.Checked}}</dd></dl><p id="release-state">{{.Release}}</p><p>The browser clock does not authorize release.</p></section>
<section aria-labelledby="availability"><h2 id="availability">Content availability</h2><p id="availability-state" role="status" aria-live="polite">{{.Availability}}</p>
{{if .Available}}<p><a id="download" href="/api/v1/public/capsules/{{.Manifest.ID}}/download">Download content</a> (opaque attachment, {{.Manifest.PlaintextBytes}} bytes)</p>
{{if .Text}}<h3>Text preview (at most 4096 bytes)</h3><pre id="preview">{{.Preview}}</pre>{{end}}{{end}}
<p>Each download authenticates the complete content before it returns plaintext. Availability at this check does not guarantee later retrieval.</p>
<p><a id="bundle" href="/api/v1/public/capsules/{{.Manifest.ID}}/bundle">Download recovery bundle</a>. An unreleased server-held bundle excludes its key.</p>
<p><a href="/c/{{.Manifest.ID}}">Refresh availability</a></p></section></main></body></html>`
