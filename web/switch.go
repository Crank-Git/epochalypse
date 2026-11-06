package web

// SwitchHTML distinguishes retained observations from independent agent authority.
const SwitchHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Owner switches — epochalypse</title><link rel="stylesheet" href="/owner/style.css"><script src="/owner/switches.js" defer></script></head>
<body><main><a href="/owner">Owner dashboard</a><h1>Configure a switch draft</h1>
<p>Switch production use remains disabled. The owner signs arm requests with a local key through the CLI. Export the recovery bundle before arm.</p>
<p>A network partition can trigger irreversible release despite an available owner. Missing receipts leave an uncertain outcome.</p>
<form id="switch-form"><fieldset id="switch-fields" disabled><legend>Draft configuration</legend>
<p><label for="switch-interval">Check-in interval (seconds)</label> <input id="switch-interval" type="number" min="3600" max="31536000" step="1" value="86400" required></p>
<p><label for="switch-grace">Grace period (seconds)</label> <input id="switch-grace" type="number" min="0" max="2592000" step="1" value="3600" required></p>
<p>Interval: 1 hour to 365 days. Grace: zero to 30 days.</p>
<p id="switch-preview" aria-live="polite"></p><button type="submit">Create draft</button></fieldset></form>
<p id="switch-status" role="status" tabindex="-1">Check the owner session.</p>
<section><h2>Retained switches</h2><p>These records show retained evidence. An absent receipt does not prove that an agent rejects an arm request.</p><ul id="switch-list">{{range .}}{{$id := .ID}}<li data-id="{{$id}}"><code>{{$id}}</code>{{if not .RenewalStatus}}{{if .Arm}}<p>{{if eq .State "armed"}}Armed — three matching durable receipts{{else}}Uncertain outcome — the owner issues an arm request{{end}}</p><p>Armed receipts: {{len .Arm.Armed}} of 3.</p>{{else}}<p>Draft — no arm evidence reported. Remote agent state is unknown.</p>{{end}}{{end}}<p>Interval: {{.IntervalSeconds}} seconds. Grace: {{.GraceSeconds}} seconds.</p>{{with .RenewalStatus}}<section class="renewal-status" aria-label="Renewal evidence"><h3>Latest renewal</h3>{{if .Confirmed}}<p class="renewal-outcome">Renewed — three matching durable receipts confirm the request.</p>{{else}}<p class="renewal-outcome">Uncertain outcome — incomplete receipts do not prove rejection.</p>{{end}}<p>B0: <time class="renewal-time" data-unix="{{.B0}}">{{.B0}}</time>. Earliest possible disclosure boundary.</p><p>Bmax: <time class="renewal-time" data-unix="{{.Bmax}}">{{.Bmax}}</time>. Latest boundary already issued for transmission.</p><p class="renewal-allowance">E assumptions: {{.Allowance}}</p><p>Deadline: <time class="renewal-time" data-unix="{{.Deadline}}">{{.Deadline}}</time>.</p><p>Grace: {{.Grace}} seconds.</p><ul class="renewal-agents" aria-label="Agent receipt evidence">{{range .Agents}}<li data-agent-id="{{.ID}}">Agent {{.ID}}: {{.State}}</li>{{end}}</ul></section>{{end}}<form class="renewal-evidence-form"><label for="renewal-evidence-{{$id}}">Import renewal evidence from the CLI</label> <input id="renewal-evidence-{{$id}}" name="evidence" type="file" accept="application/json,.json" required><button type="submit">Submit check-in evidence</button><p class="renewal-evidence-status" role="status"></p></form><a href="/api/v1/switches/{{$id}}">Inspect signed arm evidence</a></li>{{end}}</ul></section></main></body></html>`

// SwitchJS labels proposed times before the owner creates a draft.
const SwitchJS = `'use strict';
(() => {
 const form=document.querySelector('#switch-form'), fields=document.querySelector('#switch-fields'), status=document.querySelector('#switch-status');
 const interval=document.querySelector('#switch-interval'), grace=document.querySelector('#switch-grace'), preview=document.querySelector('#switch-preview');
 document.querySelectorAll('.renewal-time').forEach(time=>{
  const seconds=Number(time.dataset.unix);
  if (!Number.isSafeInteger(seconds) || seconds<=0) return;
  const date=new Date(seconds*1000);
  if (!Number.isFinite(date.getTime())) {time.textContent=seconds+' Unix seconds — outside the browser date range';return;}
  time.dateTime=date.toISOString();
  time.textContent=date.toISOString()+' UTC / '+date.toLocaleString()+' local';
 });
 document.querySelectorAll('.renewal-evidence-form').forEach(form=>{
  form.addEventListener('submit',async event=>{
   event.preventDefault();
   const item=form.closest('li'), file=form.elements.evidence.files[0], message=form.querySelector('.renewal-evidence-status');
   if (!file) {message.textContent='Select a JSON evidence file.';return;}
   if (file.size>65536) {message.textContent='The evidence file exceeds 65536 bytes.';return;}
   form.querySelector('button').disabled=true; message.textContent='The check-in evidence is pending.';
   try {
    const evidence=JSON.parse(await file.text());
    if (!evidence || typeof evidence!=='object' || Array.isArray(evidence) || Object.keys(evidence).some(key=>!['confirmed_request','confirmed_receipts','request','receipts'].includes(key))) throw new Error('The evidence file contains unsupported fields.');
    const proof=await request('/api/v1/csrf');
    await request('/api/v1/switches/'+encodeURIComponent(item.dataset.id)+'/checkins',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':proof.csrf_token},body:JSON.stringify(evidence)});
    message.textContent='Check-in evidence accepted. Refreshing renewal status.';
    location.reload();
   } catch(error) {message.textContent=error.message || 'The check-in evidence request fails.';}
   finally {form.querySelector('button').disabled=false;}
  });
 });
 function times() {
  if (!interval.validity.valid || !grace.validity.valid) {preview.textContent='Enter whole seconds within the permitted ranges.';return;}
  const deadline=new Date(Date.now()+Number(interval.value)*1000), boundary=new Date(deadline.getTime()+Number(grace.value)*1000);
  preview.textContent='Proposed times if armed now. Deadline UTC: '+deadline.toISOString()+'. Deadline local: '+deadline.toLocaleString()+'. Grace: '+grace.value+' seconds. Release boundary UTC: '+boundary.toISOString()+'. Release boundary local: '+boundary.toLocaleString()+'. Preview only; no agent accepts these times.';
 }
 async function request(path,options={}) {
  const response=await fetch(path,{...options,signal:AbortSignal.timeout(10000),cache:'no-store'}),body=await response.json();
  if (!response.ok) throw new Error(body.error?.message || 'The switch request fails.');
  return body;
 }
 interval.addEventListener('input',times);grace.addEventListener('input',times);times();
 form.addEventListener('submit',async event=>{
  event.preventDefault(); if (fields.disabled) return; fields.disabled=true;status.textContent='The draft request is pending.';
  try {
   const proof=await request('/api/v1/csrf');
   const record=await request('/api/v1/switches',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':proof.csrf_token},body:JSON.stringify({interval_seconds:Number(interval.value),grace_seconds:Number(grace.value)})});
   const item=document.createElement('li'); item.dataset.id=record.id; item.textContent=record.id+' — Draft — unarmed and unreplicated. Interval: '+record.interval_seconds+' seconds. Grace: '+record.grace_seconds+' seconds.';document.querySelector('#switch-list').append(item);
   status.textContent='Draft created. It is unarmed and unreplicated.';
  } catch(error) {status.textContent=error.message || 'The switch service is unavailable.';}
  finally {fields.disabled=false;}
 });
 request('/api/v1/csrf').then(()=>{fields.disabled=false;status.textContent='Configure the draft interval and grace.';}).catch(()=>{status.textContent='The owner session is unavailable. Reload this page.';});
})();`
