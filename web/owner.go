// Package web contains the owner interface assets.
package web

// DashboardHTML uses text fields so html/template escapes all object metadata.
const DashboardHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Owner dashboard — epochalypse</title><link rel="stylesheet" href="/owner/style.css"></head>
<body><a class="skip" href="#dashboard">Skip to dashboard</a><main id="dashboard">
<header><h1>Signed in</h1><p>Your owner session is active.</p><h2>Owner dashboard</h2></header>
<nav aria-label="Dashboard sections"><a href="#capsules">Capsules</a><a href="#switches">Switches</a><a href="#countdowns">Countdowns</a><a href="#agents">Agent health</a><a href="/owner">Refresh dashboard</a></nav>
<div class="sections">{{range .Sections}}<section id="{{.ID}}" aria-labelledby="{{.ID}}-heading">
<h2 id="{{.ID}}-heading">{{.Heading}}</h2><p class="state">State: {{.State}}</p><p>{{.Message}}</p>
{{if .Items}}<ul>{{range .Items}}<li><strong>{{.Name}}</strong> <span>{{.State}}</span> <small>{{.ID}}</small></li>{{end}}</ul>{{end}}
</section>{{end}}</div>
<p><a href="/owner/capsules">Create or publish a capsule</a>. <a href="/owner/countdowns">Create or inspect a countdown</a>. Use the local CLI to create a scoped token for API access.</p>
<form method="post" action="/owner/sign-out"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button type="submit">Sign out</button></form>
</main></body></html>`

// OwnerCSS keeps the dashboard readable with narrow screens and either color scheme.
const OwnerCSS = `:root{color-scheme:light dark;font-family:system-ui,sans-serif;line-height:1.6}body{margin:0;background:Canvas;color:CanvasText}main{max-width:72rem;margin:auto;padding:clamp(1rem,4vw,3rem)}h1,h2{line-height:1.2}nav{display:flex;flex-wrap:wrap;gap:1rem;margin-block:2rem}a{color:LinkText}a:focus-visible,button:focus-visible{outline:3px solid Highlight;outline-offset:4px}.sections{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,20rem),1fr));gap:1rem}section{border:1px solid GrayText;border-radius:.4rem;padding:1.2rem;overflow-wrap:anywhere}.state{font-weight:600}button{font:inherit;padding:.5rem 1rem;cursor:pointer}.skip{display:block;padding:.5rem}small{display:block}`
