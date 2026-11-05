package web

// SwitchHTML keeps draft configuration separate from agent authority.
const SwitchHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Owner switches — epochalypse</title><link rel="stylesheet" href="/owner/style.css"><script src="/owner/switches.js" defer></script></head>
<body><main><a href="/owner">Owner dashboard</a><h1>Configure a switch draft</h1>
<p>A draft is unarmed and unreplicated. It has no accepted deadline or release boundary. Switch production use remains disabled.</p>
<form id="switch-form"><fieldset id="switch-fields" disabled><legend>Draft configuration</legend>
<p><label for="switch-interval">Check-in interval (seconds)</label> <input id="switch-interval" type="number" min="3600" max="31536000" step="1" value="86400" required></p>
<p><label for="switch-grace">Grace period (seconds)</label> <input id="switch-grace" type="number" min="0" max="2592000" step="1" value="3600" required></p>
<p>Interval: 1 hour to 365 days. Grace: zero to 30 days.</p>
<p id="switch-preview" aria-live="polite"></p><button type="submit">Create draft</button></fieldset></form>
<p id="switch-status" role="status" tabindex="-1">Check the owner session.</p>
<section><h2>Retained drafts</h2><ul id="switch-list">{{range .}}<li data-id="{{.ID}}"><code>{{.ID}}</code><p>Draft — unarmed and unreplicated</p><p>Interval: {{.IntervalSeconds}} seconds. Grace: {{.GraceSeconds}} seconds.</p></li>{{end}}</ul></section></main></body></html>`

// SwitchJS labels proposed times before the owner creates a draft.
const SwitchJS = `'use strict';
(() => {
 const form=document.querySelector('#switch-form'), fields=document.querySelector('#switch-fields'), status=document.querySelector('#switch-status');
 const interval=document.querySelector('#switch-interval'), grace=document.querySelector('#switch-grace'), preview=document.querySelector('#switch-preview');
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
