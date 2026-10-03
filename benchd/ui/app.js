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
  const metricFor = (m, engine) => engine.endsWith('Async') && (m.key === 'Cycles' || m.key === 'Instrs') ? METRICS.ns : m;
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
  const when = t => t ? new Date(t).toISOString().slice(0, 16).replace('T', ' ') : '-';
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
  const sig = (m, key) => Math.abs(m.Delta) > floorOf(key) && Math.sign(m.MedDelta) === Math.sign(m.Delta);
  function cls(m, key) {
    if (!sig(m, key)) return 'same';
    const strong = Math.abs(m.Delta) >= 5 ? ' strong' : '';
    return (m.Delta < 0 ? 'better' : 'worse') + strong;
  }
  const badge = (m, key) => `<span class="badge ${cls(m, key)}" title="noise floor of this operation ${floorOf(key).toFixed(2)}% · median Δ ${pct(m.MedDelta)} · 95% interval [${pct(m.Lo, 2)}, ${pct(m.Hi, 2)}], p ${m.P.toFixed(3)}">${pct(m.Delta)}</span>`;
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
    m = metricFor(m, r.Engine);
    if (label && m.key === 'Ns') label += ' (time)';
    const M = r[m.key];
    if (unmeasured(M)) return `<div class="d">${label ? `<span class="muted small">${label}</span>` : ''}<span class="muted">not measured</span></div>`;
    const link = jobID ? `#/job/${jobID}/leaf/${r.Engine}/${r.Object}/${r.Op}` : null;
    const body = `<div class="d">${label ? `<span class="muted small">${label}</span>` : ''}<span class="v">${m.fmt(M.Base)}<span class="arrow">→</span><b>${m.fmt(M.Head)}</b></span>${badge(M, r.Engine + '/' + r.Object + '/' + r.Op)}</div>`;
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
      m = metricFor(mAll, b);
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
    return [Math.min(M.DMin, M.Delta), Math.max(M.DMax, M.Delta)];
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
          const r = ds.meta[i];
          if (!r) return;
          const M = r[ds.mkey || key], [lo, hi] = range(M);
          const h = Math.max(4, bar.height);
          const xm = x.getPixelForValue(M.Delta);
          ctx.fillStyle = getComputedStyle(document.body).getPropertyValue('--fg').trim() || '#fff';
          ctx.fillRect(xm - 1, bar.y - h / 2, 2, h);
          const right = M.Delta >= 0;
          ctx.textAlign = right ? 'left' : 'right';
          ctx.fillStyle = ds.borderColor;
          ctx.fillText((M.Delta > 0 ? '+' : '') + M.Delta.toFixed(2) + '%', x.getPixelForValue(right ? Math.max(hi, 0) : Math.min(lo, 0)) + (right ? 5 : -5), bar.y);
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
    const labels = mx.ops;
    const datasets = [];
    const all = [...mx.engines, ...mx.asyncEngines];
    all.forEach((e, i) => {
      const color = COLORS[i % COLORS.length];
      const em = metricFor(m, e);
      const cells = labels.map(op => { const r = mx.cells[e + '/' + op]; return r && !unmeasured(r[em.key]) ? r : null; });
      datasets.push({
        label: e + (em !== m ? ' (time)' : ''), type: 'bar', backgroundColor: color + 'aa', borderColor: color, borderWidth: 1, mkey: em.key,
        data: cells.map(r => r ? range(r[em.key]) : null),
        meta: cells,
        barPercentage: 0.8, categoryPercentage: 0.8, minBarLength: 3,
      });
    });
    const floors = labels.map(op => Math.max(...all.map(e => mx.cells[e + '/' + op] ? floorOf(e + '/' + mx.obj + '/' + op) : 0)));
    const raw = Math.min(100, Math.max(0.5, ...datasets.flatMap(d => d.data.filter(v => v !== null).flat().map(Math.abs))) * 1.4);
    const lim = raw < 2 ? Math.ceil(raw * 4) / 4 : raw < 10 ? Math.ceil(raw) : Math.ceil(raw / 5) * 5;
    return {
      data: { labels, datasets },
      plugins: [noiseBand, barLabels],
      options: {
        indexAxis: 'y', responsive: true, maintainAspectRatio: false, animation: false,
        onClick: (ev, els, c) => { if (els.length && jobID) location.hash = `#/job/${jobID}/leaf/${all[els[0].datasetIndex]}/${mx.obj}/${labels[els[0].index]}`; },
        onHover: (ev, els) => { ev.native.target.style.cursor = els.length ? 'pointer' : 'default'; },
        scales: {
          x: { min: -lim, max: lim, ticks: { callback: v => (v > 0 ? '+' : '') + (Math.round(v * 100) / 100) + '%', maxTicksLimit: 9 }, grid: { color: ctx => ctx.tick.value === 0 ? getComputedStyle(document.body).getPropertyValue('--fg') : getComputedStyle(document.body).getPropertyValue('--line2') } },
          y: { grid: { display: false } },
        },
        plugins: {
          legend: { labels: { boxWidth: 12, generateLabels: c => {
            const items = Chart.defaults.plugins.legend.labels.generateLabels(c);
            items.push({ text: 'grey band: noise floor of the operation (±p95 of master-vs-master runs)', fillStyle: getComputedStyle(document.body).getPropertyValue('--line2').trim(), strokeStyle: getComputedStyle(document.body).getPropertyValue('--muted').trim(), lineWidth: 1, hidden: false, datasetIndex: -1 });
            return items;
          } }, onClick: (e, item, legend) => { if (item.datasetIndex >= 0) Chart.defaults.plugins.legend.onClick.call(legend, e, item, legend); } },
          noiseBand: { floors },
          barLabels: { key: m.key },
          tooltip: { callbacks: { label: ctx => {
            const c = ctx.dataset.meta[ctx.dataIndex], M = c[ctx.dataset.mkey || m.key], [lo, hi] = range(M);
            return `${ctx.dataset.label}: mean ${pct(M.Delta)} · single runs ${pct(lo, 2)} to ${pct(hi, 2)} · ${m.fmt(M.Base)} → ${m.fmt(M.Head)} · ${c.N} runs per side`;
          } } },
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
          const r = ds.meta[i];
          if (!r) return;
          const M = r[opts.key], h = Math.max(4, bar.height);
          let right = x.getPixelForValue(headRange(M)[1]);
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
  const absEngines = (mx, m) => [...mx.engines, ...mx.asyncEngines, ...(chartLibs ? mx.baselines : [])].filter(e => metricFor(m, e) === m);
  // absRowBars is the largest number of bars one operation of the object
  // shows; every row gets room for that many bars of the same thickness.
  const absRowBars = (mx, m) => Math.max(1, ...mx.ops.map(op => absEngines(mx, m).filter(e => { const r = mx.cells[e + '/' + op]; return r && !unmeasured(r[m.key]) && r[m.key].Head > 0; }).length));
  function absChartCfg(mx, m, jobID) {
    const labels = mx.ops;
    const all = absEngines(mx, m);
    const datasets = all.map((e, i) => {
      const color = COLORS[i % COLORS.length], ref = mx.baselines.includes(e);
      const cells = labels.map(op => { const r = mx.cells[e + '/' + op]; return r && !unmeasured(r[m.key]) && r[m.key].Head > 0 ? r : null; });
      return {
        label: e + (ref ? ' (library)' : ''), type: 'bar', backgroundColor: color + (ref ? '55' : 'aa'), borderColor: color, borderWidth: 1,
        data: cells.map(r => r ? headRange(r[m.key]) : null),
        meta: cells, engine: e, own: !ref, skipNull: true,
        barThickness: 9, minBarLength: 3,
      };
    });
    const vals = datasets.flatMap(d => d.meta.filter(Boolean).flatMap(r => [...headRange(r[m.key]), d.own && r[m.key].Base > 0 ? r[m.key].Base : r[m.key].Head]));
    return {
      data: { labels, datasets },
      plugins: [absLabels],
      options: {
        indexAxis: 'y', responsive: true, maintainAspectRatio: false, animation: false,
        onClick: (ev, els) => { if (els.length && jobID && datasets[els[0].datasetIndex].own) location.hash = `#/job/${jobID}/leaf/${datasets[els[0].datasetIndex].engine}/${mx.obj}/${labels[els[0].index]}`; },
        scales: {
          x: { type: 'logarithmic', min: vals.length ? Math.min(...vals) / 1.6 : undefined, max: vals.length ? Math.max(...vals) * 2.5 : undefined, ticks: { callback: v => m.fmt(v), maxTicksLimit: 10 }, title: { display: true, text: `${m.label} ${m.unit}, logarithmic` } },
          y: { grid: { display: false } },
        },
        plugins: {
          legend: { labels: { boxWidth: 12 } },
          absLabels: { fmt: m.fmt, key: m.key },
          tooltip: { callbacks: { label: ctx => {
            const c = ctx.dataset.meta[ctx.dataIndex], M = c[m.key], [lo, hi] = headRange(M);
            return `${ctx.dataset.label}: mean ${m.fmt(M.Head)} · single runs ${m.fmt(lo)} to ${m.fmt(hi)}${ctx.dataset.own ? ` · base ${m.fmt(M.Base)} (${pct(M.Delta)})` : ''} · ${c.N} runs`;
          } } },
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
        <div class="card"><h3>Ratio per engine <span class="muted" style="text-transform:none">(cycles when counted, else time)</span></h3><div class="chips">${engineTotals(d.Summaries || []).map(x => `<span class="chip" title="geomean of head/base ${x.Cycles ? 'cycles' : 'time'} over ${x.N} operations of every object">${x.Engine} <b>${pct(x.Geomean, 1)}</b></span>`).join('') || '<span class="muted">-</span>'}</div><details class="small" style="margin-top:6px"><summary>per object</summary><div class="chips" style="margin-top:4px">${(d.Summaries || []).map(x => `<span class="chip">${x.Engine}·${x.Object} <b>${pct(x.Geomean, 1)}</b></span>`).join('')}</div></details></div>
      </div>
      ${j.Note ? `<p class="note">${esc(j.Note)}</p>` : ''}${d.BaselineJob ? `<p class="note">Other libraries: values from the reference job <a href="#/job/${d.BaselineJob}">#${d.BaselineJob}</a> on the same machine and payload. They cannot hash Gloas objects unless they implement progressive merkleization (PrysmSSZ does), and none but PrysmSSZ can express the Gloas state.</p>` : ''}${j.Error ? `<pre class="err">${esc(j.Error)}</pre>` : ''}
      <div class="toolbar">${metricTabs()}<div class="tabs" id="modeTabs"><button data-mode="rel" class="${chartMode === 'rel' ? 'active' : ''}" title="charts show head against base in percent">change in %</button><button data-mode="abs" class="${chartMode === 'abs' ? 'active' : ''}" title="charts show the measured values">measured values</button></div>${chartMode === 'abs' ? `<label class="check"><input type="checkbox" id="chartLibs" ${chartLibs ? 'checked' : ''}> with the other libraries</label>` : ''}<span class="muted">${m.label} ${m.unit}: base → head and Δ per engine and operation. Green/red: |Δ| exceeds this operation's noise floor (p95 of the self-comparisons, at least 0.5%); bold: |Δ| ≥ 5%; grey: within noise. Click a bar or cell for every sample.</span></div>
      ${mxs.map((mx, i) => { const bars = chartMode === 'abs' ? absRowBars(mx, m) : [...mx.engines, ...mx.asyncEngines].length; return `<h2>${objName(mx.obj)}</h2><div class="chart" style="height:${mx.ops.length * Math.max(30, bars * 14 + 8) + 80}px"><canvas id="dc${i}"></canvas></div><p class="note">${chartMode === 'abs' ? `One bar per engine${chartLibs ? ' and reference library' : ''} per operation, in measured ${m.label.toLowerCase()}: the bar spans the values of the single runs, the white tick and the number are their mean, the grey tick on our own engines is the base's mean. The axis is logarithmic because the operations of one object span several orders of magnitude, so a bar is short when the runs agree.${chartLibs ? ' Paler bars are the other libraries, from the latest reference job.' : ''}` : `One bar per engine (colours in the legend) per operation: the bar spans the ${m.label.toLowerCase()} deltas of the single runs (each run compares head and base measured minutes apart), the white tick and the number are the mean over the runs. A long bar means the runs disagreed. The grey band behind each row is that operation's noise floor on this machine: how far two measurements of the same code differ (p95 over the master-vs-master noise jobs, at least 0.5%). A result inside the band is not a change. Async engines appear only on HashTreeRoot.`}</p>${matrixTable(mx, m, j.ID)}`; }).join('') || '<p class="muted">no results yet</p>'}
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
    const samples = d.Samples;
    const stat = (M, f) => r.Baseline
      ? `<div class="big">${f(M.Head)}</div><div class="sub">cv ${M.CVHead.toFixed(2)}%</div>`
      : `<div class="big">${badge(M)}</div><div class="sub">${f(M.Base)} → ${f(M.Head)} · 95% [${pct(M.Lo, 1)}, ${pct(M.Hi, 1)}] · p ${M.P.toFixed(3)} · median Δ ${pct(M.MedDelta)} · cv ${M.CVBase.toFixed(2)}% / ${M.CVHead.toFixed(2)}%</div>`;
    app.innerHTML = `<h1><span class="mono">${engine} / ${object} / ${op}</span> <span class="muted small">job <a href="#/job/${id}">#${id}</a>${d.Runs.length > 1 ? `, pooled over ${d.Runs.length} runs` : ''}</span></h1>
      <div class="cards">
        <div class="card"><h3>Time per op</h3>${stat(r.Ns, fmtNs)}</div>
        ${r.Cycles.Head ? `<div class="card"><h3>Cycles per op</h3>${stat(r.Cycles, fmtNum)}</div><div class="card"><h3>Instructions per op</h3>${stat(r.Instrs, fmtNum)}</div>` : ''}
        <div class="card"><h3>Memory per op</h3>${stat(r.Bytes, fmtBytes)}</div>
        <div class="card"><h3>Allocations per op</h3>${stat(r.Allocs, fmtNum)}</div>
        <div class="card"><h3>Measurement</h3><div class="big">${r.N} × ${r.Iters}</div><div class="sub">measurements per side × iterations each${r.Steal ? ` · ${r.Steal} steal ticks (wake-ups)` : ''}</div></div>
      </div>
      <div class="toolbar">${metricTabs()}<span class="muted">every sample, in measurement order (pass, seed, side); dashed lines are the side means</span></div>
      <div class="chart h300"><canvas id="sc"></canvas></div>
      <table><thead><tr><th>Run</th><th>Side</th><th class="num">Pass</th><th>Seed</th><th class="num">Iterations</th><th class="num">Time</th><th class="num">Cycles</th><th class="num">Instrs</th><th class="num">Memory</th><th class="num">Allocs</th><th class="num">Steal</th></tr></thead><tbody>
      ${samples.map(s => `<tr><td><a href="#/job/${s.JobID}">#${s.JobID}</a></td><td>${s.Side}</td><td class="num">${s.Pass + 1}</td><td class="mono">${s.Seed}</td><td class="num">${s.Iters}</td><td class="num">${fmtNs(s.Ns)}</td><td class="num">${s.Cycles ? fmtNum(s.Cycles) : '-'}</td><td class="num">${s.Instrs ? fmtNum(s.Instrs) : '-'}</td><td class="num">${fmtBytes(s.Bytes)}</td><td class="num">${fmtNum(s.Allocs)}</td><td class="num">${s.Steal}</td></tr>`).join('')}
      </tbody></table>
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
  async function viewOps() {
    const rows = await get('/api/ops');
    const m = METRICS[metric];
    const all = sortEngines([...new Set(rows.flatMap(r => r.Engines))]);
    const engines = all.filter(e => !opsHidden.has(e));
    const cell = (r, e) => {
      const l = r.Latest[e], t = r.Trends[e];
      if (!l) return '<td class="grp muted">-</td>';
      const em = metricFor(m, e);
      if (unmeasured(l[em.key])) return '<td class="grp muted">-</td>';
      return `<td class="grp"><b>${em.fmt(l[em.key].Head)}</b>${em !== m ? ' <span class="ci">time</span>' : ''}${t && t.N >= 3 ? ` <span class="ci">trend ${pct(t.SlopePct30d, 1)}/30d</span>` : ''}</td>`;
    };
    app.innerHTML = `<h1>Operations</h1>
      <div class="toolbar">${metricTabs()}<span class="muted">latest head value per engine and library; master trend over 30 days once three master points exist</span></div>
      <div class="toolbar chips" id="engsel">${all.map(e => `<a href="#" data-e="${e}" class="chip ${opsHidden.has(e) ? '' : 'commit'}" title="show or hide this column">${e}</a>`).join(' ')}<a href="#" data-e="*" class="chip">all</a><a href="#" data-e="-" class="chip">ours only</a></div>
      <div style="overflow-x:auto"><table><thead><tr><th>Object</th><th>Operation</th>${engines.map(e => `<th class="grp">${e}</th>`).join('')}</tr></thead><tbody>
      ${rows.map((r, i) => `<tr><td>${i > 0 && rows[i - 1].Object === r.Object ? `<span class="muted">${r.Object}</span>` : objName(r.Object)}</td><td class="mono"><a href="#/op/${r.Object}/${r.Op}">${r.Op}</a></td>${engines.map(e => cell(r, e)).join('')}</tr>`).join('')}
      </tbody></table></div>`;
    bindMetricTabs(viewOps);
    document.querySelectorAll('#engsel a').forEach(a => a.onclick = (ev => {
      ev.preventDefault();
      const e = a.dataset.e;
      if (e === '*') opsHidden = new Set();
      else if (e === '-') opsHidden = new Set(all.filter(x => !['Codegen', 'Reflection', 'CodegenAsync', 'ReflectionAsync'].includes(x)));
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
      <table><thead><tr><th>Commit</th><th>Subject</th><th>Committed</th><th>Job</th><th>Against the base</th><th>Against the previous measured commit</th></tr></thead><tbody>
      ${d.Rows.map(r => { const j = r.Job; const prev = r.Previous ? d.Rows.find(x => x.Job && x.Job.ID === r.Previous) : null; return `<tr class="${j ? '' : 'unmeasured'}"><td>${commitLink(r.SHA)}${r.Detached ? ' <span class="chip" title="measured earlier; a force push took this commit out of the branch">earlier head</span>' : ''}</td>
        <td class="desc">${esc(r.Subject)}</td><td class="muted">${when(r.Time)}</td>
        <td>${j ? `<a href="#/job/${j.ID}">#${j.ID}</a> ${chip(j)}` : '<span class="muted">not measured</span>'}</td>
        <td><div class="chips">${j && j.State === 'done' ? chips(r.Summaries) : j ? `<span class="muted">${esc(j.Note || j.State)}</span>` : ''}</div></td>
        <td><div class="chips">${prev ? chips(r.Step) + ` <a href="#/compare?a=${prev.SHA}&b=${r.SHA}">compare</a>` : ''}</div></td></tr>`; }).join('')}
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
      return prMode === 'abs' ? M.Head : M.Delta;
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
    const v = await get(`/api/op/${object}/${op}`);
    const m = METRICS[metric];
    const main = (await get('/api/status')).MainBranch;
    app.innerHTML = `<h1><span class="mono">${object} / ${op}</span>${objInfo(object)}</h1>
      <div class="cards">${v.Engines.map((e, i) => {
        const l = v.Latest[e], t = v.Trends[e], n = v.Noise[e];
        return `<div class="card"><h3><i class="legend"><i style="background:${COLORS[i]}"></i></i>${e}</h3><div class="big">${fmtNs(l.Ns.Head)}</div><div class="sub">${fmtBytes(l.Bytes.Head)} · ${fmtNum(l.Allocs.Head)} allocs/op · latest head (job <a href="#/job/${l.JobID}">#${l.JobID}</a>)</div>${t && t.N >= 3 ? `<div class="sub">master trend ${pct(t.SlopePct30d, 1)} / 30d over ${t.N} points (R² ${t.R2.toFixed(2)}), projected ${fmtNs(t.Projected30d)}</div>` : ''}${n ? `<div class="sub">noise floor p95 |Δ| ${n.toFixed(2)}%</div>` : ''}</div>`;
      }).join('')}</div>
      <div class="toolbar">${metricTabs()}<span class="muted">master history: head value of every ${esc(main)} job, newest right</span></div>
      <div class="chart h300"><canvas id="hc"></canvas></div>
      <h2>Every job</h2>
      <table><thead><tr><th>Job</th><th>Kind</th><th>Branch</th><th>Head</th><th>Base</th>${v.Engines.map(e => `<th class="grp">${e}</th>`).join('')}</tr></thead><tbody>
      ${v.History.map(row => `<tr><td><a href="#/job/${row.Job.ID}">#${row.Job.ID}</a></td><td><span class="chip ${row.Job.Kind}">${row.Job.Kind}</span></td><td class="mono">${esc(row.Job.Branch)}</td><td>${commitLink(row.Job.HeadSHA)}</td><td>${commitLink(row.Job.BaseSHA)}</td>${v.Engines.map(e => {
        const r = row.Results[e];
        if (!r) return '<td class="grp muted">-</td>';
        return `<td class="grp cell">${r.Baseline ? cellAbs(r, m) : cellDelta(r, m, row.Job.ID)}</td>`;
      }).join('')}</tr>`).join('')}
      </tbody></table>`;
    bindMetricTabs(() => viewOp(object, op));
    destroyCharts();
    const pts = v.History.slice().reverse().filter(row => row.Job.Branch === main && row.Job.Kind !== 'noise' && row.Job.Finished);
    const datasets = v.Engines.map((e, i) => ({
      label: e, borderColor: COLORS[i], backgroundColor: COLORS[i], pointRadius: 3, borderWidth: 1.5, tension: 0.1, spanGaps: true,
      data: pts.map(row => row.Results[e] ? { x: when(row.Job.Finished), y: row.Results[e][m.key].Head, j: row.Job } : null).filter(Boolean),
    }));
    chart('hc', {
      type: 'line',
      data: { labels: pts.map(row => when(row.Job.Finished)), datasets },
      options: {
        responsive: true, maintainAspectRatio: false, animation: false, parsing: { xAxisKey: 'x', yAxisKey: 'y' },
        scales: { x: { type: 'category', ticks: { maxRotation: 0, autoSkip: true } }, y: { ticks: { callback: val => m.fmt(val) }, title: { display: true, text: m.unit } } },
        onClick: (ev, els) => { if (els.length) { const d = datasets[els[0].datasetIndex].data[els[0].index]; if (d && d.j) location.hash = '#/job/' + d.j.ID; } },
        plugins: { legend: { labels: { boxWidth: 12 } }, tooltip: { callbacks: { label: ctx => `${ctx.dataset.label}: ${m.fmt(ctx.raw.y)} · ${short(ctx.raw.j.HeadSHA)} (job #${ctx.raw.j.ID})` } } },
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
