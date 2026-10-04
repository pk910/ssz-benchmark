/* dynamic-ssz benchmark UI: one page, hash routes, data from /api. */
(() => {
  'use strict';

  const appEl = document.getElementById('app');
  // A timed refresh updates the page in place: the new markup is diffed
  // into the existing nodes, so the scroll position, open sections and the
  // charts stay where they are. Navigation replaces the page.
  let refreshing = false;
  function morph(from, to) {
    const a = Array.from(from.childNodes), b = Array.from(to.childNodes);
    for (let i = 0; i < b.length; i++) {
      const n = b[i], o = a[i];
      if (!o) { from.appendChild(n); continue; }
      if (o.nodeType !== n.nodeType || o.nodeName !== n.nodeName) { from.replaceChild(n, o); continue; }
      if (o.nodeType !== 1) { if (o.nodeValue !== n.nodeValue) o.nodeValue = n.nodeValue; continue; }
      if (o.nodeName === 'CANVAS') continue; // the chart on it is updated by chart()
      for (const at of Array.from(o.attributes)) {
        if (!n.hasAttribute(at.name) && !(o.nodeName === 'DETAILS' && at.name === 'open')) o.removeAttribute(at.name);
      }
      for (const at of Array.from(n.attributes)) if (o.getAttribute(at.name) !== at.value) o.setAttribute(at.name, at.value);
      morph(o, n);
    }
    for (let i = a.length - 1; i >= b.length; i--) from.removeChild(a[i]);
  }
  const app = {
    set innerHTML(html) {
      html = html.replace(/<table/g, '<div class="tw"><table').replace(/<\/table>/g, '</table></div>');
      if (!refreshing) { appEl.innerHTML = html; return; }
      const t = document.createElement('div');
      t.innerHTML = html;
      morph(appEl, t);
    },
    querySelectorAll: sel => appEl.querySelectorAll(sel),
  };
  const liveEl = document.getElementById('live');
  const OBJECTS = ['FuluState', 'FuluBlock', 'FuluBlocks', 'GloasState', 'GloasBlock', 'GloasBlocks', 'GloasEnvelope'];
  const OPS = ['Unmarshal', 'UnmarshalReader', 'UnmarshalReaderUnknown', 'SizeSSZ', 'Marshal', 'MarshalTo', 'MarshalWriter', 'HashTreeRoot', 'GetTree'];
  // What each benchmark object is; shown on hover of the info mark next to
  // its name.
  const OBJECT_INFO = {
    FuluState: 'A real mainnet beacon state in the Fulu layout, 348 MB. Its pending lists are filled up artificially so every part of the state is exercised: 65,536 pending deposits, partial withdrawals and consolidations each, 2,048 eth1 data votes.',
    FuluBlock: 'One real mainnet block, extended so that every operation list is at its mainnet limit: proposer and attester slashings, attestations with full aggregation bits, deposits, voluntary exits, BLS changes, deposit, withdrawal and consolidation requests, blob commitments. 2.4 MB. Real blocks almost never carry slashings, deposits or exits, so this block is what exercises every code path.',
    FuluBlocks: 'The 32 real, unmodified mainnet blocks of one epoch, 40 KB to 323 KB each, 4.8 MB together. One iteration runs the operation on all 32 in sequence. This is the traffic the library sees in production: mostly transactions and attestations, in varying sizes.',
    GloasState: 'The same state as FuluState, converted to the Gloas layout: identical content, different structure (progressive containers and lists, builder registry and payment lists). Differences to FuluState show what the Gloas SSZ changes cost.',
    GloasBlock: 'The extended block of FuluBlock converted to the Gloas layout (progressive lists, payload attestations, the execution payload replaced by its bid). Identical content where the two forks overlap.',
    GloasBlocks: 'The 32 real blocks of FuluBlocks converted to the Gloas layout. One iteration runs the operation on all 32 in sequence.',
    GloasEnvelope: 'The signed execution payload envelope that belongs to GloasBlock: in Gloas the execution payload travels separately from the beacon block. 1.9 MB, mostly transactions.',
  };
  const objInfo = o => OBJECT_INFO[o] ? `<span class="info" tabindex="0" data-tip="${OBJECT_INFO[o].replace(/"/g, '&quot;')}">i</span>` : '';
  const objName = o => `${o}${objInfo(o)}`;
  const ENGINES = ['Codegen', 'Reflection', 'CodegenAsync', 'ReflectionAsync', 'FastSSZ', 'FastSSZv1', 'FastSSZv2', 'PrysmSSZ', 'KaralabeSSZ', 'KaralabeSSZAsync'];
  const COLORS = ['#2f5fd1', '#d97706', '#7c3aed', '#0f9d8a', '#6b7280', '#db2777'];
  const METRICS = {
    ns: { key: 'Ns', label: 'Time', unit: 'per op', fmt: fmtNs },
    cycles: { key: 'Cycles', label: 'Cycles', unit: 'cycles/op', fmt: fmtNum },
    instrs: { key: 'Instrs', label: 'Instructions', unit: 'instrs/op', fmt: fmtNum },
    bytes: { key: 'Bytes', label: 'Memory', unit: 'B/op', fmt: fmtBytes },
    allocs: { key: 'Allocs', label: 'Allocations', unit: 'allocs/op', fmt: fmtNum },
  };
  let metric = localStorage.getItem('metric') || 'cycles';
  // The job charts show the change in percent, or the measured values.
  let chartMode = localStorage.getItem('chartMode') === 'abs' ? 'abs' : 'rel';
  // Whether the measured-values charts include the reference libraries.
  let chartLibs = localStorage.getItem('chartLibs') === '1';
  // The counters see the measured thread only. An async engine does its
  // work on other threads, so its cycles and instructions say nothing; its
  // cells show wall time on those tabs.
  // metricFor is the metric an engine is read by on a tab. The counters of
  // an async engine cover one of its threads only, so the counter tabs
  // show its time, unless the result says all its threads were counted.
  const allThreads = r => !!(r && r.Extra && r.Extra.threads);
  const metricFor = (m, engine, r) => engine.endsWith('Async') && (m.key === 'Cycles' || m.key === 'Instrs') && !allThreads(r) ? METRICS.ns : m;
  let repo = '';
  const charts = [];

  /* ---------- helpers ---------- */
  const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const rank = (order, v) => { const i = order.indexOf(v); return i < 0 ? order.length : i; };
  function fmtNs(v) {
    if (v == null || isNaN(v)) return '-';
    if (v >= 1e9) return (v / 1e9).toFixed(3) + ' s';
    if (v >= 1e6) return (v / 1e6).toFixed(2) + ' ms';
    if (v >= 1e3) return (v / 1e3).toFixed(2) + ' µs';
    return v.toFixed(1) + ' ns';
  }
  function fmtBytes(v) {
    if (v == null || isNaN(v)) return '-';
    if (v >= 1 << 30) return (v / (1 << 30)).toFixed(2) + ' GB';
    if (v >= 1 << 20) return (v / (1 << 20)).toFixed(2) + ' MB';
    if (v >= 1 << 10) return (v / (1 << 10)).toFixed(1) + ' KB';
    return v.toFixed(0) + ' B';
  }
  function fmtNum(v) {
    if (v == null || isNaN(v)) return '-';
    if (v >= 1e6) return (v / 1e6).toFixed(2) + 'M';
    if (v >= 1e3) return (v / 1e3).toFixed(1) + 'k';
    return v.toFixed(0);
  }
  const pct = (v, d = 2) => (v >= 0 ? '+' : '') + v.toFixed(d) + '%';
  const short = s => (s || '').slice(0, 12);
  const dur = s => { s = Math.round(s); const m = Math.floor(s / 60); return m ? `${m}m${String(s % 60).padStart(2, '0')}s` : `${s}s`; };
  // when writes a time in the viewer's time zone, as YYYY-MM-DD HH:MM.
  const when = t => {
    if (!t) return '-';
    const d = new Date(t), p = n => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
  };
  const ago = t => {
    if (!t) return '';
    const d = (Date.now() - new Date(t)) / 1000;
    if (d < 60) return 'just now';
    if (d < 3600) return `${Math.floor(d / 60)}m ago`;
    if (d < 172800) return `${Math.floor(d / 3600)}h ago`;
    return `${Math.floor(d / 86400)}d ago`;
  };
  // A change counts when it exceeds the machine's noise for that operation
  // (p95 |Δ| over the self-comparisons, at least 0.5%) and the median
  // delta agrees in sign with the mean.
  let leafNoise = {};
  const floorOf = key => Math.max(0.5, (key && leafNoise[key]) || 0);
  // delta is the result's headline: the median of the per-pass deltas
  // (each pass links both sides with another function layout), so one
  // layout under which a side runs far off does not move it.
  const delta = m => m.PN > 0 ? m.PMed : m.Delta;
  // bandOf is how far that median may lie from zero without being a
  // change: the operation's noise floor, or twice the standard error of
  // the per-pass deltas when the layouts scattered this pair more.
  const bandOf = (m, key) => Math.max(floorOf(key), m.PN > 1 ? 2 * m.PSpread / Math.sqrt(m.PN) : 0);
  const sig = (m, key) => Math.abs(delta(m)) > bandOf(m, key) && (!(m.PN > 0) || m.PAgree >= Math.ceil(0.75 * m.PN));
  function cls(m, key) {
    if (!sig(m, key)) return 'same';
    const strong = Math.abs(delta(m)) >= 5 ? ' strong' : '';
    return (delta(m) < 0 ? 'better' : 'worse') + strong;
  }
  const badge = (m, key) => `<span class="badge ${cls(m, key)}" title="median of the per-pass deltas${m.PN > 0 ? ` (${m.PAgree} of ${m.PN} passes agree, single passes ${pct(m.DMin, 2)} to ${pct(m.DMax, 2)})` : ''} · band ±${bandOf(m, key).toFixed(2)}% (noise floor ${floorOf(key).toFixed(2)}%) · mean Δ ${pct(m.Delta)}">${pct(delta(m))}</span>`;
  // The further figures a run can carry, by their key in Extra.
  const EXTRAS = {
    'br-miss': { name: 'branch misses', short: 'br-miss' },
    'fe-stall': { name: 'frontend stall cycles', short: 'fe-stall' },
    'l1d-miss': { name: 'L1 data misses', short: 'l1d-miss' },
    'l2-miss': { name: 'L2 data misses', short: 'l2-miss' },
    retained: { name: 'kept alive by the result', bytes: true },
    stack: { name: 'stack one call needs beyond 32 KiB', bytes: true },
  };
  const DIAG = { 'dec-uops': 'decoded uops', 'l1i-miss': 'L1i misses', 'dtlb-miss': 'dTLB misses', 'itlb-miss': 'iTLB misses' };
  // kindOf tells what a change of an operation is made of: 'code' when its
  // instructions per op moved (it does different work), 'same' when they
  // did not (the same work executes differently), null without counters.
  const INSTR_MOVED = 0.5;
  function kindOf(r) {
    if (!r || !r.Instrs || !(r.Instrs.Base > 0) || !(r.Instrs.Head > 0) || r.Engine.endsWith('Async')) return null;
    return Math.abs(delta(r.Instrs)) >= INSTR_MOVED ? 'code' : 'same';
  }
  const kindText = r => kindOf(r) === 'code'
    ? `instructions per op ${pct(delta(r.Instrs))}: the code does different work`
    : `instructions per op unchanged (${pct(delta(r.Instrs))}): the same work executes differently (code layout, cache or branch behaviour)`;
  const kindChip = r => kindOf(r) ? `<span class="kind ${kindOf(r)}" title="${kindText(r)}">${kindOf(r) === 'code' ? 'code' : 'same work'}</span>` : '';
  // cpi is cycles per instruction of one side of a result.
  const cpi = (r, side) => r.Instrs && r.Instrs[side] > 0 && r.Cycles[side] > 0 ? r.Cycles[side] / r.Instrs[side] : 0;
  // buildRows pairs the head and base build facts of a job per package.
  function buildRows(builds) {
    const by = {};
    (builds || []).forEach(b => { (by[b.Pkg] = by[b.Pkg] || {})[b.Side] = b; });
    return Object.keys(by).sort().map(pkg => ({ pkg, head: by[pkg].head, base: by[pkg].base })).filter(x => x.head);
  }
  // sizeDelta writes a head size with its change against the base.
  const sizeDelta = (h, b) => `${fmtBytes(h)}${b > 0 && b !== h ? ` <span class="${h > b ? 'worse' : 'better'}">${pct((h - b) / b * 100)}</span>` : ''}`;
  function buildCard(builds) {
    const rows = buildRows(builds);
    if (!rows.length) return '';
    const secs = rows[0].head.BuildSeconds;
    return `<div class="card"><h3>Build <span class="muted" style="text-transform:none">(head, change against the base)</span></h3>${rows.map(x => `<div class="sub mono">${x.pkg}: code ${sizeDelta(x.head.TextBytes, x.base ? x.base.TextBytes : 0)} · binary ${sizeDelta(x.head.BinBytes, x.base ? x.base.BinBytes : 0)} · generated source ${sizeDelta(x.head.GenBytes, x.base ? x.base.GenBytes : 0)}</div>`).join('')}${secs > 0 ? `<div class="sub">generating and compiling the head took ${dur(secs)}</div>` : ''}</div>`;
  }
  const ciText = m => `<span class="ci">[${pct(m.Lo, 1)}, ${pct(m.Hi, 1)}]</span>`;
  const chip = j => `<span class="chip ${j.State}">${j.State}</span> <span class="chip ${j.Kind}">${j.Kind}</span>`;
  const commitLink = sha => sha === 'baselines' ? '<span class="muted">reference libraries</span>' : `<a class="mono" href="${repo}/commit/${sha}" target="_blank" rel="noopener">${short(sha)}</a>`;
  const prLink = j => j.PR ? ` · <a href="${repo}/pull/${j.PR}" target="_blank" rel="noopener">PR #${j.PR}</a> <a href="#/pr/${j.PR}" title="every measured head of this pull request">history</a>` : '';
  const sortLeaf = (a, b) => rank(OBJECTS, a.Object) - rank(OBJECTS, b.Object) || a.Object.localeCompare(b.Object)
    || rank(OPS, a.Op) - rank(OPS, b.Op) || a.Op.localeCompare(b.Op) || rank(ENGINES, a.Engine) - rank(ENGINES, b.Engine);
  const sortEngines = es => es.sort((a, b) => rank(ENGINES, a) - rank(ENGINES, b) || a.localeCompare(b));

  // engineTotals folds per-object geomeans into one per engine, over
  // cycles when the job has cycle counts, else over time.
  function engineTotals(sums) {
    const acc = {};
    const useCycles = (sums || []).some(s => s.CyclesGeomean);
    (sums || []).forEach(s => {
      const a = acc[s.Engine] || (acc[s.Engine] = { Engine: s.Engine, logSum: 0, N: 0 });
      a.logSum += Math.log(1 + (useCycles && !s.Engine.endsWith('Async') ? s.CyclesGeomean : s.Geomean) / 100) * s.N;
      a.N += s.N;
    });
    return sortEngines(Object.keys(acc)).map(e => ({ Engine: e, N: acc[e].N, Geomean: (Math.exp(acc[e].logSum / acc[e].N) - 1) * 100, Cycles: useCycles }));
  }

  async function get(url) {
    const r = await fetch(url);
    if (!r.ok) throw new Error(`${url}: ${r.status}`);
    return r.json();
  }
  function destroyCharts() {
    if (refreshing) {
      // Keep the charts whose canvas survived; chart() updates them.
      for (let i = charts.length - 1; i >= 0; i--) if (!charts[i].canvas.isConnected) charts.splice(i, 1)[0].destroy();
      return;
    }
    while (charts.length) charts.pop().destroy();
  }
  function chart(id, cfg) {
    const el = document.getElementById(id);
    if (!el || typeof Chart === 'undefined') return null;
    const existing = refreshing && Chart.getChart(el);
    if (existing) {
      existing.data = cfg.data;
      existing.options = cfg.options;
      existing.update('none');
      return existing;
    }
    Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;
    Chart.defaults.font.size = 11;
    Chart.defaults.color = getComputedStyle(document.body).getPropertyValue('--muted').trim();
    // candle: the candle under the pointer, over its whole length (body
    // and thin line), not only over the body.
    Chart.Tooltip.positioners.pointer = (items, pos) => items.length ? { x: pos.x, y: items[0].element.y } : false;
    Chart.Interaction.modes.candle = (c, e) => {
      const x = c.scales.x;
      let best = null;
      c.data.datasets.forEach((ds, di) => {
        if (!c.isDatasetVisible(di) || !ds.whisker) return;
        c.getDatasetMeta(di).data.forEach((bar, i) => {
          const cell = ds.meta[i];
          if (!cell) return;
          const ends = [...ds.whisker(cell).map(v => x.getPixelForValue(v)), bar.x, bar.base].filter(Number.isFinite);
          const dy = Math.abs(e.y - bar.y);
          if (dy > Math.max(6, bar.height / 2 + 1) || e.x < Math.min(...ends) - 4 || e.x > Math.max(...ends) + 4) return;
          if (!best || dy < best.dy) best = { dy, element: bar, datasetIndex: di, index: i };
        });
      });
      return best ? [{ element: best.element, datasetIndex: best.datasetIndex, index: best.index }] : [];
    };
    const c = new Chart(el, cfg);
    charts.push(c);
    return c;
  }
  function metricTabs(onChange) {
    return `<div class="tabs" id="metricTabs">${Object.entries(METRICS).map(([k, m]) => `<button data-m="${k}" class="${k === metric ? 'active' : ''}">${m.label}</button>`).join('')}</div>`;
  }
  function bindMetricTabs(rerender) {
    const t = document.getElementById('metricTabs');
    if (!t) return;
    t.querySelectorAll('button').forEach(b => b.onclick = () => { metric = b.dataset.m; localStorage.setItem('metric', metric); rerender(); });
  }

  /* ---------- live header ---------- */
  async function refreshLive() {
    try {
      const s = await get('/api/status');
      repo = s.Repo || repo;
      const active = (s.Runners || []).filter(r => r.Current && !r.Down);
      if (s.DaemonDown) {
        liveEl.innerHTML = `<span class="err">controller daemon not reporting</span> · ${s.Queued} queued`;
      } else if (active.length) {
        liveEl.innerHTML = active.map(r => { const p = r.Progress || {}; return `<a href="#/job/${r.Current.ID}"><b>#${r.Current.ID}</b></a> on ${esc(r.Runner)}${p.Leaves ? ` pass ${p.Pass}${p.PlannedPasses ? '/~' + p.PlannedPasses : ''} · ${p.Leaf}/${p.Leaves}` : ''}`; }).join(' · ') + ` · ${s.Queued} queued`;
      } else {
        liveEl.innerHTML = `idle · ${s.Queued} queued`;
      }
    } catch (e) { liveEl.textContent = ''; }
  }

  /* ---------- views ---------- */
  function jobRows(jobs, opts = {}) {
    if (!jobs || !jobs.length) return '<p class="muted">none</p>';
    return `<table><thead><tr><th>#</th><th>State</th><th>Branch / PR</th><th>Head</th><th>Base</th><th>Runner</th>${opts.summaries ? '<th>Ratio per engine <span class="muted" style="text-transform:none">(cycles when counted, else time)</span></th>' : ''}<th class="num">Passes</th><th class="num">Took</th><th>${opts.queue ? 'Queued' : 'Finished'}</th></tr></thead><tbody>` +
      jobs.map(row => {
        const j = row.Job || row;
        const sums = row.Summaries ? `<td><div class="chips">${engineTotals(row.Summaries).map(s => `<span class="chip" title="geomean of head/base ${s.Cycles ? 'cycles' : 'time'} over ${s.N} operations of every object">${s.Engine} <b>${pct(s.Geomean, 1)}</b></span>`).join('')}</div></td>` : (opts.summaries ? '<td></td>' : '');
        const fin = j.State === 'queued' ? `${j.Priority ? 'priority ' + j.Priority + ' · ' : ''}${ago(j.Created)}` : j.State === 'running' ? `since ${when(j.Started)}` : when(j.Finished);
        return `<tr><td><a href="#/job/${j.ID}">${j.ID}</a></td><td>${chip(j)}</td><td class="mono">${esc(j.Branch)}${prLink(j)}</td>` +
          `<td>${commitLink(j.HeadSHA)} <span class="muted desc" style="display:inline-block;vertical-align:bottom">${esc(j.HeadDesc).replace(/^[0-9a-f]{7} /, '')}</span></td>` +
          `<td>${commitLink(j.BaseSHA)} <span class="muted">${esc(j.BaseRef)}</span></td><td class="mono muted">${esc(j.Runner || '')}</td>${sums}<td class="num">${j.Passes || ''}</td><td class="num">${j.Seconds ? dur(j.Seconds) : ''}</td>` +
          `<td class="muted">${fin}${j.Error ? ` <span class="err" title="${esc(j.Error)}">error</span>` : ''}</td></tr>`;
      }).join('') + '</tbody></table>';
  }

  async function viewDash() {
    const d = await get('/api/dashboard');
    repo = d.Repo;
    const s = d.Status;
    const runnerCard = r => {
      const p = r.Progress || {};
      const state = r.State === 'controller' ? '' : ` <span class="chip ${r.State === 'ok' ? 'done' : r.State === 'rejected' ? 'failed' : 'running'}" title="${esc(r.Note)}">${r.State}</span>`;
      if (r.Down) return `<div class="card"><h3>${esc(r.Runner)}${state}</h3><div class="big err">not reporting</div><div class="sub">last seen ${when(r.Seen)}</div></div>`;
      if (!r.Current) return `<div class="card"><h3>${esc(r.Runner)}${state}</h3><div class="big">idle</div><div class="sub">${r.State === 'rejected' ? 'rejected: ' + esc(r.Note) : 'waiting for the next job'}</div></div>`;
      return `<div class="card" style="grid-column: span 2"><h3>${esc(r.Runner)}${state}</h3>
        <div class="big">job <a href="#/job/${r.Current.ID}">#${r.Current.ID}</a> <span class="chip ${r.Current.Kind}">${r.Current.Kind}</span></div>
        <div class="sub mono">${esc(r.Current.Branch)} ${short(r.Current.HeadSHA)} vs ${short(r.Current.BaseSHA)}</div>
        <div class="sub">${esc(r.Phase)} · ${dur(r.Running)}</div>
        ${p.Leaves ? `<div class="bar"><div style="width:${r.Percent}%"></div></div><div class="sub">pass ${p.Pass}${p.PlannedPasses ? ' of ~' + p.PlannedPasses : ''} · leaf ${p.Leaf}/${p.Leaves} · ${dur(p.Measured / 1e9)} measured</div>` : ''}</div>`;
    };
    const runners = (s.Runners || []).length ? s.Runners.map(runnerCard).join('') : `<div class="card" style="grid-column: span 2"><h3>Runners</h3><div class="big err">no runner has reported</div></div>`;
    const noise = d.Noise.Jobs
      ? `<div class="big">${d.Noise.MedianAbs.toFixed(2)}% <span class="muted">median</span> · ${d.Noise.P95Abs.toFixed(2)}% <span class="muted">p95</span></div><div class="sub">|Δ time| of identical binaries over ${d.Noise.Jobs} self-comparison jobs · <a href="#/noise">per operation</a></div>`
      : '<div class="big">not measured yet</div><div class="sub">master vs master runs every 6 hours and in the idle rotation</div>';
    app.innerHTML = `<h1>Dashboard</h1>
      <div class="cards">
        ${runners}
        <div class="card"><h3>Queue</h3><div class="big">${s.Queued}</div><div class="sub">jobs waiting · ${s.Active} running</div></div>
        <div class="card"><h3>Commit jobs</h3><div class="big">${s.Done} done</div><div class="sub">${s.Failed} failed</div></div>
        <div class="card"><h3>Noise floor</h3>${noise}</div>
        <div class="card"><h3>Machine</h3><div class="big">${esc(s.Host)}</div><div class="sub">load ${esc(s.Load)} · up ${esc(s.Uptime)}${s.FetchErr ? ` · <span class="err">${esc(s.FetchErr)}</span>` : ` · fetched ${ago(s.LastFetch)}`}</div></div>
      </div>
      <h2>Queue <span class="muted small">execution order</span></h2>${jobRows(d.Queue, { queue: true })}
      <h2>Finished <span class="muted small">most recent first</span></h2>${jobRows(d.Finished, { summaries: true })}
      <p class="note">Chips: geomean of head/base time per engine and object. The job page has the per-operation detail; a single number is only a hint.</p>`;
  }

  async function viewJobs(kind) {
    const jobs = await get('/api/jobs?limit=500' + (kind ? '&kind=' + kind : ''));
    const tabs = ['', 'commit', 'release', 'noise', 'baseline'].map(k => `<a href="#/jobs${k ? '?kind=' + k : ''}" class="chip ${k === (kind || '') ? 'commit' : ''}">${k || 'all'}</a>`).join(' ');
    app.innerHTML = `<h1>Jobs</h1><div class="toolbar">${tabs}</div>${jobRows(jobs)}`;
  }

  /* job page: metric switch, delta charts, matrices */
  function buildMatrices(results) {
    const byObj = {};
    results.forEach(r => (byObj[r.Object] = byObj[r.Object] || []).push(r));
    return Object.keys(byObj).sort((a, b) => rank(OBJECTS, a) - rank(OBJECTS, b) || a.localeCompare(b)).map(obj => {
      const rs = byObj[obj];
      // Async engines measure one operation; they show inside the parent
      // engine's cell instead of a column of their own.
      const engines = sortEngines([...new Set(rs.filter(r => !r.Baseline && !r.Engine.endsWith('Async')).map(r => r.Engine))]);
      const asyncEngines = sortEngines([...new Set(rs.filter(r => !r.Baseline && r.Engine.endsWith('Async')).map(r => r.Engine))]);
      const baselines = sortEngines([...new Set(rs.filter(r => r.Baseline).map(r => r.Engine))]);
      const ops = [...new Set(rs.map(r => r.Op))].sort((a, b) => rank(OPS, a) - rank(OPS, b) || a.localeCompare(b));
      const cells = {};
      rs.forEach(r => cells[r.Engine + '/' + r.Op] = r);
      return { obj, engines, asyncEngines, baselines, ops, cells };
    });
  }

  const unmeasured = M => !M || (M.Base === 0 && M.Head === 0);
  function cellDelta(r, m, jobID, label) {
    const tab = m;
    m = metricFor(m, r.Engine, r);
    if (label && m !== tab) label += ' (time)';
    else if (label && allThreads(r) && (m.key === 'Cycles' || m.key === 'Instrs')) label += ' (all threads)';
    const M = r[m.key];
    if (unmeasured(M)) return `<div class="d">${label ? `<span class="muted small">${label}</span>` : ''}<span class="muted">not measured</span></div>`;
    const link = jobID ? `#/job/${jobID}/leaf/${r.Engine}/${r.Object}/${r.Op}` : null;
    const body = `<div class="d">${label ? `<span class="muted small">${label}</span>` : ''}<span class="v">${m.fmt(M.Base)}<span class="arrow">→</span><b>${m.fmt(M.Head)}</b></span>${(m.key === 'Ns' || m.key === 'Cycles') && sig(M, r.Engine + '/' + r.Object + '/' + r.Op) ? kindChip(r) : ''}${badge(M, r.Engine + '/' + r.Object + '/' + r.Op)}</div>`;
    return link ? `<a href="${link}" title="every sample of this leaf">${body}</a>` : body;
  }
  function cellAbs(r, m) {
    const M = r[m.key];
    if (unmeasured(M)) return '<span class="muted">not measured</span>';
    return `<div class="d"><span class="v"><b>${m.fmt(M.Head)}</b></span><span class="ci">cv ${M.CVHead.toFixed(1)}%</span></div>`;
  }

  // refCell lists the reference libraries of one operation: each one's
  // value, and how the head of every engine of ours compares (ours ÷ theirs,
  // below 1 is faster or smaller). An async reference compares with our
  // async engines.
  function refCell(mx, m, op) {
    const lines = mx.baselines.map(b => {
      const ref = mx.cells[b + '/' + op];
      const isAsync = b.endsWith('Async');
      const mAll = m;
      m = metricFor(mAll, b, ref);
      if (!ref || unmeasured(ref[m.key])) { m = mAll; return ''; }
      const ours = (isAsync ? mx.asyncEngines : mx.engines).map(e => {
        const r = mx.cells[e + '/' + op];
        if (!r || unmeasured(r[m.key]) || !(ref[m.key].Head > 0)) return '';
        const x = r[m.key].Head / ref[m.key].Head;
        return `<span class="${x < 0.95 ? 'better' : x > 1.05 ? 'worse' : 'muted'}">${e.replace('Async', '')} ${x.toFixed(2)}×</span>`;
      }).filter(Boolean).join(' ');
      const line = `<div class="ref"><span class="muted">${b.replace('Async', ' async')}${m !== mAll ? ' (time)' : ''}</span><b>${m.fmt(ref[m.key].Head)}</b><span class="ours">${ours}</span></div>`;
      m = mAll;
      return line;
    }).filter(Boolean).join('');
    return lines || '<span class="muted">-</span>';
  }

  function matrixTable(mx, m, jobID) {
    const head = `<tr><th>Operation</th>${mx.engines.map(e => `<th class="grp">${e} <span class="muted" style="text-transform:none;letter-spacing:0">base → head</span></th>`).join('')}${mx.baselines.length ? '<th class="grp">Other libraries <span class="muted" style="text-transform:none;letter-spacing:0">their value · ours ÷ theirs</span></th>' : ''}</tr>`;
    const rows = mx.ops.map(op => {
      const tds = mx.engines.map(e => {
        const r = mx.cells[e + '/' + op], ra = mx.cells[e + 'Async/' + op];
        return `<td class="grp cell">${r ? cellDelta(r, m, jobID) : '<span class="muted">-</span>'}${ra ? cellDelta(ra, m, jobID, 'async') : ''}</td>`;
      }).join('');
      const refs = mx.baselines.length ? `<td class="grp small refs">${refCell(mx, m, op)}</td>` : '';
      return `<tr><td class="mono"><a href="#/op/${mx.obj}/${op}">${op}</a></td>${tds}${refs}</tr>`;
    }).join('');
    return `<table><thead>${head}</thead><tbody>${rows}</tbody></table>`;
  }


  // range is the span of the single-run deltas of a metric: the bar of
  // the delta chart. A result from before the range was recorded, or with
  // one run, spans its mean only.
  function range(M) {
    if (M.DMin === M.DMax && M.DMin === 0 && M.Delta !== 0) return [M.Delta, M.Delta];
    return [Math.min(M.DMin, delta(M)), Math.max(M.DMax, delta(M))];
  }
  // body is the middle half of the per-pass deltas (first to third
  // quartile): the thick part of a candle. The whisker is range().
  function body(M) {
    if (!(M.PN > 1)) return [delta(M), delta(M)];
    return [Math.min(M.PQ1, delta(M)), Math.max(M.PQ3, delta(M))];
  }
  // chartRows lists the rows of an object's chart: its operations, and
  // below an operation that the async engines run too, a row of its own
  // for them. Every row then holds one candle per engine; the async engine
  // takes the colour of its synchronous one. With sameUnit, an async row
  // whose values are in another unit on this tab is left out.
  function chartRows(mx, m, sameUnit) {
    const rows = [];
    mx.ops.forEach(op => {
      rows.push({ label: op, op, async: false });
      const eng = mx.asyncEngines.filter(e => mx.cells[e + '/' + op]);
      if (!eng.length) return;
      const cell = mx.cells[eng[0] + '/' + op];
      const other = metricFor(m, eng[0], cell) !== m;
      if (other && sameUnit) return;
      rows.push({ label: op + (other ? ' (async, time)' : allThreads(cell) && (m.key === 'Cycles' || m.key === 'Instrs') ? ' (async, all threads)' : ' (async)'), op, async: true });
    });
    return rows;
  }
  // rowCell is the result an engine shows in a row, with the metric it is
  // read by, or null.
  function rowCell(mx, m, e, row) {
    const engine = row.async ? e + 'Async' : e;
    if (row.async && !mx.asyncEngines.includes(engine)) return null;
    const r = mx.cells[engine + '/' + row.op], key = metricFor(m, engine, r).key;
    return r && !unmeasured(r[key]) ? { r, key, engine, op: row.op } : null;
  }
  // candleTip is the callout of a candle: the engine and row as the
  // title in the engine's colour, then one aligned line per fact. rows
  // returns [name, value] pairs for the hovered cell.
  function candleTip(rows) {
    const css = v => getComputedStyle(document.body).getPropertyValue(v).trim();
    return {
      position: 'pointer', backgroundColor: css('--card'), borderColor: css('--muted'), borderWidth: 1, cornerRadius: 6, padding: 10, caretSize: 6, displayColors: false,
      titleColor: ctx => (ctx.tooltip && ctx.tooltip.dataPoints && ctx.tooltip.dataPoints[0] && ctx.tooltip.dataPoints[0].dataset.borderColor) || css('--fg'),
      titleFont: { size: 12, weight: '600' }, titleMarginBottom: 8,
      bodyColor: css('--fg'), bodyFont: { family: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace', size: 11 }, bodySpacing: 4,
      footerColor: css('--muted'), footerFont: { size: 11, weight: 'normal' }, footerMarginTop: 8,
      callbacks: {
        title: items => { const c = items[0].dataset.meta[items[0].dataIndex]; return `${c.engine} · ${items[0].label}`; },
        label: ctx => {
          const lines = rows(ctx.dataset.meta[ctx.dataIndex], ctx.dataset).filter(Boolean);
          const w = Math.max(...lines.map(l => l[0].length));
          return lines.map(l => l[0].padEnd(w + 2) + l[1]);
        },
        footer: items => items[0].dataset.own === false ? '' : 'click for the single runs',
      },
    };
  }
  // barLabels draws the mean as a tick on each bar and writes it beyond
  // the bar's far end.
  const barLabels = {
    id: 'barLabels',
    afterDatasetsDraw(c, args, opts) {
      const ctx = c.ctx, x = c.scales.x, key = opts.key;
      ctx.save();
      ctx.font = '11px ' + Chart.defaults.font.family;
      ctx.textBaseline = 'middle';
      c.data.datasets.forEach((ds, di) => {
        const meta = c.getDatasetMeta(di);
        if (meta.hidden) return;
        meta.data.forEach((bar, i) => {
          const cell = ds.meta[i];
          if (!cell) return;
          const M = cell.r[cell.key], [lo, hi] = range(M), [b0, b1] = body(M);
          const h = Math.max(4, bar.height), area = c.chartArea;
          // whisker: every pass, a thin line clipped to the chart
          const wl = Math.max(area.left, x.getPixelForValue(lo)), wr = Math.min(area.right, x.getPixelForValue(hi));
          ctx.strokeStyle = ds.borderColor;
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          ctx.moveTo(wl, bar.y); ctx.lineTo(wr, bar.y);
          // end caps, or an arrow head where the whisker leaves the chart
          [[lo, wl, -1], [hi, wr, 1]].forEach(([v, px, dir]) => {
            const out = dir < 0 ? x.getPixelForValue(v) < area.left : x.getPixelForValue(v) > area.right;
            if (out) { ctx.moveTo(px - dir * 5, bar.y - 3); ctx.lineTo(px, bar.y); ctx.lineTo(px - dir * 5, bar.y + 3); }
            else { ctx.moveTo(px, bar.y - 3); ctx.lineTo(px, bar.y + 3); }
          });
          ctx.stroke();
          // median
          const xm = x.getPixelForValue(delta(M));
          ctx.fillStyle = getComputedStyle(document.body).getPropertyValue('--fg').trim() || '#fff';
          ctx.fillRect(xm - 1, bar.y - h / 2 - 1, 2, h + 2);
          const right = delta(M) >= 0;
          ctx.textAlign = right ? 'left' : 'right';
          ctx.fillStyle = ds.borderColor;
          const edge = right ? Math.min(area.right - 44, Math.max(x.getPixelForValue(Math.max(b1, 0)), wr)) : Math.max(area.left + 44, Math.min(x.getPixelForValue(Math.min(b0, 0)), wl));
          ctx.fillText((delta(M) > 0 ? '+' : '') + delta(M).toFixed(2) + '%', edge + (right ? 6 : -6), bar.y);
        });
      });
      ctx.restore();
    },
  };
  // noiseBand shades the noise floor of each operation around zero.
  const noiseBand = {
    id: 'noiseBand',
    beforeDatasetsDraw(c, args, opts) {
      const floors = opts.floors || [];
      const x = c.scales.x, y = c.scales.y, ctx = c.ctx;
      ctx.save();
      ctx.fillStyle = getComputedStyle(document.body).getPropertyValue('--line2').trim() || '#8884';
      ctx.beginPath();
      ctx.rect(x.left, y.top, x.width, y.height);
      ctx.clip();
      floors.forEach((f, i) => {
        if (!f) return;
        const h = y.getPixelForValue(i + 0.5) - y.getPixelForValue(i - 0.5);
        ctx.fillRect(x.getPixelForValue(-f), y.getPixelForValue(i) - h / 2 + 2, x.getPixelForValue(f) - x.getPixelForValue(-f), h - 4);
      });
      ctx.restore();
    },
  };
  function deltaChartCfg(mx, m, jobID) {
    const rows = chartRows(mx, m, false), labels = rows.map(r => r.label);
    const datasets = mx.engines.map((e, i) => {
      const color = COLORS[i % COLORS.length];
      const cells = rows.map(row => rowCell(mx, m, e, row));
      return {
        label: e, type: 'bar', backgroundColor: color + 'aa', borderColor: color, borderWidth: 1,
        data: cells.map(c => c ? body(c.r[c.key]) : null),
        meta: cells, whisker: c => range(c.r[c.key]),
        barPercentage: 0.8, categoryPercentage: 0.8, minBarLength: 3,
      };
    });
    const floors = rows.map((row, i) => Math.max(0, ...datasets.map(d => { const c = d.meta[i]; return c ? bandOf(c.r[c.key], c.engine + '/' + mx.obj + '/' + c.op) : 0; })));
    // The axis shows every candle with both ends of its thin line, zero
    // and the bands, with room for the labels; it need not be symmetric.
    const ends = datasets.flatMap(d => d.meta.filter(Boolean).flatMap(c => range(c.r[c.key])));
    const maxFloor = Math.max(0.5, ...floors);
    let lo = Math.min(0, -maxFloor, ...ends), hi = Math.max(0, maxFloor, ...ends);
    const pad = (hi - lo) * 0.09;
    lo -= pad; hi += pad;
    return {
      data: { labels, datasets },
      plugins: [noiseBand, barLabels],
      options: {
        indexAxis: 'y', responsive: true, maintainAspectRatio: false, animation: false, interaction: { mode: 'candle' },
        onClick: (ev, els, c) => { if (els.length && jobID) { const c = datasets[els[0].datasetIndex].meta[els[0].index]; location.hash = `#/job/${jobID}/leaf/${c.engine}/${mx.obj}/${c.op}`; } },
        onHover: (ev, els) => { ev.native.target.style.cursor = els.length ? 'pointer' : 'default'; },
        scales: {
          x: { min: lo, max: hi, ticks: { callback: v => (v > 0 ? '+' : '') + (Math.round(v * 100) / 100) + '%', maxTicksLimit: 9 }, grid: { color: ctx => ctx.tick.value === 0 ? getComputedStyle(document.body).getPropertyValue('--fg') : getComputedStyle(document.body).getPropertyValue('--line2') } },
          y: { grid: { display: false } },
        },
        plugins: {
          legend: { labels: { boxWidth: 12, generateLabels: c => {
            const items = Chart.defaults.plugins.legend.labels.generateLabels(c);
            items.push({ text: 'grey band: what is not a change here (noise floor of the operation, widened where the layouts scatter this pair)', fillStyle: getComputedStyle(document.body).getPropertyValue('--line2').trim(), strokeStyle: getComputedStyle(document.body).getPropertyValue('--muted').trim(), lineWidth: 1, hidden: false, datasetIndex: -1 });
            return items;
          } }, onClick: (e, item, legend) => { if (item.datasetIndex >= 0) Chart.defaults.plugins.legend.onClick.call(legend, e, item, legend); } },
          noiseBand: { floors },
          barLabels: { key: m.key },
          tooltip: candleTip(c => {
            const M = c.r[c.key], [lo, hi] = range(M), [b0, b1] = body(M), cm = Object.values(METRICS).find(x => x.key === c.key) || m;
            const band = bandOf(M, c.engine + '/' + mx.obj + '/' + c.op), ch = Math.abs(delta(M)) > band && (!(M.PN > 0) || M.PAgree >= 0.75 * M.PN);
            return [
              ['median', pct(delta(M))],
              M.PN > 1 && ['middle half', `${pct(b0)} … ${pct(b1)}`],
              M.PN > 1 && ['all passes', `${pct(lo)} … ${pct(hi)}`],
              M.PN > 0 && ['passes', `${M.PN}, ${M.PAgree} in the same direction`],
              ['band', `±${band.toFixed(2)}%  →  ${ch ? (delta(M) < 0 ? 'faster' : 'slower') : 'no change'}`],
              ['base → head', `${cm.fmt(M.Base)} → ${cm.fmt(M.Head)} ${cm.unit || ''}`],
              cpi(c.r, 'Head') > 0 && ['cycles/instr', `${cpi(c.r, 'Base').toFixed(3)} → ${cpi(c.r, 'Head').toFixed(3)}`],
              ch && kindOf(c.r) && ['kind', kindOf(c.r) === 'code' ? `code: instructions ${pct(delta(c.r.Instrs))}` : `same work: instructions ${pct(delta(c.r.Instrs))}`],
            ];
          }),
        },
      },
    };
  }

  // headRange is the span of the single runs' head values of a metric; a
  // result without the range recorded spans its mean only.
  function headRange(M) {
    if (!(M.HMax > 0)) return [M.Head, M.Head];
    return [Math.min(M.HMin, M.Head), Math.max(M.HMax, M.Head)];
  }
  // headBody is the middle half of the single runs' head values.
  function headBody(M) {
    if (!(M.HQ3 > 0)) return [M.Head, M.Head];
    return [M.HQ1, M.HQ3];
  }
  // absLabels marks the mean on each bar with a white tick, the base mean
  // of our own engines with a grey one, and writes the mean beyond the bar.
  const absLabels = {
    id: 'absLabels',
    afterDatasetsDraw(c, args, opts) {
      const ctx = c.ctx, x = c.scales.x, fg = getComputedStyle(document.body).getPropertyValue('--fg').trim() || '#fff', muted = getComputedStyle(document.body).getPropertyValue('--muted').trim() || '#888';
      ctx.save();
      ctx.font = '11px ' + Chart.defaults.font.family;
      ctx.textBaseline = 'middle';
      ctx.textAlign = 'left';
      c.data.datasets.forEach((ds, di) => {
        const meta = c.getDatasetMeta(di);
        if (meta.hidden) return;
        meta.data.forEach((bar, i) => {
          const cell = ds.meta[i];
          if (!cell) return;
          const M = cell.r[cell.key], h = Math.max(4, bar.height);
          const [hlo, hhi] = headRange(M);
          ctx.strokeStyle = ds.borderColor;
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          ctx.moveTo(x.getPixelForValue(hlo), bar.y); ctx.lineTo(x.getPixelForValue(hhi), bar.y);
          ctx.moveTo(x.getPixelForValue(hlo), bar.y - 3); ctx.lineTo(x.getPixelForValue(hlo), bar.y + 3);
          ctx.moveTo(x.getPixelForValue(hhi), bar.y - 3); ctx.lineTo(x.getPixelForValue(hhi), bar.y + 3);
          ctx.stroke();
          let right = x.getPixelForValue(hhi);
          if (ds.own && M.Base > 0) {
            ctx.fillStyle = muted;
            ctx.fillRect(x.getPixelForValue(M.Base) - 1, bar.y - h / 2, 2, h);
            right = Math.max(right, x.getPixelForValue(M.Base));
          }
          ctx.fillStyle = fg;
          ctx.fillRect(x.getPixelForValue(M.Head) - 1, bar.y - h / 2, 2, h);
          ctx.fillStyle = ds.borderColor;
          ctx.fillText(opts.fmt(M.Head), right + 5, bar.y);
        });
      });
      ctx.restore();
    },
  };
  // absChartCfg: per operation the measured values of every engine and
  // reference library on a logarithmic axis (operations of one object span
  // nanoseconds to seconds). A bar spans the head values of the single
  // runs, the tick on it is their mean. An engine whose values are in
  // another unit on this tab (the async engines on the counter tabs) is
  // left out.
  const absEngines = mx => [...mx.engines, ...(chartLibs ? mx.baselines : [])];
  // absCell is the value an engine or library shows in a row, or null; a
  // library has no async rows.
  function absCell(mx, m, e, row) {
    if (mx.baselines.includes(e)) {
      const r = !row.async && mx.cells[e + '/' + row.op];
      return r && metricFor(m, e) === m && !unmeasured(r[m.key]) && r[m.key].Head > 0 ? { r, key: m.key, engine: e, op: row.op } : null;
    }
    const c = rowCell(mx, m, e, row);
    return c && c.key === m.key && c.r[m.key].Head > 0 ? c : null;
  }
  // absRowBars is the largest number of bars one row of the object shows;
  // every row gets room for that many bars of the same thickness.
  const absRowBars = (mx, m) => Math.max(1, ...chartRows(mx, m, true).map(row => absEngines(mx).filter(e => absCell(mx, m, e, row)).length));
  function absChartCfg(mx, m, jobID) {
    const rows = chartRows(mx, m, true), labels = rows.map(r => r.label);
    const datasets = absEngines(mx).map((e, i) => {
      const color = COLORS[i % COLORS.length], ref = mx.baselines.includes(e);
      const cells = rows.map(row => absCell(mx, m, e, row));
      return {
        label: e + (ref ? ' (library)' : ''), type: 'bar', backgroundColor: color + (ref ? '55' : 'aa'), borderColor: color, borderWidth: 1,
        data: cells.map(c => c ? headBody(c.r[m.key]) : null),
        meta: cells, own: !ref, skipNull: true, whisker: c => [...headRange(c.r[m.key]), ...(!ref && c.r[m.key].Base > 0 ? [c.r[m.key].Base] : [])],
        barThickness: 9, minBarLength: 3,
      };
    });
    const vals = datasets.flatMap(d => d.meta.filter(Boolean).flatMap(c => [...headRange(c.r[m.key]), d.own && c.r[m.key].Base > 0 ? c.r[m.key].Base : c.r[m.key].Head]));
    return {
      data: { labels, datasets },
      plugins: [absLabels],
      options: {
        indexAxis: 'y', responsive: true, maintainAspectRatio: false, animation: false, interaction: { mode: 'candle' },
        onClick: (ev, els) => { if (els.length && jobID && datasets[els[0].datasetIndex].own) { const c = datasets[els[0].datasetIndex].meta[els[0].index]; location.hash = `#/job/${jobID}/leaf/${c.engine}/${mx.obj}/${c.op}`; } },
        scales: {
          x: { type: 'logarithmic', min: vals.length ? Math.min(...vals) / 1.6 : undefined, max: vals.length ? Math.max(...vals) * 2.5 : undefined, ticks: { callback: v => m.fmt(v), maxTicksLimit: 10 }, title: { display: true, text: `${m.label} ${m.unit}, logarithmic` } },
          y: { grid: { display: false } },
        },
        plugins: {
          legend: { labels: { boxWidth: 12 } },
          absLabels: { fmt: m.fmt, key: m.key },
          tooltip: candleTip((c, ds) => {
            const M = c.r[m.key], [lo, hi] = headRange(M), [b0, b1] = headBody(M), u = ' ' + (m.unit || '');
            return [
              ['mean', m.fmt(M.Head) + u],
              c.r.N > 1 && ['middle half', `${m.fmt(b0)} … ${m.fmt(b1)}`],
              c.r.N > 1 && ['single runs', `${m.fmt(lo)} … ${m.fmt(hi)}`],
              ['runs', String(c.r.N)],
              ds.own && M.Base > 0 && ['base', `${m.fmt(M.Base)}${u}  (head ${pct(M.Delta)})`],
            ];
          }),
        },
      },
    };
  }

  async function viewJob(id) {
    const d = await get('/api/job/' + id);
    repo = d.Repo;
    leafNoise = (d.Noise && d.Noise.PerLeaf) || {};
    const j = d.Job, m = METRICS[metric];
    const mxs = buildMatrices(d.Results);
    const r = d.Runner, p = (r && r.Progress) || {};
    const live = d.Live ? `<div class="card wide"><div class="sub">${r ? esc(r.Runner) + ': ' + esc(r.Phase) : 'running on ' + esc(j.Runner) + ' (no live report yet)'}</div>${r && p.Leaves ? `<div class="bar"><div style="width:${r.Percent}%"></div></div><div class="sub">pass ${p.Pass}${p.PlannedPasses ? ' of ~' + p.PlannedPasses : ''} · leaf ${p.Leaf}/${p.Leaves} · ${dur(p.Measured / 1e9)} measured · running ${dur(r.Running)} · provisional results from the samples so far, refreshes every 20 s</div>` : ''}</div>` : '';
    const runs = d.Runs && d.Runs.length > 1 ? ` · pooled over ${d.Runs.length} runs (${d.Runs.map(r => `<a href="#/job/${r}">#${r}</a>`).join(', ')})` : '';
    app.innerHTML = `<h1>Job #${j.ID} ${chip(j)}${d.Live ? '<span class="chip running">live</span>' : ''}</h1>
      ${live}
      <div class="cards">
        <div class="card"><h3>Head</h3><div class="mono">${commitLink(j.HeadSHA)} ${esc(j.HeadDesc).replace(/^[0-9a-f]{7} /, '')}</div><div class="sub mono">${esc(j.Branch)}${prLink(j)}</div></div>
        <div class="card"><h3>Base (${esc(j.BaseRef)})</h3><div class="mono">${commitLink(j.BaseSHA)} ${esc(j.BaseDesc).replace(/^[0-9a-f]{7} /, '')}</div>${j.BaseSHA !== j.HeadSHA ? `<div class="sub"><a href="${repo}/compare/${j.BaseSHA}...${j.HeadSHA}" target="_blank" rel="noopener">diff on GitHub</a></div>` : ''}</div>
        <div class="card"><h3>Measurement</h3><div class="mono">${j.Passes} passes${j.Seconds ? ', ' + dur(j.Seconds) : ''}${runs}</div><div class="sub">runner ${esc(j.Runner || '-')}</div><div class="sub">${esc(j.GoVersion)} · harness ${j.Harness}${d.Steal ? ` · ${d.Steal} steal ticks (wake-ups, within the limit)` : ' · no steal'}</div><div class="sub">queued ${when(j.Created)} · finished ${when(j.Finished)}</div></div>
        ${buildCard(d.Builds)}
        <div class="card"><h3>Ratio per engine <span class="muted" style="text-transform:none">(cycles when counted, else time)</span></h3><div class="chips">${engineTotals(d.Summaries || []).map(x => `<span class="chip" title="geomean of head/base ${x.Cycles ? 'cycles' : 'time'} over ${x.N} operations of every object">${x.Engine} <b>${pct(x.Geomean, 1)}</b></span>`).join('') || '<span class="muted">-</span>'}</div><details class="small" style="margin-top:6px"><summary>per object</summary><div class="chips" style="margin-top:4px">${(d.Summaries || []).map(x => `<span class="chip">${x.Engine}·${x.Object} <b>${pct(x.Geomean, 1)}</b></span>`).join('')}</div></details></div>
      </div>
      ${j.Note ? `<p class="note">${esc(j.Note)}</p>` : ''}${d.BaselineJob ? `<p class="note">Other libraries: values from the reference job <a href="#/job/${d.BaselineJob}">#${d.BaselineJob}</a> on the same machine and payload. They cannot hash Gloas objects unless they implement progressive merkleization (PrysmSSZ does), and none but PrysmSSZ can express the Gloas state.</p>` : ''}${j.Error ? `<pre class="err">${esc(j.Error)}</pre>` : ''}
      <div class="toolbar">${metricTabs()}<div class="tabs" id="modeTabs"><button data-mode="rel" class="${chartMode === 'rel' ? 'active' : ''}" title="charts show head against base in percent">change in %</button><button data-mode="abs" class="${chartMode === 'abs' ? 'active' : ''}" title="charts show the measured values">measured values</button></div>${chartMode === 'abs' ? `<label class="check"><input type="checkbox" id="chartLibs" ${chartLibs ? 'checked' : ''}> with the other libraries</label>` : ''}<span class="muted">${m.label} ${m.unit}: base → head and Δ per engine and operation. Δ is the median over the passes. Green/red: a change (outside the band, most passes agree); bold: |Δ| ≥ 5%; grey: no change. Click a bar or cell for every sample.</span></div>
      ${mxs.map((mx, i) => { const bars = chartMode === 'abs' ? absRowBars(mx, m) : mx.engines.length, rows = chartRows(mx, m, chartMode === 'abs').length; return `<h2>${objName(mx.obj)}</h2><div class="chart" style="height:${rows * Math.max(30, bars * 14 + 8) + 80}px"><canvas id="dc${i}"></canvas></div><p class="note">${chartMode === 'abs' ? `One bar per engine${chartLibs ? ' and reference library' : ''} per operation, in measured ${m.label.toLowerCase()}: the thick body is the middle half of the single runs' values and the thin line reaches to the lowest and the highest run, the white tick and the number are their mean, the grey tick on our own engines is the base's mean. The axis is logarithmic because the operations of one object span several orders of magnitude, so a bar is short when the runs agree.${chartLibs ? ' Paler bars are the other libraries, from the latest reference job.' : ''}` : `One candle per engine (colours in the legend) per operation. The thick body is the middle half of the per-pass ${m.label.toLowerCase()} deltas (each pass links head and base with another function layout and measures them minutes apart), the thin line reaches to the lowest and the highest pass, the white tick and the number are the median. A single pass far off shows as a long thin line and leaves the body and the scale alone; an arrow head means the line continues beyond the chart. The grey band behind each row is what does not count as a change there: the operation's noise floor on this machine, widened where the passes scatter. A result is a change when its median lies outside the band and at least three quarters of the passes agree. The async engines have a row of their own below the operation they run, in the colour of their engine. On the counter tabs that row shows the cycles and instructions of all their threads together (the total work, not the latency), or time for a job measured before all threads were counted.`}</p>${matrixTable(mx, m, j.ID)}`; }).join('') || '<p class="muted">no results yet</p>'}
      ${d.Noise && d.Noise.Jobs ? `<p class="note">Noise floor over ${d.Noise.Jobs} self-comparisons: median |Δ time| ${d.Noise.MedianAbs.toFixed(2)}%, p95 ${d.Noise.P95Abs.toFixed(2)}%.</p>` : ''}
      <details><summary>Raw files</summary><p class="mono">${(d.Files || []).map(f => `<a href="/raw/${j.ID}/${f}" target="_blank">${f}</a>`).join(' · ')}</p></details>`;
    bindMetricTabs(() => viewJob(id));
    destroyCharts();
    mxs.forEach((mx, i) => chart('dc' + i, (chartMode === 'abs' ? absChartCfg : deltaChartCfg)(mx, m, j.ID)));
    document.querySelectorAll('#modeTabs button').forEach(b => b.onclick = () => { chartMode = b.dataset.mode; localStorage.setItem('chartMode', chartMode); viewJob(id); });
    const libs = document.getElementById('chartLibs');
    if (libs) libs.onchange = () => { chartLibs = libs.checked; localStorage.setItem('chartLibs', chartLibs ? '1' : '0'); viewJob(id); };
    if (d.Live) scheduleRefresh(20000);
  }

  async function viewLeaf(id, engine, object, op) {
    const d = await get(`/api/job/${id}/leaf/${engine}/${object}/${op}`);
    const r = d.Result, m = METRICS[metric];
    const samples = d.Samples.filter(s => !(s.Extra && s.Extra.diag));
    const diag = d.Samples.filter(s => s.Extra && s.Extra.diag);
    const X = r.Extra || {};
    const xfmt = k => EXTRAS[k].bytes ? fmtBytes : fmtNum;
    const xline = k => X[k] ? `<div class="sub mono">${EXTRAS[k].name}: ${r.Baseline || !X[k].Base ? xfmt(k)(X[k].Head) : `${xfmt(k)(X[k].Base)} → ${xfmt(k)(X[k].Head)}${X[k].Base > 0 ? ' ' + pct((X[k].Head - X[k].Base) / X[k].Base * 100, 1) : ''}`} <span class="muted">(${X[k].N} run${X[k].N === 1 ? '' : 's'} per side)</span></div>` : '';
    const counterCard = ['br-miss', 'l2-miss', 'fe-stall', 'l1d-miss'].some(k => X[k]) ? `<div class="card"><h3>More counters per op <span class="muted" style="text-transform:none">(median)</span></h3>${['br-miss', 'fe-stall', 'l1d-miss', 'l2-miss'].map(xline).join('')}${X.threads ? `<div class="sub">counted over all ${X.threads.Head} threads of the process</div>` : ''}</div>` : '';
    const memCard = X.stack || X.retained ? `<div class="card"><h3>Memory beyond allocations</h3>${['retained', 'stack'].map(xline).join('')}</div>` : '';
    const pairText = s => s.Extra ? Object.keys(EXTRAS).filter(k => !EXTRAS[k].bytes && s.Extra[k] !== undefined).map(k => `${EXTRAS[k].short} ${fmtNum(s.Extra[k])}`).join(' · ') : '';
    // offClock marks a run whose clock rate (cycles per ns) is more than
    // 1% off the median of all runs shown.
    const clocks = samples.filter(s => s.Cycles && s.Ns && !engine.endsWith('Async')).map(s => s.Cycles / s.Ns).sort((a, b) => a - b);
    const clockMed = clocks.length ? clocks[Math.floor(clocks.length / 2)] : 0;
    const offClock = s => clockMed > 0 && s.Cycles && s.Ns && Math.abs(s.Cycles / s.Ns / clockMed - 1) > 0.01;
    const stat = (M, f) => r.Baseline
      ? `<div class="big">${f(M.Head)}</div><div class="sub">cv ${M.CVHead.toFixed(2)}%</div>`
      : `<div class="big">${badge(M)}</div><div class="sub">${f(M.Base)} → ${f(M.Head)} · 95% [${pct(M.Lo, 1)}, ${pct(M.Hi, 1)}] · p ${M.P.toFixed(3)} · median Δ ${pct(M.MedDelta)} · cv ${M.CVBase.toFixed(2)}% / ${M.CVHead.toFixed(2)}%</div>`;
    app.innerHTML = `<h1><span class="mono">${engine} / ${object} / ${op}</span> <span class="muted small">job <a href="#/job/${id}">#${id}</a>${d.Runs.length > 1 ? `, pooled over ${d.Runs.length} runs` : ''}</span></h1>
      <div class="cards">
        <div class="card"><h3>Time per op</h3>${stat(r.Ns, fmtNs)}</div>
        ${r.Cycles.Head ? `<div class="card"><h3>Cycles per op</h3>${stat(r.Cycles, fmtNum)}</div><div class="card"><h3>Instructions per op</h3>${stat(r.Instrs, fmtNum)}</div>` : ''}
        ${cpi(r, 'Head') > 0 ? `<div class="card"><h3>Cycles per instruction</h3><div class="big">${r.Baseline ? cpi(r, 'Head').toFixed(3) : `${cpi(r, 'Base').toFixed(3)} → ${cpi(r, 'Head').toFixed(3)}`}</div><div class="sub">${r.Baseline || !kindOf(r) ? 'how fast the work executes' : kindText(r)}</div></div>` : ''}
        ${counterCard}
        <div class="card"><h3>Memory per op</h3>${stat(r.Bytes, fmtBytes)}</div>
        <div class="card"><h3>Allocations per op</h3>${stat(r.Allocs, fmtNum)}</div>
        ${memCard}
        <div class="card"><h3>Measurement</h3><div class="big">${r.N} × ${r.Iters}</div><div class="sub">measurements per side × iterations each${r.Steal ? ` · ${r.Steal} steal ticks (wake-ups)` : ''}</div></div>
      </div>
      <div class="toolbar">${metricTabs()}<span class="muted">every sample, in measurement order (pass, seed, side); dashed lines are the side means</span></div>
      <div class="chart h300"><canvas id="sc"></canvas></div>
      <table><thead><tr><th>Run</th><th>Side</th><th class="num">Pass</th><th>Seed</th><th class="num">Iterations</th><th class="num">Time</th><th class="num">Cycles</th><th class="num">Instrs</th><th class="num" title="cycles per instruction">Cyc/instr</th><th class="num" title="cycles per nanosecond: the clock rate the run saw. A run far from the others was throttled or disturbed.">GHz</th><th class="num">Memory</th><th class="num">Allocs</th><th class="num">Steal</th><th title="the two further counters of this pass, per op">Counters of the pass</th></tr></thead><tbody>
      ${samples.map(s => `<tr><td><a href="#/job/${s.JobID}">#${s.JobID}</a></td><td>${s.Side}</td><td class="num">${s.Pass + 1}</td><td class="mono">${s.Seed}</td><td class="num">${s.Iters}</td><td class="num">${fmtNs(s.Ns)}</td><td class="num">${s.Cycles ? fmtNum(s.Cycles) : '-'}</td><td class="num">${s.Instrs ? fmtNum(s.Instrs) : '-'}</td><td class="num">${s.Instrs && s.Cycles ? (s.Cycles / s.Instrs).toFixed(3) : '-'}</td><td class="num${offClock(s) ? ' warn' : ''}">${s.Cycles && s.Ns ? (s.Cycles / s.Ns).toFixed(3) : '-'}</td><td class="num">${fmtBytes(s.Bytes)}</td><td class="num">${fmtNum(s.Allocs)}</td><td class="num">${s.Steal}</td><td class="small mono">${pairText(s)}</td></tr>`).join('')}
      </tbody></table>
      ${diag.length ? `<h2>Diagnostic runs</h2><p class="note">Under one layout the two sides executed the same instructions in clearly different cycles. Each side was run again, untimed, with the counters outside the rotation, to show where the cycles went: uops that had to be decoded (not served from the op cache), instruction cache and TLB misses.</p>
      <table><thead><tr><th>Side</th><th>Seed</th><th class="num">Cycles</th><th class="num">Instrs</th><th class="num">Cyc/instr</th><th>Counters per op</th></tr></thead><tbody>
      ${diag.map(s => `<tr><td>${s.Side}</td><td class="mono">${s.Seed}</td><td class="num">${fmtNum(s.Cycles)}</td><td class="num">${fmtNum(s.Instrs)}</td><td class="num">${s.Instrs ? (s.Cycles / s.Instrs).toFixed(3) : '-'}</td><td class="small mono">${Object.keys(DIAG).filter(k => s.Extra[k] !== undefined).map(k => `${DIAG[k]} ${fmtNum(s.Extra[k])}`).join(' · ')}</td></tr>`).join('')}
      </tbody></table>` : ''}
      <p class="muted"><a href="#/op/${object}/${op}">history of this operation</a></p>`;
    bindMetricTabs(() => viewLeaf(id, engine, object, op));
    destroyCharts();
    const val = s => s[m.key];
    const sides = ['base', 'head'];
    const datasets = sides.map((side, i) => ({
      label: side, type: 'scatter', pointRadius: 5, backgroundColor: COLORS[i] + 'cc', borderColor: COLORS[i],
      data: samples.map((s, k) => s.Side === side ? { x: k + 1, y: val(s), s } : null).filter(Boolean),
    }));
    sides.forEach((side, i) => {
      const vs = samples.filter(s => s.Side === side).map(val);
      if (!vs.length) return;
      const mean = vs.reduce((a, b) => a + b, 0) / vs.length;
      datasets.push({ label: side + ' mean', type: 'line', borderColor: COLORS[i], borderDash: [5, 4], borderWidth: 1, pointRadius: 0, data: [{ x: 1, y: mean }, { x: samples.length, y: mean }] });
    });
    chart('sc', {
      data: { datasets },
      options: {
        responsive: true, maintainAspectRatio: false, animation: false,
        scales: { x: { type: 'linear', title: { display: true, text: 'run (pass · seed)' }, ticks: { stepSize: 1, callback: v => { const s = samples[v - 1]; return s ? `p${s.Pass + 1} · ${s.Seed}` : ''; } } }, y: { ticks: { callback: v => m.fmt(v) }, title: { display: true, text: m.unit } } },
        plugins: { legend: { labels: { boxWidth: 12 } }, tooltip: { callbacks: { label: ctx => { const s = ctx.raw.s; return s ? `${s.Side} pass ${s.Pass + 1} seed ${s.Seed} (job #${s.JobID}): ${m.fmt(ctx.raw.y)}, ${s.Iters} iterations${s.Steal ? ', ' + s.Steal + ' steal' : ''}` : `${ctx.dataset.label}: ${m.fmt(ctx.raw.y)}`; } } } },
      },
    });
  }

  // The engines shown in the operations table; remembered per browser.
  let opsHidden = new Set(JSON.parse(localStorage.getItem('opsHidden') || '[]'));
  // The branch whose values the operations pages show; remembered per
  // browser. Empty means the main branch.
  let opsBranch = localStorage.getItem('opsBranch') || '';
  const branchSelect = (branches, current) => `<select id="opsBranch" title="branch whose measured commits are shown">${branches.map(b => `<option value="${esc(b.Name)}" ${b.Name === current ? 'selected' : ''}>${esc(b.Name)}${b.PR ? ` (#${b.PR})` : ''} · ${b.Commits} commit${b.Commits === 1 ? '' : 's'}</option>`).join('')}</select>`;
  function bindBranchSelect(rerender) {
    const sel = document.getElementById('opsBranch');
    if (sel) sel.onchange = () => { opsBranch = sel.value; localStorage.setItem('opsBranch', opsBranch); rerender(); };
  }
  const trendText = (t, n) => `<span class="ci" title="change of the line fitted through the last ${n} measured commits, from the first to the last of them">trend ${pct(t, 1)} / ${n} commits</span>`;

  async function viewOps() {
    const d = await get('/api/ops' + (opsBranch ? '?branch=' + encodeURIComponent(opsBranch) : ''));
    if (opsBranch && !d.Commits && d.Branches.length && !d.Branches.some(b => b.Name === opsBranch)) { opsBranch = ''; localStorage.removeItem('opsBranch'); return viewOps(); }
    leafNoise = d.Noise || {};
    const rows = d.Rows, m = METRICS[metric];
    // One column per engine or library; its async variant has a row of
    // its own below the operation it runs.
    const all = sortEngines([...new Set(d.Engines.map(e => e.replace(/Async$/, '')))]);
    const engines = all.filter(e => !opsHidden.has(e));
    const cell = (r, e) => {
      const c = r.Cells[e];
      if (!c) return '<td class="grp muted">-</td>';
      const em = metricFor(m, e), M = c[em.key];
      if (unmeasured(M)) return '<td class="grp muted">-</td>';
      const key = e + '/' + r.Object + '/' + r.Op;
      const value = !c.Baseline && !d.Main && M.Base > 0
        ? `<span class="v">${em.fmt(M.Base)}<span class="arrow">→</span><b>${em.fmt(M.Head)}</b></span> ${badge(M, key)}`
        : `<b>${em.fmt(M.Head)}</b>`;
      return `<td class="grp">${c.Baseline ? value : `<a href="#/job/${c.JobID}/leaf/${e}/${r.Object}/${r.Op}">${value}</a>`}${M.TrendN ? ' ' + trendText(M.Trend, M.TrendN) : ''}</td>`;
    };
    const asyncRow = r => Object.keys(r.Cells).some(e => e.endsWith('Async'))
      ? `<tr><td><span class="muted">${r.Object}</span></td><td class="mono muted">${r.Op} (async${metricFor(m, 'Async') !== m ? ', time' : ''})</td>${engines.map(e => cell(r, e + 'Async')).join('')}</tr>` : '';
    const h = d.Head;
    app.innerHTML = `<h1>Operations</h1>
      <div class="toolbar">${metricTabs()}${branchSelect(d.Branches, d.Branch)}<span class="muted">${h ? `at ${commitLink(h.HeadSHA)} (job <a href="#/job/${h.ID}">#${h.ID}</a>)${d.PR ? ` · <a href="#/pr/${d.PR}">pull request #${d.PR}</a>` : ''}` : 'no measured commit on this branch'}</span></div>
      <p class="note">${d.Main
        ? `The value of every operation at the newest measured commit of ${esc(d.Branch)}, and its trend over the last ${d.Commits} measured commits (three are needed).`
        : `The base the branch started from and the value at its newest measured commit, both measured in the same job, with the change between them. The trend runs from the base through the ${d.Commits} measured commit${d.Commits === 1 ? '' : 's'} of the branch (three points are needed).`} The other libraries show their newest measurement, whatever the branch.</p>
      <div class="toolbar chips" id="engsel">${all.map(e => `<a href="#" data-e="${e}" class="chip ${opsHidden.has(e) ? '' : 'commit'}" title="show or hide this column">${e}</a>`).join(' ')}<a href="#" data-e="*" class="chip">all</a><a href="#" data-e="-" class="chip">ours only</a></div>
      <div style="overflow-x:auto"><table><thead><tr><th>Object</th><th>Operation</th>${engines.map(e => `<th class="grp">${e}${!d.Main && ['Codegen', 'Reflection'].includes(e) ? ' <span class="muted" style="text-transform:none;letter-spacing:0">base → head</span>' : ''}</th>`).join('')}</tr></thead><tbody>
      ${rows.map((r, i) => `<tr><td>${i > 0 && rows[i - 1].Object === r.Object ? `<span class="muted">${r.Object}</span>` : objName(r.Object)}</td><td class="mono"><a href="#/op/${r.Object}/${r.Op}">${r.Op}</a></td>${engines.map(e => cell(r, e)).join('')}</tr>${asyncRow(r)}`).join('')}
      </tbody></table></div>`;
    bindMetricTabs(viewOps);
    bindBranchSelect(viewOps);
    document.querySelectorAll('#engsel a').forEach(a => a.onclick = (ev => {
      ev.preventDefault();
      const e = a.dataset.e;
      if (e === '*') opsHidden = new Set();
      else if (e === '-') opsHidden = new Set(all.filter(x => !['Codegen', 'Reflection'].includes(x)));
      else if (opsHidden.has(e)) opsHidden.delete(e); else opsHidden.add(e);
      localStorage.setItem('opsHidden', JSON.stringify([...opsHidden]));
      viewOps();
    }));
  }

  /* ---------- pull request ---------- */
  // What the pull request chart shows: the change against the base in
  // percent, or measured values; for all operations (geomean, percent
  // only) or one operation.
  let prMode = localStorage.getItem('prMode') === 'abs' ? 'abs' : 'rel';
  let prLeaf = localStorage.getItem('prLeaf') || '';
  async function viewPR(n) {
    const d = await get('/api/pr/' + n);
    repo = d.Repo;
    const m = METRICS[metric];
    const chips = sums => engineTotals(sums).map(x => `<span class="chip" title="geomean over ${x.N} operations (${x.Cycles && !x.Engine.endsWith('Async') ? 'cycles' : 'time'})">${x.Engine} <b>${pct(x.Geomean, 1)}</b></span>`).join('') || '<span class="muted">-</span>';
    const measured = d.Rows.filter(r => r.Job && r.Job.State === 'done');
    const leaves = [...new Set(measured.flatMap(r => r.Results.map(x => x.Object + '/' + x.Op)))].sort((a, b) => { const [ao, ap] = a.split('/'), [bo, bp] = b.split('/'); return rank(OBJECTS, ao) - rank(OBJECTS, bo) || rank(OPS, ap) - rank(OPS, bp); });
    let leaf = leaves.includes(prLeaf) ? prLeaf : '';
    if (prMode === 'abs' && !leaf) leaf = leaves.includes('FuluState/HashTreeRoot') ? 'FuluState/HashTreeRoot' : (leaves[0] || '');
    const count = d.Rows.filter(r => !r.Detached).length;
    app.innerHTML = `<h1>Pull request <a href="${repo}/pull/${d.PR}" target="_blank" rel="noopener">#${d.PR}</a> <span class="mono muted" style="font-size:13px;font-weight:400">${esc(d.Branch || '')}</span></h1>
      <div class="toolbar">${metricTabs()}<div class="tabs" id="prMode"><button data-mode="rel" class="${prMode === 'rel' ? 'active' : ''}">change against the base</button><button data-mode="abs" class="${prMode === 'abs' ? 'active' : ''}">measured values</button></div>
        <select id="prLeaf">${prMode === 'rel' ? `<option value="">all operations (geomean)</option>` : ''}${leaves.map(l => `<option value="${l}" ${l === leaf ? 'selected' : ''}>${l.replace('/', ' / ')}</option>`).join('')}</select>
        <span class="muted">${measured.length} of ${count} commits measured</span></div>
      ${measured.length ? '<div class="chart h300"><canvas id="prc"></canvas></div>' : '<p class="muted">no commit of this pull request has been measured yet</p>'}
      <p class="note">The commits of the pull request, oldest first. A push is measured at its head, so commits pushed together share one measurement at the last of them. "Against the base" is the full effect of the pull request at that commit. "Against the previous measured commit" is what came in since; it compares two separate jobs and is less exact than a job's own comparison.</p>
      <table><thead><tr><th>Commit</th><th>Subject</th><th>Committed</th><th>Job</th><th>Against the base</th><th>Against the previous measured commit</th><th title="size of the compiled benchmark code and of the generated SSZ source at this commit, summed over the harness packages, with the change against the base">Code · generated</th></tr></thead><tbody>
      ${d.Rows.map(r => { const j = r.Job; const prev = r.Previous ? d.Rows.find(x => x.Job && x.Job.ID === r.Previous) : null; return `<tr class="${j ? '' : 'unmeasured'}"><td>${commitLink(r.SHA)}${r.Detached ? ' <span class="chip" title="measured earlier; a force push took this commit out of the branch">earlier head</span>' : ''}</td>
        <td class="desc">${esc(r.Subject)}</td><td class="muted">${when(r.Time)}</td>
        <td>${j ? `<a href="#/job/${j.ID}">#${j.ID}</a> ${chip(j)}` : '<span class="muted">not measured</span>'}</td>
        <td><div class="chips">${j && j.State === 'done' ? chips(r.Summaries) : j ? `<span class="muted">${esc(j.Note || j.State)}</span>` : ''}</div></td>
        <td><div class="chips">${prev ? chips(r.Step) + ` <a href="#/compare?a=${prev.SHA}&b=${r.SHA}">compare</a>` : ''}</div></td>
        <td class="small">${(() => { const b = buildRows(r.Builds); if (!b.length) return ''; const sum = (side, f) => b.reduce((a, x) => a + (x[side] ? x[side][f] : 0), 0); return `${sizeDelta(sum('head', 'TextBytes'), sum('base', 'TextBytes'))} · ${sizeDelta(sum('head', 'GenBytes'), sum('base', 'GenBytes'))}`; })()}</td></tr>`; }).join('')}
      </tbody></table>`;
    bindMetricTabs(() => viewPR(n));
    document.querySelectorAll('#prMode button').forEach(b => b.onclick = () => { prMode = b.dataset.mode; localStorage.setItem('prMode', prMode); viewPR(n); });
    const sel = document.getElementById('prLeaf');
    if (sel) sel.onchange = () => { prLeaf = sel.value; localStorage.setItem('prLeaf', prLeaf); viewPR(n); };
    destroyCharts();
    if (!measured.length) return;
    // One point per commit of the branch; a commit without a measurement
    // leaves a gap the line bridges.
    const xs = d.Rows;
    const engines = sortEngines([...new Set(measured.flatMap(r => leaf ? r.Results.filter(x => x.Object + '/' + x.Op === leaf).map(x => x.Engine) : engineTotals(r.Summaries).map(x => x.Engine)))]);
    const value = (r, e) => {
      if (!r.Job || r.Job.State !== 'done') return null;
      if (!leaf) { const t = engineTotals(r.Summaries).find(x => x.Engine === e); return t ? t.Geomean : null; }
      const [o, op] = leaf.split('/'), res = r.Results.find(x => x.Engine === e && x.Object === o && x.Op === op);
      if (!res) return null;
      const M = res[metricFor(m, e).key];
      if (unmeasured(M)) return null;
      return prMode === 'abs' ? M.Head : delta(M);
    };
    const fmtY = v => prMode === 'abs' ? m.fmt(v) : pct(v, 2);
    chart('prc', {
      type: 'line',
      data: { labels: xs.map(r => short(r.SHA).slice(0, 7)), datasets: engines.filter(e => prMode === 'rel' || metricFor(m, e) === m).map((e, i) => ({
        label: e + (metricFor(m, e) !== m && leaf ? ' (time)' : ''), borderColor: COLORS[rank(ENGINES, e) % COLORS.length], backgroundColor: COLORS[rank(ENGINES, e) % COLORS.length], tension: 0, pointRadius: 4, spanGaps: true,
        data: xs.map(r => value(r, e)) })) },
      options: { responsive: true, maintainAspectRatio: false, animation: false,
        onClick: (ev, els) => { if (els.length) { const r = xs[els[0].index]; if (r.Job) location.hash = `#/job/${r.Job.ID}`; } },
        scales: {
          y: { title: { display: true, text: prMode === 'abs' ? `${leaf.replace('/', ' / ')}: ${m.label.toLowerCase()} ${m.unit}` : `${leaf ? leaf.replace('/', ' / ') : 'geomean over all operations'}: Δ against the base` }, ticks: { callback: fmtY } },
          x: { title: { display: true, text: 'commits of the pull request, oldest first' }, ticks: { maxRotation: 0, autoSkip: true } } },
        plugins: { legend: { labels: { boxWidth: 12 } }, tooltip: { callbacks: { title: items => { const r = xs[items[0].dataIndex]; return short(r.SHA) + ' ' + r.Subject.slice(0, 70); }, label: ctx => `${ctx.dataset.label}: ${fmtY(ctx.parsed.y)}` } } } },
    });
  }

  async function viewOp(object, op) {
    const v = await get(`/api/op/${object}/${op}` + (opsBranch ? '?branch=' + encodeURIComponent(opsBranch) : ''));
    const m = METRICS[metric];
    app.innerHTML = `<h1><span class="mono">${object} / ${op}</span>${objInfo(object)}</h1>
      <div class="cards">${v.Engines.map((e, i) => {
        const l = v.Latest[e], em = metricFor(m, e), t = (v.Trends[e] || {})[em.key], n = v.Noise[e];
        return `<div class="card"><h3><i class="legend"><i style="background:${COLORS[i]}"></i></i>${e}</h3><div class="big">${unmeasured(l[em.key]) ? '-' : em.fmt(l[em.key].Head)}${em !== m ? ' <span class="ci">time</span>' : ''}</div><div class="sub">${fmtNs(l.Ns.Head)} · ${fmtBytes(l.Bytes.Head)} · ${fmtNum(l.Allocs.Head)} allocs/op · job <a href="#/job/${l.JobID}">#${l.JobID}</a></div>${t && t.N >= 3 ? `<div class="sub">${trendText(t.Pct, t.N)}${n ? ` · noise floor ±${n.toFixed(2)}%` : ''}</div>` : ''}</div>`;
      }).join('')}</div>
      <div class="toolbar">${metricTabs()}${branchSelect(v.Branches, v.Branch)}<span class="muted">head value at every measured commit of ${esc(v.Branch)}, oldest left</span></div>
      <div class="chart h300"><canvas id="hc"></canvas></div>
      <h2>Measured commits of ${esc(v.Branch)}</h2>
      <table><thead><tr><th>Job</th><th>Head</th><th>Subject</th><th>Base</th>${v.Engines.filter(e => v.History.some(row => row.Results[e])).map(e => `<th class="grp">${e} <span class="muted" style="text-transform:none;letter-spacing:0">base → head</span></th>`).join('')}</tr></thead><tbody>
      ${v.History.map(row => `<tr><td><a href="#/job/${row.Job.ID}">#${row.Job.ID}</a></td><td>${commitLink(row.Job.HeadSHA)}</td><td class="desc">${esc(row.Job.HeadDesc).replace(/^[0-9a-f]{7} /, '')}</td><td>${commitLink(row.Job.BaseSHA)}</td>${v.Engines.filter(e => v.History.some(x => x.Results[e])).map(e => {
        const r = row.Results[e];
        if (!r) return '<td class="grp muted">-</td>';
        return `<td class="grp cell">${cellDelta(r, m, row.Job.ID)}</td>`;
      }).join('')}</tr>`).join('')}
      </tbody></table>`;
    bindMetricTabs(() => viewOp(object, op));
    bindBranchSelect(() => viewOp(object, op));
    destroyCharts();
    const pts = v.History.slice().reverse();
    const own = v.Engines.filter(e => pts.some(row => row.Results[e]));
    const datasets = own.map(e => {
      const i = v.Engines.indexOf(e), em = metricFor(m, e);
      return {
        label: e + (em !== m ? ' (time)' : ''), borderColor: COLORS[i], backgroundColor: COLORS[i], pointRadius: 3, borderWidth: 1.5, tension: 0.1, spanGaps: true, yAxisID: em !== m ? 'y2' : 'y',
        data: pts.map(row => row.Results[e] && !unmeasured(row.Results[e][em.key]) ? { x: short(row.Job.HeadSHA), y: row.Results[e][em.key].Head, j: row.Job, fmt: em.fmt } : null).filter(Boolean),
      };
    });
    const second = datasets.some(ds => ds.yAxisID === 'y2');
    chart('hc', {
      type: 'line',
      data: { labels: pts.map(row => short(row.Job.HeadSHA)), datasets },
      options: {
        responsive: true, maintainAspectRatio: false, animation: false, parsing: { xAxisKey: 'x', yAxisKey: 'y' },
        scales: Object.assign({ x: { type: 'category', ticks: { maxRotation: 0, autoSkip: true } }, y: { ticks: { callback: val => m.fmt(val) }, title: { display: true, text: m.unit } } },
          second ? { y2: { position: 'right', grid: { display: false }, ticks: { callback: val => fmtNs(val) }, title: { display: true, text: 'time per op (async)' } } } : {}),
        onClick: (ev, els) => { if (els.length) { const p = datasets[els[0].datasetIndex].data[els[0].index]; if (p && p.j) location.hash = '#/job/' + p.j.ID; } },
        plugins: { legend: { labels: { boxWidth: 12 } }, tooltip: { callbacks: { label: ctx => `${ctx.dataset.label}: ${ctx.raw.fmt(ctx.raw.y)} · ${esc(ctx.raw.j.HeadDesc)} (job #${ctx.raw.j.ID})` } } },
      },
    });
  }

  async function viewCompare(params) {
    const a = params.get('a') || '', b = params.get('b') || '';
    const d = await get(`/api/compare?a=${encodeURIComponent(a)}&b=${encodeURIComponent(b)}`);
    const m = METRICS[metric];
    const key = { ns: ['Ns', 'NsDelta'], cycles: ['Cycles', 'CyclesDelta'], instrs: ['Instrs', 'InstrsDelta'], bytes: ['Bytes', 'BytesDelta'], allocs: ['Allocs', 'AllocsDelta'] }[metric];
    let body = '';
    if (a && b) {
      if (d.AJob && d.BJob) {
        body = `<div class="cards">
          <div class="card"><h3>A</h3><div class="mono">${commitLink(d.A)}</div><div class="sub">job <a href="#/job/${d.AJob.ID}">#${d.AJob.ID}</a> (${d.ASide}) · ${when(d.AJob.Finished)}</div></div>
          <div class="card"><h3>B</h3><div class="mono">${commitLink(d.B)}</div><div class="sub">job <a href="#/job/${d.BJob.ID}">#${d.BJob.ID}</a> (${d.BSide}) · ${when(d.BJob.Finished)}</div></div>
          <div class="card"><h3>Source</h3><div class="big">${d.AJob.ID === d.BJob.ID ? 'same job' : 'different jobs'}</div><div class="sub">${d.AJob.ID === d.BJob.ID ? 'interleaved measurement' : 'measured at different times; the noise floor applies on top'}</div></div></div>
          <div class="toolbar">${metricTabs()}</div>
          <table><thead><tr><th>Engine</th><th>Object</th><th>Operation</th><th class="num">A</th><th class="num">B</th><th class="num">Δ</th></tr></thead><tbody>
          ${d.Rows.map(r => { const dl = r[key[1]]; return `<tr><td>${r.Engine}</td><td>${r.Object}</td><td class="mono"><a href="#/op/${r.Object}/${r.Op}">${r.Op}</a></td><td class="num">${m.fmt(r.A[key[0]])}</td><td class="num">${m.fmt(r.B[key[0]])}</td><td class="num"><span class="badge ${Math.abs(dl) < 1 ? 'same' : dl < 0 ? 'better' : 'worse'}">${pct(dl)}</span></td></tr>`; }).join('')}
          </tbody></table>`;
      } else body = '<p class="err">One of the commits has no finished measurement.</p>';
    }
    app.innerHTML = `<h1>Compare two commits</h1>
      <form id="cmp" class="toolbar"><label>A <input type="text" name="a" value="${esc(a)}" placeholder="sha, branch or tag"></label><label>B <input type="text" name="b" value="${esc(b)}" placeholder="sha, branch or tag"></label><button class="btn" type="submit">Compare</button></form>
      <p class="note">Values come from the newest finished job that measured each commit, as head or base.</p>
      ${body}
      <h2>Measured commits <span class="muted small">click a pair to compare its two sides</span></h2>
      <table><thead><tr><th>Job</th><th>Kind</th><th>Branch</th><th>Head</th><th>Base</th><th>Finished</th><th></th></tr></thead><tbody>
      ${d.Jobs.filter(j => j.State === 'done').map(j => `<tr><td><a href="#/job/${j.ID}">#${j.ID}</a></td><td><span class="chip ${j.Kind}">${j.Kind}</span></td><td class="mono">${esc(j.Branch)}</td><td>${commitLink(j.HeadSHA)} <span class="muted">${esc(j.HeadDesc).replace(/^[0-9a-f]{7} /, '')}</span></td><td>${commitLink(j.BaseSHA)} <span class="muted">${esc(j.BaseRef)}</span></td><td class="muted">${when(j.Finished)}</td><td><a href="#/compare?a=${j.BaseSHA}&b=${j.HeadSHA}">compare</a></td></tr>`).join('')}
      </tbody></table>`;
    document.getElementById('cmp').onsubmit = ev => { ev.preventDefault(); const f = ev.target; location.hash = `#/compare?a=${encodeURIComponent(f.a.value.trim())}&b=${encodeURIComponent(f.b.value.trim())}`; };
    bindMetricTabs(() => viewCompare(params));
  }

  async function viewNoise() {
    const d = await get('/api/noise');
    const m = METRICS[metric];
    const st = r => r[m.key];
    let sortKey = 'leaf';
    const render = () => {
      const rows = d.Rows.slice();
      if (sortKey === 'p95') rows.sort((a, b) => st(b).P95Abs - st(a).P95Abs);
      else if (sortKey === 'cv') rows.sort((a, b) => st(b).CV - st(a).CV);
      else rows.sort(sortLeaf);
      return `<table><thead><tr><th>Runner</th><th>Engine</th><th>Object</th><th>Operation</th><th class="num">n</th><th class="num">median |Δ|</th><th class="num sortable" data-k="p95">p95 |Δ| ▾</th><th class="num">max |Δ|</th><th class="num sortable" data-k="cv">median CV ▾</th><th class="num">steal</th></tr></thead><tbody>
        ${rows.map(r => { const s = st(r); return `<tr><td class="mono muted">${esc(r.Runner)}</td><td>${r.Engine}</td><td>${r.Object}</td><td class="mono"><a href="#/op/${r.Object}/${r.Op}">${r.Op}</a></td><td class="num">${r.N}</td><td class="num">${s.MedianAbs.toFixed(2)}%</td><td class="num"><span class="badge ${s.P95Abs < 1 ? 'better' : s.P95Abs < 3 ? 'same' : 'worse'}">${s.P95Abs.toFixed(2)}%</span></td><td class="num">${s.MaxAbs.toFixed(2)}%</td><td class="num">${s.CV.toFixed(2)}%</td><td class="num">${r.Steal}</td></tr>`; }).join('')}
        </tbody></table>`;
    };
    const draw = () => {
      app.innerHTML = `<h1>Noise floor</h1>
        <div class="cards">
          <div class="card"><h3>Self-comparison jobs</h3><div class="big">${d.Jobs}</div><div class="sub">master vs master, every 6 hours and in the idle rotation</div></div>
          ${Object.entries(d.PerRunner || {}).map(([name, st]) => `<div class="card"><h3>|Δ time| on ${esc(name)}</h3><div class="big">${st.MedianAbs.toFixed(2)}% <span class="muted">median</span> · ${st.P95Abs.toFixed(2)}% <span class="muted">p95</span></div><div class="sub">over all operations and noise jobs of this machine</div></div>`).join('')}
          <div class="card"><h3>Steal ticks kept</h3><div class="big">${d.Steal}</div><div class="sub">hypervisor steal during kept runs (runs with steal are repeated once)</div></div>
        </div>
        <div class="toolbar">${metricTabs()}<span class="muted">per operation: |Δ| between the two sides of identical binaries; green p95 below 1%, red above 3%</span></div>
        <div id="ntable">${render()}</div>
        <h2>Noise jobs</h2>${jobRows(d.JobList || [])}`;
      bindMetricTabs(draw);
      app.querySelectorAll('th.sortable').forEach(th => th.onclick = () => { sortKey = sortKey === th.dataset.k ? 'leaf' : th.dataset.k; document.getElementById('ntable').innerHTML = render(); app.querySelectorAll('th.sortable').forEach(t => t.onclick = th.onclick); });
    };
    draw();
  }

  /* ---------- routing ---------- */
  let refreshTimer = null;
  function scheduleRefresh(ms) { clearTimeout(refreshTimer); refreshTimer = setTimeout(() => route(true), ms); }
  async function route(refresh) {
    clearTimeout(refreshTimer);
    refreshing = refresh === true;
    const hash = location.hash || '#/';
    const [path, query] = hash.slice(1).split('?');
    const params = new URLSearchParams(query || '');
    const parts = path.split('/').filter(Boolean);
    document.querySelectorAll('header nav a').forEach(a => a.classList.toggle('active', a.dataset.nav === (parts[0] || 'dash') || ((parts[0] === 'job' || parts[0] === 'pr') && a.dataset.nav === 'jobs') || (parts[0] === 'op' && a.dataset.nav === 'ops')));
    try {
      if (parts.length === 0) { await viewDash(); scheduleRefresh(30000); }
      else if (parts[0] === 'jobs') await viewJobs(params.get('kind'));
      else if (parts[0] === 'job' && parts[2] === 'leaf') await viewLeaf(parts[1], parts[3], parts[4], parts.slice(5).join('/'));
      else if (parts[0] === 'job') await viewJob(parts[1]);
      else if (parts[0] === 'ops') await viewOps();
      else if (parts[0] === 'op') await viewOp(parts[1], parts.slice(2).join('/'));
      else if (parts[0] === 'compare') await viewCompare(params);
      else if (parts[0] === 'pr') await viewPR(parts[1]);
      else if (parts[0] === 'noise') await viewNoise();
      else app.innerHTML = '<p class="muted">not found</p>';
    } catch (e) {
      app.innerHTML = `<p class="err">${esc(e.message)}</p>`;
    }
    if (!refreshing) window.scrollTo(0, 0);
    refreshing = false;
    refreshLive();
  }
  window.addEventListener('hashchange', route);
  setInterval(refreshLive, 15000);
  route();
})();
