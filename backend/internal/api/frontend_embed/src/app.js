// OpenZerg live UI. Plain ES module. No build step.
//
// Subscribes to /api/events, draws pods on a golden-angle spiral in the
// arena, updates the generation banner from generation_start/end events,
// and surfaces the run log + history on the side panels.

const goldenAngleRadians = Math.PI * (3 - Math.sqrt(5));

const state = {
  currentRunID: null,
  currentGeneration: 0,
  population: 0,
  bestFitness: 0,
  probesByIndex: new Map(),
  cancelling: false,
};

const el = {
  statusPill: document.getElementById('status-pill'),
  intOpenrouter: document.getElementById('int-openrouter'),
  intNimble: document.getElementById('int-nimble'),
  history: document.getElementById('history'),
  generationBadge: document.getElementById('generation-badge'),
  genNum: document.getElementById('gen-num'),
  genTitle: document.getElementById('gen-title'),
  genSub: document.getElementById('gen-sub'),
  bestScore: document.getElementById('best-score'),
  arena: document.getElementById('arena'),
  targetName: document.getElementById('target-name'),
  targetState: document.getElementById('target-state'),
  log: document.getElementById('log'),
  form: document.getElementById('start-form'),
  startBtn: document.getElementById('start-btn'),
  cancelBtn: document.getElementById('cancel-btn'),
};

function setStatus(text, classes = '') {
  el.statusPill.textContent = text;
  el.statusPill.className = 'pill ' + classes;
}

function appendLog(message, cssClass = '') {
  const li = document.createElement('li');
  if (cssClass) li.className = cssClass;
  li.textContent = `${new Date().toLocaleTimeString()}  ${message}`;
  el.log.appendChild(li);
  el.log.scrollTop = el.log.scrollHeight;
  while (el.log.children.length > 400) el.log.removeChild(el.log.firstChild);
}

function placeProbe(index, total) {
  const radius = 70 + ((index % 9) * 16);
  const angle = index * goldenAngleRadians;
  const rect = el.arena.getBoundingClientRect();
  const centerX = rect.width / 2;
  const centerY = rect.height / 2;
  return {
    left: centerX + Math.cos(angle) * radius,
    top: centerY + Math.sin(angle) * radius,
  };
}

function ensureProbe(index) {
  let probe = state.probesByIndex.get(index);
  if (probe) return probe;
  probe = document.createElement('div');
  probe.className = 'probe';
  const { left, top } = placeProbe(index, state.population);
  probe.style.left = `${left}px`;
  probe.style.top = `${top}px`;
  el.arena.appendChild(probe);
  state.probesByIndex.set(index, probe);
  return probe;
}

function clearProbes() {
  for (const probe of state.probesByIndex.values()) probe.remove();
  state.probesByIndex.clear();
}

function colorFromResult(status, evidence) {
  const ev = (evidence || '').toLowerCase();
  if (status === 'BREACH') return 'breach';
  if (status === 'PARTIAL') return 'partial';
  if (status === 'RECON') return 'warm';
  if (status === 'ERROR' || ev.includes('timeout')) return 'error';
  return 'partial';
}

function onEvent(envelope) {
  const payload = envelope.payload || {};
  switch (envelope.type) {
    case 'hello': {
      appendLog(`SSE connected (buffer=${payload.buffer_len || 0})`);
      break;
    }
    case 'run_start': {
      state.currentRunID = envelope.run_id;
      state.currentGeneration = 0;
      state.bestFitness = 0;
      clearProbes();
      el.bestScore.textContent = '0.00';
      el.targetName.textContent = (payload.target_url || '').replace(/^https?:\/\//, '');
      el.targetState.textContent = 'engaged';
      el.targetState.className = 'state';
      setStatus(`run ${envelope.run_id}`, 'live');
      el.startBtn.disabled = true;
      el.cancelBtn.disabled = false;
      appendLog(`run_start ${envelope.run_id} -> ${payload.target_url} (pop=${payload.population}, gens=${payload.generations})`);
      break;
    }
    case 'generation_start': {
      state.currentGeneration = payload.generation;
      state.population = payload.population;
      clearProbes();
      el.generationBadge.textContent = `gen ${payload.generation}`;
      el.genNum.textContent = String(payload.generation);
      el.genTitle.textContent = `generation ${payload.generation}`;
      el.genSub.textContent = `${payload.population} pods spawning`;
      appendLog(`generation_start gen=${payload.generation} pop=${payload.population}`);
      break;
    }
    case 'pod_spawn': {
      const probe = ensureProbe(payload.index);
      probe.title = `${payload.pod_id} ${payload.vector || ''}`;
      appendLog(`pod_spawn   gen=${payload.generation} pod=${payload.index} vec=${payload.vector || '?'}`);
      break;
    }
    case 'pod_result': {
      const probe = ensureProbe(payload.index);
      const css = colorFromResult(payload.status, payload.evidence);
      probe.classList.add(css);
      const tag = payload.status || (payload.error ? 'ERROR' : 'OK');
      appendLog(`pod_result  gen=${payload.generation} pod=${payload.index} status=${tag}  ${payload.evidence || payload.error || ''}`);
      break;
    }
    case 'generation_end': {
      if (payload.best_fitness > state.bestFitness) state.bestFitness = payload.best_fitness;
      el.bestScore.textContent = state.bestFitness.toFixed(2);
      el.genSub.textContent = `${payload.population} pods, ${payload.survivors} survivors, best ${payload.best_fitness.toFixed(2)}`;
      appendLog(`generation_end gen=${payload.generation} survivors=${payload.survivors} best=${payload.best_fitness.toFixed(2)}`);
      break;
    }
    case 'mutation': {
      appendLog(`mutation    gen=${payload.generation} source=${payload.source}`);
      break;
    }
    case 'breach': {
      el.targetState.textContent = 'breached';
      el.targetState.className = 'state bad';
      setStatus('BREACH', 'live');
      appendLog(`BREACH      gen=${payload.generation} pod=${payload.pod_id} vector=${payload.vector}`, 'breach');
      break;
    }
    case 'run_end': {
      const outcome = payload.outcome || 'UNKNOWN';
      const css = outcome === 'BREACH' ? 'live' : '';
      setStatus(`done: ${outcome}`, css);
      el.startBtn.disabled = false;
      el.cancelBtn.disabled = true;
      state.cancelling = false;
      appendLog(`run_end     outcome=${outcome} best=${(payload.best_fitness || 0).toFixed(2)}`);
      refreshHistory();
      break;
    }
    default: {
      // ignore unknown event types — forward-compat
      break;
    }
  }
}

function openEventStream() {
  const source = new EventSource('/api/events');
  source.onerror = () => setStatus('disconnected', 'error');
  source.onopen = () => {
    if (!state.currentRunID) setStatus('connected');
  };
  // EventSource.onmessage only fires for SSE chunks WITHOUT an `event:`
  // field. Our server always sets `event: <type>`, so we have to register
  // a listener per known type. The default handler is kept as a fallback
  // for forward compatibility — e.g., new event types added on the server
  // but not yet known to this build of the UI.
  const handle = (msg) => {
    try {
      const envelope = JSON.parse(msg.data);
      onEvent(envelope);
    } catch (err) {
      console.warn('bad SSE payload', err);
    }
  };
  const knownTypes = [
    'hello',
    'run_start',
    'generation_start',
    'pod_spawn',
    'pod_log',
    'pod_result',
    'generation_end',
    'mutation',
    'nimble_call',
    'breach',
    'run_end',
    'integration_status',
  ];
  for (const type of knownTypes) source.addEventListener(type, handle);
  source.onmessage = handle; // fallback for unnamed events
  return source;
}

async function refreshIntegrations() {
  try {
    const openrouterResp = await fetch('/api/integrations/openrouter').then((r) => r.json());
    el.intOpenrouter.textContent = openrouterResp.ok ? `ok (${openrouterResp.model})` : 'missing';
    el.intOpenrouter.className = openrouterResp.ok ? 'live' : '';
  } catch (e) {
    el.intOpenrouter.textContent = 'error';
  }
  try {
    const nimbleResp = await fetch('/api/integrations/nimble').then((r) => r.json());
    el.intNimble.textContent = nimbleResp.ok ? 'ok' : 'missing';
    el.intNimble.className = nimbleResp.ok ? 'live' : '';
  } catch (e) {
    el.intNimble.textContent = 'error';
  }
}

async function refreshHistory() {
  try {
    const runs = await fetch('/api/runs').then((r) => r.json());
    if (!Array.isArray(runs) || runs.length === 0) {
      el.history.textContent = 'no completed runs yet';
      return;
    }
    el.history.innerHTML = '';
    for (const run of runs) {
      const row = document.createElement('div');
      const css = run.outcome === 'BREACH' ? 'breach' : 'exhausted';
      row.className = `history-row ${css}`;
      row.innerHTML = `<b>${run.run_id}</b><br>${run.outcome} · best ${(run.best_fitness || 0).toFixed(2)}<br>${run.target_url}`;
      row.title = JSON.stringify(run, null, 2);
      el.history.appendChild(row);
    }
  } catch (e) {
    el.history.textContent = 'failed to load history';
  }
}

el.form.addEventListener('submit', async (event) => {
  event.preventDefault();
  const formData = new FormData(el.form);
  const body = {
    target_url: formData.get('target_url'),
    population: Number(formData.get('population')) || 3,
    generations: Number(formData.get('generations')) || 1,
    disable_nimble: formData.get('disable_nimble') === 'on',
    enable_cve_seed: formData.get('enable_cve_seed') === 'on',
  };
  try {
    const resp = await fetch('/api/runs', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    });
    const json = await resp.json();
    if (!resp.ok) {
      appendLog(`start failed: ${json.error || resp.status}`, 'error');
      return;
    }
    appendLog(`POST /api/runs accepted run_id=${json.run_id}`);
  } catch (err) {
    appendLog(`start failed: ${err.message}`, 'error');
  }
});

el.cancelBtn.addEventListener('click', async () => {
  if (state.cancelling) return;
  state.cancelling = true;
  try {
    const resp = await fetch('/api/runs/current/cancel', { method: 'POST' });
    const json = await resp.json();
    if (!resp.ok) {
      appendLog(`cancel failed: ${json.error || resp.status}`, 'error');
      state.cancelling = false;
      return;
    }
    appendLog(`cancel sent for ${json.run_id}`);
  } catch (err) {
    appendLog(`cancel failed: ${err.message}`, 'error');
    state.cancelling = false;
  }
});

el.cancelBtn.disabled = true;
refreshIntegrations();
refreshHistory();
openEventStream();
