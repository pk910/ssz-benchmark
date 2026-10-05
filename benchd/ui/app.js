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
      // Every table gets a scrolling frame, except the small fact tables
      // inside cards.
      html = html.replace(/<table(?! class="facts")/g, '<div class="tw"><table').replace(/<\/table>/g, '</table></div>').replace(/(<table class="facts"[\s\S]*?<\/table>)<\/div>/g, '$1');
      if (!refreshing) { appEl.innerHTML = html; return; }
      const t = document.createElement('div');
      t.innerHTML = html;
      morph(appEl, t);
    },
    querySelectorAll: sel => appEl.querySelectorAll(sel),
  };
  const liveEl = document.getElementById('live');
  const OBJECTS = ['FuluState', 'FuluBlock', 'FuluBlocks', 'FuluMinState', 'FuluMinBlock', 'GloasState', 'GloasBlock', 'GloasBlocks', 'GloasEnvelope', 'GloasMinState', 'GloasMinBlock'];
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
    FuluMinState: 'FuluState cut to the minimal preset, the same types with the minimal spec values: every eighth validator with its balance, participation and inactivity score (296,693 validators), the vectors at their minimal lengths (64 block roots, state roots, randao mixes and slashings), 32 eth1 votes, sync committees of 32, the pending partial withdrawals and consolidations capped at 64. Every spec value the types depend on differs from its compiled default here, which the mainnet objects never exercise. Three operations are measured on it: Unmarshal, Marshal and HashTreeRoot.',
    FuluMinBlock: 'FuluBlock cut to the minimal preset: aggregation bits and attester slashing indices at the limit of a slot\'s committees (8,192), committee bits of 4, sync committee bits of 32, 4 withdrawals; everything else as in FuluBlock. Measured with the minimal spec values on Unmarshal, Marshal and HashTreeRoot.',
    GloasMinState: 'FuluMinState converted to the Gloas layout, with the Gloas fields at their minimal sizes: a PTC window of 24 slots of 16, 16 builder pending payments, 64 slots of payload availability. Measured with the minimal spec values on Unmarshal, Marshal and HashTreeRoot.',
    GloasMinBlock: 'FuluMinBlock converted to the Gloas layout, with payload attestations of 16 bits. Measured with the minimal spec values on Unmarshal, Marshal and HashTreeRoot.',
  };
  const objInfo = o => OBJECT_INFO[o] ? `<span class="info" tabindex="0" data-tip="${OBJECT_INFO[o].replace(/"/g, '&quot;')}">i</span>` : '';
  const objName = o => `${o}${objInfo(o)}`;
  // How an engine is named on the pages. The engines of dynamic-ssz carry
  // the library's name like the others do.
  const ENGINE_LABELS = { Codegen: 'dynamic-ssz codegen', Reflection: 'dynamic-ssz reflection', EthereumSSZ: 'ethereum_ssz (Rust)', Grandine: 'grandine ssz (Rust)', Teku: 'teku ssz (Java)', LodestarValue: 'lodestar ssz value (TS)', LodestarTree: 'lodestar ssz tree (TS)', Nimbus: 'nim-ssz (Nim)', SszPP: 'sszpp (C++)' };
  // An engine named *Async is its parent engine hashing with background
  // workers. The harness runs it as an engine of its own; the pages show
  // it as an operation of the parent, "HashTreeRoot (async)", so an engine
  // name on a page is always the parent's.
  const isAsync = e => /Async$/.test(e);
  const baseEngine = e => e.replace(/Async$/, '');
  const opLabel = (e, op) => isAsync(e) ? op + ' (async)' : op;
  const opRank = op => rank(OPS, op.replace(/ \(async\)$/, '')) * 2 + (/\(async\)$/.test(op) ? 1 : 0);
  const engName = e => ENGINE_LABELS[baseEngine(e)] || baseEngine(e);
  const ENGINES = ['Codegen', 'Reflection', 'CodegenAsync', 'ReflectionAsync', 'FastSSZ', 'FastSSZv1', 'FastSSZv2', 'PrysmSSZ', 'KaralabeSSZ', 'KaralabeSSZAsync', 'EthereumSSZ', 'Grandine', 'Teku', 'LodestarValue', 'LodestarTree', 'Nimbus', 'SszPP'];
  const COLORS = ['#2f5fd1', '#d97706', '#7c3aed', '#0f9d8a', '#6b7280', '#db2777'];
  const METRICS = {
    ns: { key: 'Ns', label: 'Time', unit: 'per op', fmt: fmtNs },
    cycles: { key: 'Cycles', label: 'Cycles', unit: 'cycles/op', fmt: fmtNum },
    instrs: { key: 'Instrs', label: 'Instructions', unit: 'instrs/op', fmt: fmtNum },
    bytes: { key: 'Bytes', label: 'Memory', unit: 'B/op', fmt: fmtBytes },
    allocs: { key: 'Allocs', label: 'Allocations', unit: 'allocs/op', fmt: fmtNum },
    // The further figures of a run (its Extra values). They exist on the
    // job page and the page of one operation of a job only: each is
    // measured in some of the passes, so it has a median per side and no
    // per-pass statistics.
    brmiss: { key: 'XBrMiss', extra: 'br-miss', label: 'Branch misses', unit: 'branch misses/op', fmt: fmtNum },
    festall: { key: 'XFeStall', extra: 'fe-stall', label: 'Frontend stalls', unit: 'stalled frontend cycles/op', fmt: fmtNum },
    l1d: { key: 'XL1d', extra: 'l1d-miss', label: 'L1d misses', unit: 'L1 data misses/op', fmt: fmtNum },
    l2: { key: 'XL2', extra: 'l2-miss', label: 'L2 misses', unit: 'L2 data misses/op', fmt: fmtNum },
    retained: { key: 'XRetained', extra: 'retained', label: 'Kept alive', unit: 'B the result keeps alive', fmt: fmtBytes },
    stack: { key: 'XStack', extra: 'stack', label: 'Stack', unit: 'B of stack beyond 32 KiB', fmt: fmtBytes },
    faults: { key: 'XFaults', extra: 'faults', label: 'Page faults', unit: 'page faults/op of the measured thread', fmt: fmtNum },
    sysns: { key: 'XSysNs', extra: 'sys-ns', label: 'Kernel time', unit: 'kernel time per op of the measured thread', fmt: fmtNs },
  };
  // curMetric is the metric of the active tab; a page without the further
  // figures shows cycles while one of them is selected.
  const curMetric = extras => { const cur = METRICS[localStorage.getItem('metric') || 'cycles']; return cur && (extras || !cur.extra) ? cur : METRICS.cycles; };
  // withExtras gives a result a metric for each further figure: the
  // medians of the two sides and their difference.
  function withExtras(r) {
    Object.values(METRICS).filter(m => m.extra).forEach(m => {
      const x = r.Extra && r.Extra[m.extra];
      r[m.key] = { Base: x ? x.Base || 0 : 0, Head: x ? x.Head : 0, Delta: x && x.Base > 0 ? (x.Head - x.Base) / x.Base * 100 : 0, PN: 0, PMed: 0, PAgree: 0, PSpread: 0, DMin: 0, DMax: 0, CVBase: 0, CVHead: 0, Runs: x ? x.N : 0, Loose: true };
    });
    return r;
  }
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
  const allThreads = r => !!(r && (r.Threads > 0 || (r.Extra && r.Extra.threads)));
  const metricFor = (m, engine, r) => engine.endsWith('Async') && (m.key === 'Cycles' || m.key === 'Instrs') && !allThreads(r) ? METRICS.ns : m;
  let repo = '';
  let subjectRepos = {}; // library -> repository URL, from the status
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
  // A further figure (Loose) has no per-pass statistics and repeats within
  // a few percent: it counts as changed from 10%.
  const sig = (m, key) => m.Loose ? Math.abs(delta(m)) >= 10 : Math.abs(delta(m)) > bandOf(m, key) && (!(m.PN > 0) || m.PAgree >= Math.ceil(0.75 * m.PN));
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
    faults: { name: 'page faults', short: 'faults', kernel: true },
    'sys-ns': { name: 'kernel time', short: 'sys', kernel: true, fmt: fmtNs },
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
    const secs = rows[0].head.BuildSeconds, based = rows.some(x => x.base);
    const cell = (x, f) => `<td class="num">${sizeDelta(x.head[f], x.base ? x.base[f] : 0)}</td>`;
    return `<div class="card"><h3>Build${based ? ' <span class="muted" style="text-transform:none">(head, change against the base)</span>' : ''}</h3>
      <table class="facts"><thead><tr><th>Package</th><th class="num" title="size of the compiled code (text section) of the benchmark binary">Code</th><th class="num" title="size of the benchmark binary">Binary</th><th class="num" title="size of the generated SSZ source">Generated</th></tr></thead><tbody>
      ${rows.map(x => `<tr><td class="mono">${esc(x.pkg.replace(/^.*-/, ''))}</td>${cell(x, 'TextBytes')}${cell(x, 'BinBytes')}${cell(x, 'GenBytes')}</tr>`).join('')}
      </tbody></table>${secs > 0 ? `<div class="sub">generated and compiled in ${dur(secs)}</div>` : ''}</div>`;
  }
  const ciText = m => `<span class="ci">[${pct(m.Lo, 1)}, ${pct(m.Hi, 1)}]</span>`;
  // kindsOf names what a job measures. A job of another library measures
  // a commit of its main branch, a release, or both when they coincide.
  const kindsOf = j => j.Kind !== 'lib' && !(j.Targets || '').includes('+') ? [j.Kind] : [...new Set((j.Targets || j.Branch || '').split('+').map(t => t === 'master' ? 'commit' : 'release'))];
  const kindChips = j => kindsOf(j).map(k => `<span class="chip ${k}">${k}</span>`).join(' ');
  // An idle rerun of a commit or pair that was measured before is marked:
  // it is no new work, its runs are pooled with the earlier ones.
  const isRefinement = j => /refinement run/.test(j.Note || '');
  const chip = j => `<span class="chip ${j.State}">${j.State}</span> ${kindChips(j)}${isRefinement(j) ? ' <span class="chip refine" title="idle time: the same commits measured again under other layouts; the runs are pooled with the earlier ones">refinement</span>' : ''}`;
  // Every link to GitHub stands behind the GitHub mark (ghLink); a plain
  // link always leads to a page here.
  const ghLink = (url, title) => `<a class="gh" href="${url}" target="_blank" rel="noopener" title="${esc(title)}">${GH_MARK}</a>`;
  // repoLink names the repository of a library, linked to its page here,
  // with the mark leading to GitHub.
  const repoLink = subject => { const sub = subject || 'dynamic-ssz', url = subjectRepos[sub] || repo; return `<a href="#/repo/${encodeURIComponent(sub)}" title="the commits and pull requests measured of this library">${esc(url ? url.replace('https://github.com/', '') : sub)}</a>${url ? ghLink(url, 'the repository on GitHub') : ''}`; };
  // prRef names a pull request of the mirrored library, linked to its page
  // here (every measured head), with the mark leading to GitHub.
  const prRef = (n, label) => `<a href="#/pr/${n}" title="every measured head of this pull request">${label || '#' + n}</a>${repo ? ghLink(`${repo}/pull/${n}`, 'the pull request on GitHub') : ''}`;
  // refLabel is the branch or tag a job's commit was found under.
  const refLabel = j => esc(j.Branch) + prLink(j);
  // commitLink links a commit to its page here, with a small mark that
  // leads to the commit in the repository of its library (ours when none
  // is named).
  const GH_MARK = '<svg viewBox="0 0 16 16" width="11" height="11" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"/></svg>';
  const commitLink = (sha, subject) => {
    if (!sha) return '<span class="muted">-</span>';
    if (sha === 'baselines') return '<span class="muted">reference libraries</span>';
    const sub = subject || 'dynamic-ssz', url = subjectRepos[sub] || repo;
    return `<a class="mono" href="#/commit/${sub}/${sha}" title="everything measured of this commit">${short(sha)}</a>${url ? ghLink(`${url}/commit/${sha}`, 'this commit on GitHub') : ''}`;
  };
  const prLink = j => j.PR ? ` · ${prRef(j.PR, 'PR #' + j.PR)}` : '';
  const sortLeaf = (a, b) => rank(OBJECTS, a.Object) - rank(OBJECTS, b.Object) || a.Object.localeCompare(b.Object)
    || rank(OPS, a.Op) - rank(OPS, b.Op) || a.Op.localeCompare(b.Op) || rank(ENGINES, a.Engine) - rank(ENGINES, b.Engine);
  const sortEngines = es => es.sort((a, b) => rank(ENGINES, a) - rank(ENGINES, b) || a.localeCompare(b));

  // foldSummaries folds the per-engine-and-object geomeans of a job by a
  // key, over cycles when the job has cycle counts, else over time. The
  // async hashing of an engine folds into the engine, by time (its cycles
  // are those of all its threads).
  function foldSummaries(sums, keyOf) {
    const acc = {};
    const useCycles = (sums || []).some(s => s.CyclesGeomean);
    (sums || []).forEach(s => {
      const k = keyOf(s), a = acc[k] || (acc[k] = { Engine: baseEngine(s.Engine), Object: s.Object, logSum: 0, N: 0 });
      a.logSum += Math.log(1 + (useCycles && !isAsync(s.Engine) ? s.CyclesGeomean : s.Geomean) / 100) * s.N;
      a.N += s.N;
    });
    return Object.values(acc).sort((a, b) => rank(ENGINES, a.Engine) - rank(ENGINES, b.Engine) || rank(OBJECTS, a.Object) - rank(OBJECTS, b.Object))
      .map(a => ({ Engine: a.Engine, Object: a.Object, N: a.N, Geomean: (Math.exp(a.logSum / a.N) - 1) * 100, Cycles: useCycles }));
  }
  // engineTotals is one geomean per engine, objectTotals one per engine
  // and object.
  const engineTotals = sums => foldSummaries(sums, s => baseEngine(s.Engine));
  const objectTotals = sums => foldSummaries(sums, s => baseEngine(s.Engine) + '/' + s.Object);

  async function get(url) {
    const r = await fetch(url);
    if (!r.ok) throw new Error(`${url}: ${r.status}`);
    return r.json();
  }
  // callout: one floating box for the hover breakdowns of a page, filled
  // when a marked element is hovered and placed next to it.
  let calloutEl = null;
  function showCallout(html, at) {
    if (!calloutEl) { calloutEl = document.createElement('div'); calloutEl.className = 'callout'; document.body.appendChild(calloutEl); }
    calloutEl.innerHTML = html;
    calloutEl.style.display = 'block';
    const r = at.getBoundingClientRect(), w = calloutEl.offsetWidth, h = calloutEl.offsetHeight;
    let left = Math.max(8, Math.min(r.left, innerWidth - w - 8)), top = r.bottom + 6;
    if (top + h > innerHeight - 8) top = Math.max(8, r.top - h - 6);
    calloutEl.style.left = left + 'px';
    calloutEl.style.top = top + 'px';
  }
  function hideCallout() { if (calloutEl) calloutEl.style.display = 'none'; }
  // bindCallouts shows a callout for the elements of root matching sel,
  // with the markup that html(el) gives.
  function bindCallouts(root, sel, html) {
    if (!root) return;
    root.addEventListener('mouseover', ev => { const el = ev.target.closest(sel); if (el && root.contains(el)) showCallout(html(el), el); });
    root.addEventListener('mouseout', ev => { const el = ev.target.closest(sel); if (el && !el.contains(ev.relatedTarget)) hideCallout(); });
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
  function metricTabs(extras) {
    const cur = curMetric(extras);
    const tab = ([k, m]) => `<button data-m="${k}" class="${m === cur ? 'active' : ''}" title="${m.unit}">${m.label}</button>`;
    const all = Object.entries(METRICS);
    return `<div class="tabs" id="metricTabs">${all.filter(([, m]) => !m.extra).map(tab).join('')}</div>${extras ? `<div class="tabs" id="metricTabsMore" title="measured in some of the passes: the median per side">${all.filter(([, m]) => m.extra).map(tab).join('')}</div>` : ''}`;
  }
  function bindMetricTabs(rerender) {
    document.querySelectorAll('#metricTabs button, #metricTabsMore button').forEach(b => b.onclick = () => { metric = b.dataset.m; localStorage.setItem('metric', metric); rerender(); });
  }

  /* ---------- live header ---------- */
  async function refreshLive() {
    try {
      const s = await get('/api/status');
      repo = s.Repo || repo;
      subjectRepos = s.Subjects || subjectRepos;
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
    // The runner is named only when the jobs ran on more than one machine.
    const runners = new Set(jobs.map(row => (row.Job || row).Runner).filter(Boolean)).size > 1;
    return `<table><thead><tr><th>#</th><th>State</th><th>Repository</th><th>Branch / PR</th><th>Head</th><th>Base</th>${runners ? '<th>Runner</th>' : ''}${opts.summaries ? '<th>Ratio per engine <span class="muted" style="text-transform:none">(cycles, else time)</span></th>' : ''}<th class="num">Passes</th><th class="num">Took</th><th>${opts.queue ? 'Queued' : 'Finished'}</th></tr></thead><tbody>` +
      jobs.map(row => {
        const j = row.Job || row;
        const sums = row.Summaries ? `<td><div class="chips">${engineTotals(row.Summaries).map(s => `<span class="chip" title="geomean of head/base ${s.Cycles ? 'cycles' : 'time'} over ${s.N} operations of every object">${engName(s.Engine).replace(/^dynamic-ssz /, '')} <b>${pct(s.Geomean, 1)}</b></span>`).join('')}</div></td>` : (opts.summaries ? '<td></td>' : '');
        const fin = j.State === 'queued' ? `${j.Priority ? 'priority ' + j.Priority + ' · ' : ''}${ago(j.Created)}` : j.State === 'running' ? `since ${when(j.Started)}` : when(j.Finished);
        return `<tr><td><a href="#/job/${j.ID}">${j.ID}</a></td><td>${chip(j)}</td><td>${repoLink(j.Subject)}</td><td class="mono">${refLabel(j)}</td>` +
          `<td>${commitLink(j.HeadSHA, j.Subject)} <span class="muted desc" style="display:inline-block;vertical-align:bottom">${esc(j.HeadDesc).replace(/^[0-9a-f]{7,12} /, '')}</span></td>` +
          `<td>${commitLink(j.BaseSHA, j.Subject)} <span class="muted">${esc(j.BaseRef)}</span></td>${runners ? `<td class="mono muted">${esc(j.Runner || '')}</td>` : ''}${sums}<td class="num">${j.Passes || ''}</td><td class="num">${j.Seconds ? dur(j.Seconds) : ''}</td>` +
          `<td class="muted">${fin}${j.Error ? ` <span class="err" title="${esc(j.Error)}">error</span>` : ''}</td></tr>`;
      }).join('') + '</tbody></table>';
  }

  // The overview has two tabs: the state of the machine with the newest
  // jobs, and every job.
  const overviewTabs = active => `<h1>Overview</h1><div class="toolbar"><div class="tabs">${[['#/', 'Overview'], ['#/jobs', 'All jobs']].map(([href, label]) => `<a href="${href}" class="${label === active ? 'active' : ''}">${label}</a>`).join('')}</div></div>`;

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
    app.innerHTML = `${overviewTabs('Overview')}
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
    const rows = await get('/api/jobs?limit=500&summaries=1');
    const all = rows.map(r => r.Job);
    const params = new URLSearchParams(location.hash.split('?')[1] || '');
    const repoSel = params.get('repo') || '', sha = params.get('sha') || '';
    const link = (k, r) => `#/jobs${k || r ? '?' + [k ? 'kind=' + k : '', r ? 'repo=' + encodeURIComponent(r) : ''].filter(Boolean).join('&') : ''}`;
    const kinds = ['', 'commit', 'release', 'noise'].map(k => `<a href="${link(k, repoSel)}" class="chip ${k === (kind || '') ? 'commit' : ''}">${k || 'all kinds'}</a>`).join(' ');
    const subjects = [...new Set(all.map(j => j.Subject))].sort((a, b) => (a !== 'dynamic-ssz') - (b !== 'dynamic-ssz') || a.localeCompare(b));
    const repos = ['', ...subjects].map(r => `<a href="${link(kind, r)}" class="chip ${r === repoSel ? 'commit' : ''}">${r ? esc((subjectRepos[r] || r).replace('https://github.com/', '')) + (subjects.filter(x => subjectRepos[x] === subjectRepos[r]).length > 1 ? ` (${esc(r)})` : '') : 'all repositories'}</a>`).join(' ');
    const jobs = rows.filter(r => { const j = r.Job; return (!kind || kindsOf(j).includes(kind)) && (!repoSel || j.Subject === repoSel) && (!sha || j.HeadSHA === sha || j.BaseSHA === sha); });
    app.innerHTML = `${overviewTabs('All jobs')}<div class="toolbar">${kinds}</div><div class="toolbar">${repos}</div>${sha ? `<p class="note">Jobs that measured commit <span class="mono">${esc(sha.slice(0, 12))}</span> as head or base. <a href="#/jobs">all jobs</a></p>` : ''}${jobRows(jobs, { summaries: true })}`;
  }

  /* ---------- repositories ---------- */
  // stepChips: how a commit changed each engine against the commit before
  // it on the branch.
  const stepChips = (c, v, what) => {
    if (!c.State) return '<span class="muted">not measured</span>';
    if (c.State !== 'done') return `<span class="muted">${esc(c.State)}</span>`;
    if (!c.Compared) return `<span class="muted">${c.Against ? `no values comparable with ${what || 'the measured commit before'}` : what ? `${what} is not measured` : 'first measured commit'}</span>`;
    const skipped = c.Skipped ? ` <span class="muted small" title="the ${c.Skipped} commit${c.Skipped === 1 ? '' : 's'} before it ${c.Skipped === 1 ? 'was' : 'were'} not measured: compared with the measured commit before those">against ${short(c.Against).slice(0, 8)}, ${c.Skipped} unmeasured between</span>` : '';
    return `<div class="chips">${c.Engines.map(e => `<span class="chip ${Math.abs(e.Geomean) < 0.5 ? '' : e.Geomean < 0 ? 'better' : 'worse'}" title="against the measured commit before: geomean over ${e.N} operations (cycles, else time); ${e.Faster} faster and ${e.Slower} slower by 1% or more">${esc(engName(e.Engine).replace(/^dynamic-ssz /, ''))} <b>${pct(e.Geomean, 1)}</b>${e.Faster || e.Slower ? ` <span class="moved">${e.Faster ? `<span class="better">▼${e.Faster}</span>` : ''}${e.Slower ? `<span class="worse">▲${e.Slower}</span>` : ''}</span>` : ''}</span>`).join('')}${skipped}</div>`;
  };
  // commitAge: when a commit was made, relative while it is younger than
  // 30 days, as a date after that.
  const commitAge = t => {
    if (!t) return '';
    const d = new Date(t * 1000), days = (Date.now() - d) / 86400000;
    const text = days < 30 ? ago(d) : `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    return `<span title="committed ${when(d)}">${text}</span>`;
  };
  const tagBadges = c => (c.Tags || []).map(t => ` <span class="chip tag">${esc(t)}</span>`).join('');
  // commitDesc: the subject of a commit; its pull request, when it has
  // one, links to the pull request's page here (the mirrored library) and
  // to GitHub behind the mark.
  const commitDesc = (v, c) => {
    const desc = esc(c.Desc).replace(/^[0-9a-f]{7,12} /, '');
    if (!c.PR) return `<span class="muted desc" style="display:inline-block;vertical-align:bottom">${desc}</span>`;
    const ref = v.Repo && v.Repo === repo ? prRef(c.PR) : `#${c.PR}${v.Repo ? ghLink(`${v.Repo}/pull/${c.PR}`, 'the pull request on GitHub') : ''}`;
    const text = desc.endsWith(`(#${c.PR})`) ? `${desc.slice(0, -(`(#${c.PR})`.length))}(${ref})` : `${desc} (${ref})`;
    return `<span class="muted desc" style="display:inline-block;vertical-align:bottom">${text}</span>`;
  };
  const repoCommits = (v, commits) => !commits.length ? `<p class="muted">${v.Branch ? 'no commit of the main branch known yet' : 'a fixed version: no branch is followed'}</p>`
    : `<table class="commits"><thead><tr><th>Commit</th><th>Date</th><th>Description</th><th>Change against the measured commit before <span class="muted" style="text-transform:none">(per engine)</span></th><th class="num">Runs</th><th>Measured</th></tr></thead><tbody>${commits.map(c =>
      `<tr class="${c.State ? '' : 'unmeasured'}"><td class="nowrap">${commitLink(c.SHA, v.Name)}${tagBadges(c)}</td><td class="muted nowrap">${commitAge(c.Committed)}</td><td>${commitDesc(v, c)}</td><td>${stepChips(c, v)}</td><td class="num">${c.Runs || ''}</td><td class="muted">${c.Jobs ? `${c.Measured ? when(c.Measured * 1000) : esc(c.State)} · <a href="#/jobs?repo=${encodeURIComponent(v.Name)}&sha=${c.SHA}">${c.Jobs} job${c.Jobs === 1 ? '' : 's'}</a>` : ''}</td></tr>`).join('')}</tbody></table>`;
  const repoTargets = v => (v.Targets || []).map(t => `<span class="chip">${esc(t.Name)} <b>${esc(t.Label)}</b></span> ${commitLink(t.SHA, v.Name)}`).join(' &nbsp; ');
  const repoTitle = v => `${esc(v.Name)} <span class="muted small">${repoLink(v.Name)}</span>`;
  // repoFacts: what the page knows of the library in one line.
  const repoFacts = v => [v.Branch ? `${v.Total} commit${v.Total === 1 ? '' : 's'} of ${esc(v.Branch)}, ${v.Measured} measured` : '', v.PullRequests && v.PullRequests.length ? `<a href="#/repo/${encodeURIComponent(v.Name)}">${v.PullRequests.length} open pull request${v.PullRequests.length === 1 ? '' : 's'}</a>` : ''].filter(Boolean).join(' · ');

  async function viewRepos() {
    const repos = await get('/api/repos');
    app.innerHTML = `<h1>Repositories</h1><p class="note">Every measured library in a box of its own: its targets, the newest commits of its main branch, each compared with the one before it over everything measured of both, and its open pull requests.</p>` +
      repos.map(v => `<section class="repobox"><h2><a href="#/repo/${encodeURIComponent(v.Name)}">${esc(v.Name)}</a> <span class="muted small">${repoLink(v.Name)}</span></h2>
        <div class="toolbar">${repoTargets(v)}<span class="muted">${repoFacts(v)}</span></div>${repoCommits(v, v.Commits)}
        <p class="note"><a href="#/repo/${encodeURIComponent(v.Name)}">${v.Total > v.Commits.length ? 'all commits' : 'repository page'}${v.PullRequests && v.PullRequests.length ? ' and pull requests' : ''} →</a></p></section>`).join('');
  }

  // The change column of a library's page is a grid: one column per
  // operation of the page, so that the values line up across the engines
  // and the commits. The header names the operations in groups; a chip
  // holds the value only, the breakdown by payload type is its callout.
  const OP_GROUPS = [['Unmarshal', /^Unmarshal/], ['SizeSSZ', /^SizeSSZ/], ['Marshal', /^Marshal/], ['HashTreeRoot', /^HashTreeRoot/], ['GetTree', /^GetTree/]];
  const OP_VARIANT = { Unmarshal: 'bytes', UnmarshalReader: 'reader', UnmarshalReaderUnknown: 'unknown', SizeSSZ: '', Marshal: 'bytes', MarshalTo: 'to buf', MarshalWriter: 'writer', HashTreeRoot: 'sync', 'HashTreeRoot (async)': 'async', GetTree: '' };
  const opGroup = op => (OP_GROUPS.find(g => g[1].test(op)) || [op])[0];
  const opVariant = op => OP_VARIANT[op] !== undefined ? OP_VARIANT[op] : op;
  const gridStyle = n => `style="--n:${n}"`;
  function opGridHead(cols) {
    const groups = [];
    cols.forEach(op => { const g = opGroup(op); if (groups.length && groups[groups.length - 1].name === g) groups[groups.length - 1].n++; else groups.push({ name: g, n: 1 }); });
    return `<div class="opgrid head" ${gridStyle(cols.length)}><span></span>${groups.map(g => `<span class="grp" style="grid-column:span ${g.n}">${esc(g.name)}</span>`).join('')}<span></span>${cols.map(op => `<span class="var">${esc(opVariant(op))}</span>`).join('')}</div>`;
  }
  // stepsOf: per engine and operation of a commit, the ratio to the
  // measured commit before per payload type (cycles, else time; time for
  // the async hashing), with the commit's own value.
  function stepsOf(v, c) {
    const acc = {};
    if (!c.Steps) return acc;
    v.Leaves.forEach((l, k) => {
      const st = c.Steps[k], ns = isAsync(l.Engine) || !(st && st[1] > 0);
      const r = st ? (ns ? st[0] : st[1]) : 0;
      if (!(r > 0)) return;
      const e = baseEngine(l.Engine), op = opLabel(l.Engine, l.Op), val = c.Values && c.Values[k] ? c.Values[k][ns ? 0 : 1] : 0;
      ((acc[e] = acc[e] || {})[op] = acc[e][op] || []).push({ obj: l.Object, r, ns, val });
    });
    return acc;
  }
  const geomean = xs => (Math.exp(xs.reduce((t, x) => t + Math.log(x.r), 0) / xs.length) - 1) * 100;
  const stepClass = d => Math.abs(d) < 0.5 ? 'flat' : d < 0 ? 'better' : 'worse';
  const stepPct = d => `<span class="${stepClass(d)}">${pct(d, 1)}</span>`;
  // opGrid: the rows of one commit, one per engine, in operation precision.
  function opGrid(v, c, i, cols, hidden, src, what) {
    const steps = stepsOf(v, c), engines = sortEngines(Object.keys(steps).filter(e => !hidden.has(e)));
    if (!engines.length) return stepChips(c, v, what);
    const skipped = c.Skipped ? `<div class="muted small">against ${short(c.Against).slice(0, 8)}, ${c.Skipped} unmeasured between</div>` : '';
    return engines.map(e => `<div class="opgrid" ${gridStyle(cols.length)}><span class="eng" title="${esc(engName(e))}">${esc(engName(e).replace(/^dynamic-ssz /, ''))}</span>${cols.map(op => {
      const xs = steps[e][op];
      if (!xs) return '<span></span>';
      const d = geomean(xs);
      return `<span class="chip ${stepClass(d)}" data-ci="${i}" data-src="${src || 'c'}" data-e="${e}" data-op="${esc(op)}"><b>${pct(d, 1)}</b></span>`;
    }).join('')}</div>`).join('') + skipped;
  }
  // engineRows: the rows of one commit in engine precision: one chip per
  // engine with the count of operations that moved.
  function engineRows(v, c, i, hidden, src, what) {
    const engines = (c.Engines || []).filter(e => !hidden.has(e.Engine));
    if (!c.Compared || !engines.length) return stepChips(Object.assign({}, c, { Engines: engines }), v, what);
    const skipped = c.Skipped ? `<div class="muted small">against ${short(c.Against).slice(0, 8)}, ${c.Skipped} unmeasured between</div>` : '';
    return engines.map(e => `<div class="oprow"><span class="eng" title="${esc(engName(e.Engine))}">${esc(engName(e.Engine).replace(/^dynamic-ssz /, ''))}</span><span class="chip ${stepClass(e.Geomean)}" data-ci="${i}" data-src="${src || 'c'}" data-e="${e.Engine}"><b>${pct(e.Geomean, 1)}</b></span><span class="muted small">${e.N} operations${e.Faster || e.Slower ? `, <span class="better">▼${e.Faster}</span> <span class="worse">▲${e.Slower}</span> by 1% or more` : ''}</span></div>`).join('') + skipped;
  }
  // stepCallout: the breakdown behind a chip: per payload type for an
  // operation, per operation for an engine.
  function stepCallout(v, c, e, op, what) {
    const steps = stepsOf(v, c), against = `against ${what ? what + ' ' : ''}${short(c.Against).slice(0, 8)}${c.Skipped ? `, ${c.Skipped} unmeasured between` : ''}`;
    const ops = steps[e] || {};
    if (op) {
      const xs = ops[op] || [];
      const fmt = x => x.ns ? fmtNs : fmtNum;
      return `<div><b>${esc(engName(e))} · ${esc(op)}</b> <span class="muted">${against}</span></div>
        <table class="facts"><thead><tr><th>payload type</th><th class="num">before</th><th class="num">now</th><th class="num">change</th></tr></thead><tbody>
        ${xs.map(x => `<tr><td>${esc(x.obj)}</td><td class="num">${x.val ? fmt(x)(x.val / x.r) : '-'}</td><td class="num">${x.val ? fmt(x)(x.val) : '-'}${x.ns && !isAsync(e + (op.endsWith('(async)') ? 'Async' : '')) ? ' <span class="muted">time</span>' : ''}</td><td class="num">${stepPct((x.r - 1) * 100)}</td></tr>`).join('')}
        <tr><td><b>geomean</b></td><td></td><td></td><td class="num"><b>${stepPct(geomean(xs))}</b></td></tr></tbody></table>
        <div class="muted small">cycles per call; time for the async hashing and where no cycles were counted</div>`;
    }
    const names = Object.keys(ops).sort((a, b) => opRank(a) - opRank(b));
    const all = names.flatMap(n => ops[n]);
    return `<div><b>${esc(engName(e))}</b> <span class="muted">${against}</span></div>
      <table class="facts"><thead><tr><th>operation</th><th class="num">types</th><th class="num">change</th></tr></thead><tbody>
      ${names.map(n => `<tr><td>${esc(n)}</td><td class="num">${ops[n].length}</td><td class="num">${stepPct(geomean(ops[n]))}</td></tr>`).join('')}
      <tr><td><b>geomean</b></td><td class="num">${all.length}</td><td class="num"><b>${stepPct(geomean(all))}</b></td></tr></tbody></table>
      <div class="muted small">geomean over the payload types of each operation; hover a value in operation precision for the types</div>`;
  }

  async function viewRepo(name, page) {
    page = Math.max(1, parseInt(page, 10) || 1);
    const v = await get(`/api/repo/${encodeURIComponent(name)}?page=${page}`);
    if (v.Error) throw new Error(v.Error);
    const pages = Math.max(1, Math.ceil(v.Total / v.PageSize));
    const pageHref = p => `#/repo/${encodeURIComponent(name)}${p > 1 ? '?page=' + p : ''}`;
    const pager = pages < 2 ? '' : `<div class="toolbar pager">${page > 1 ? `<a class="chip" href="${pageHref(page - 1)}">← newer</a>` : ''}${Array.from({ length: pages }, (_, i) => i + 1).filter(p => p === 1 || p === pages || Math.abs(p - page) <= 2).map((p, i, shown) => `${i && p - shown[i - 1] > 1 ? '<span class="muted">…</span>' : ''}<a class="chip ${p === page ? 'commit' : ''}" href="${pageHref(p)}">${p}</a>`).join('')}${page < pages ? `<a class="chip" href="${pageHref(page + 1)}">older →</a>` : ''}<span class="muted">commits ${(page - 1) * v.PageSize + 1}–${(page - 1) * v.PageSize + v.Commits.length} of ${v.Total}</span></div>`;
    // The measured commits of the page, oldest first, for the charts.
    const chain = v.Commits.filter(c => c.Values).reverse();
    const engines = sortEngines([...new Set((v.Leaves || []).map(l => baseEngine(l.Engine)))]);
    const pref = (key, def, allowed) => { const x = localStorage.getItem(key); return !allowed || allowed.includes(x) ? (x || def) : def; };
    const engine = pref('repoEngine', engines[0], engines);
    const useNs = pref('repoMetric', 'ns', ['ns', 'cycles']) === 'ns';
    const agg = pref('repoAgg', 'geomean', ['geomean', 'sum']);
    const precision = pref('repoPrecision', 'engines', ['engines', 'ops']);
    let hidden = new Set((localStorage.getItem('repoHidden') || '').split(',').filter(e => engines.includes(e)));
    if (hidden.size >= engines.length) hidden = new Set();
    const tabs = (id, items, cur) => `<div class="tabs" id="${id}">${items.map(([k, label]) => `<button data-k="${k}" class="${k === cur ? 'active' : ''}">${esc(label)}</button>`).join('')}</div>`;
    // The operations of the page, the columns of the change grid.
    const cols = [...new Set((v.Leaves || []).map(l => opLabel(l.Engine, l.Op)))].sort((a, b) => opRank(a) - opRank(b));
    const changeHead = against => precision === 'ops'
      ? `<div>Change against ${against} <span class="muted" style="text-transform:none">(per engine and operation, geomean over the payload types; hover a value for the types)</span></div>${opGridHead(cols)}`
      : `Change against ${against} <span class="muted" style="text-transform:none">(per engine, geomean over its operations; hover a value for the operations)</span>`;
    const change = (c, i, src, what) => precision === 'ops' && c.Compared ? opGrid(v, c, i, cols, hidden, src, what) : engineRows(v, c, i, hidden, src, what);
    const measured = c => c.Jobs ? `${c.Measured ? when(c.Measured * 1000) : esc(c.State)} · <a href="#/jobs?repo=${encodeURIComponent(v.Name)}&sha=${c.SHA}">${c.Jobs} job${c.Jobs === 1 ? '' : 's'}</a>` : '';
    const rows = commits => !commits.length ? repoCommits(v, commits)
      : `<table class="commits"><thead><tr><th>Commit</th><th>Date</th><th>Description</th><th>${changeHead('the measured commit before')}</th><th class="num">Runs</th><th>Measured</th></tr></thead><tbody>${commits.map((c, i) => {
        return `<tr class="${c.State ? '' : 'unmeasured'}"><td class="nowrap">${commitLink(c.SHA, v.Name)}${tagBadges(c)}</td><td class="muted nowrap">${commitAge(c.Committed)}</td><td>${commitDesc(v, c)}</td><td>${change(c, i, 'c')}</td><td class="num">${c.Runs || ''}</td><td class="muted nowrap">${measured(c)}</td></tr>`;
      }).join('')}</tbody></table>`;
    // The open pull requests, each head against the head of the main
    // branch: the pooled values of both, within one harness version.
    const headOf = `the head of ${esc(v.Branch || 'the main branch')}`;
    const prs = v.PullRequests || [];
    const prRows = () => `<table class="commits"><thead><tr><th>Pull request</th><th>Branch</th><th>Head</th><th>Title</th><th>${changeHead(headOf)}</th><th class="num">Runs</th><th>Measured</th></tr></thead><tbody>${prs.map((c, i) =>
      `<tr class="${c.State ? '' : 'unmeasured'}"><td class="nowrap">${prRef(c.Number)}</td><td class="mono nowrap">${esc(c.Branch)}${c.Fork ? ' <span class="chip" title="a fork: its head is measured once a maintainer approves it">fork</span>' : ''}</td><td class="nowrap">${commitLink(c.SHA, v.Name)}</td><td><span class="muted desc" style="display:inline-block;vertical-align:bottom">${esc(c.Desc)}</span></td><td>${change(c, i, 'p', headOf)}</td><td class="num">${c.Runs || ''}</td><td class="muted nowrap">${measured(c)}</td></tr>`).join('')}</tbody></table>`;
    const prSection = prs.length ? `<h2 id="prs">Open pull requests <span class="muted small">newest first; a head against ${headOf}</span></h2>${prRows()}` : '';
    const charted = engines.length && chain.length > 1;
    app.innerHTML = `<h1>${repoTitle(v)} <a class="agent" href="/repo/${encodeURIComponent(name)}.md${page > 1 ? '?page=' + page : ''}" title="this page as text, for an agent">text</a></h1>
      <div class="toolbar">${repoTargets(v)}<span class="muted">${v.Total} commit${v.Total === 1 ? '' : 's'} of ${esc(v.Branch || 'the main branch')}, ${v.Measured} measured</span></div>
      ${charted ? `<div class="toolbar">${tabs('repoEngine', engines.map(e => [e, engName(e)]), engine)}${tabs('repoMetric', [['ns', 'Time'], ['cycles', 'Cycles']], useNs ? 'ns' : 'cycles')}${tabs('repoAgg', [['geomean', 'Mean over payload types'], ['sum', 'Sum']], agg)}</div>
      <p class="note">One line per operation: its ${useNs ? 'time' : 'cycles'} per call at every measured commit, ${agg === 'sum' ? 'summed over the payload types (the largest type dominates)' : 'as the geometric mean over the payload types (every type counts alike)'}. Operations of one kind (unmarshalling, marshalling, hashing) share a chart and its scale.</p>
      <div id="repoCharts"></div>` : ''}
      ${engines.length ? `<div class="toolbar"><span class="muted">detail</span>${tabs('repoPrecision', [['engines', 'Engines'], ['ops', 'Operations']], precision)}<span class="muted">show</span><span id="repoHidden">${engines.map(e => `<a class="chip toggle ${hidden.has(e) ? 'off' : 'commit'}" data-e="${e}" title="click to ${hidden.has(e) ? 'show' : 'hide'}">${esc(engName(e))}</a>`).join(' ')}</span></div>` : ''}
      ${prSection}
      <h2>Commits <span class="muted small">newest first</span></h2>
      ${pager}${rows(v.Commits)}${pager}`;
    const again = () => viewRepo(name, page);
    ['repoEngine', 'repoMetric', 'repoAgg', 'repoPrecision'].forEach(id => document.querySelectorAll(`#${id} button`).forEach(b => b.onclick = () => { localStorage.setItem(id, b.dataset.k); again(); }));
    document.querySelectorAll('#repoHidden a').forEach(a => a.onclick = () => { hidden.has(a.dataset.e) ? hidden.delete(a.dataset.e) : hidden.add(a.dataset.e); localStorage.setItem('repoHidden', [...hidden].join(',')); again(); });
    document.querySelectorAll('table.commits').forEach(t => bindCallouts(t, '.chip[data-ci]', el => el.dataset.src === 'p' ? stepCallout(v, prs[+el.dataset.ci], el.dataset.e, el.dataset.op, headOf) : stepCallout(v, v.Commits[+el.dataset.ci], el.dataset.e, el.dataset.op)));
    destroyCharts();
    if (!charted) return;
    // Per operation the payload types every measured commit has a value
    // of, so that a line is the same set of types throughout.
    const col = useNs ? 0 : 1, series = [];
    const addSeries = (eng, suffix) => [...new Set(v.Leaves.filter(l => l.Engine === eng).map(l => l.Op))].sort((a, b) => rank(OPS, a) - rank(OPS, b)).forEach(op => {
      const ks = v.Leaves.map((l, k) => l.Engine === eng && l.Op === op ? k : -1).filter(k => k >= 0 && chain.every(c => c.Values[k] && c.Values[k][col] > 0));
      if (!ks.length) return;
      const data = chain.map(c => agg === 'sum' ? ks.reduce((t, k) => t + c.Values[k][col], 0) : Math.exp(ks.reduce((t, k) => t + Math.log(c.Values[k][col]), 0) / ks.length));
      series.push({ op: op + suffix, n: ks.length, data });
    });
    addSeries(engine, '');
    // The async hashing of the engine is a line of its own next to the
    // operation it runs. Its cycles are those of all its threads.
    if (v.Leaves.some(l => l.Engine === engine + 'Async')) addSeries(engine + 'Async', useNs ? ' (async)' : ' (async, all threads)');
    // The operations of one kind share a chart: unmarshalling,
    // marshalling, hashing; any other stands alone.
    const family = op => op.startsWith('Unmarshal') ? 'Unmarshal' : op.startsWith('Marshal') ? 'Marshal' : /^(HashTreeRoot|GetTree)/.test(op) ? 'Hash' : op;
    const groups = [];
    series.forEach(sr => { const g = groups.find(g => family(g[0].op) === family(sr.op)); if (g) g.push(sr); else groups.push([sr]); });
    const fmt = useNs ? fmtNs : fmtNum;
    document.getElementById('repoCharts').innerHTML = groups.map((g, i) => `<div class="chart" style="height:${200 + 14 * g.length}px"><canvas id="rc${i}"></canvas></div>`).join('');
    // The charts stand below each other: the axis and the legend take
    // the same width in each, so that the commits line up.
    const sameWidths = { id: 'sameWidths', beforeInit(c) { const fit = c.legend.fit; c.legend.fit = function () { fit.call(this); this.width = 200; }; } };
    groups.forEach((g, i) => chart('rc' + i, {
      type: 'line',
      plugins: [sameWidths],
      data: { labels: chain.map(c => short(c.SHA).slice(0, 8)), datasets: g.map(sr => { const color = COLORS[series.indexOf(sr) % COLORS.length]; return { label: sr.op, n: sr.n, data: sr.data, borderColor: color, backgroundColor: color, pointRadius: 3, pointHoverRadius: 5, borderWidth: 1.5, tension: 0 }; }) },
      options: {
        responsive: true, maintainAspectRatio: false, animation: false,
        interaction: { mode: 'index', intersect: false },
        scales: { x: { ticks: { maxRotation: 0, autoSkip: true, display: i === groups.length - 1 } }, y: { afterFit: axis => { axis.width = 96; }, ticks: { callback: val => fmt(val) }, title: { display: true, text: useNs ? 'time per call' : 'cycles per call' } } },
        onClick: (ev, els) => { if (els.length) location.hash = commitHref(v.Name, chain[els[0].index].SHA); },
        plugins: {
          legend: { position: 'right', align: 'start', labels: { boxWidth: 12 } },
          tooltip: { callbacks: { title: items => { const c = chain[items[0].dataIndex]; return `${short(c.SHA)} ${(c.Desc || '').replace(/^[0-9a-f]{7,12} /, '').slice(0, 70)}`; }, label: ctx => { const prev = ctx.dataIndex > 0 ? ctx.dataset.data[ctx.dataIndex - 1] : 0; return `${ctx.dataset.label}: ${fmt(ctx.raw)}${prev ? ` (${pct((ctx.raw / prev - 1) * 100, 1)} against the commit before)` : ''} · ${ctx.dataset.n} type${ctx.dataset.n === 1 ? '' : 's'}`; } } },
        },
      },
    }));
  }

  /* job page: metric switch, delta charts, matrices */
  function buildMatrices(results) {
    const byObj = {};
    results.forEach(r => (byObj[r.Object] = byObj[r.Object] || []).push(r));
    return Object.keys(byObj).sort((a, b) => rank(OBJECTS, a) - rank(OBJECTS, b) || a.localeCompare(b)).map(obj => {
      const rs = byObj[obj];
      // Async engines measure one operation; they show inside the parent
      // engine's cell instead of a column of their own.
      // A job's own engines take the columns; the values of the other
      // libraries (r.Other) stand beside them. Without a base the job's
      // results are values only.
      const own = rs.filter(r => !r.Other);
      const engines = sortEngines([...new Set(own.filter(r => !isAsync(r.Engine)).map(r => r.Engine))]);
      const asyncEngines = sortEngines([...new Set(own.filter(r => isAsync(r.Engine)).map(r => r.Engine))]);
      const baselines = sortEngines([...new Set(rs.filter(r => r.Other).map(r => r.Engine))]);
      const oneSided = own.length > 0 && own.every(r => r.Baseline);
      const ops = [...new Set(own.map(r => r.Op))].sort((a, b) => rank(OPS, a) - rank(OPS, b) || a.localeCompare(b));
      const cells = {};
      rs.forEach(r => cells[r.Engine + '/' + r.Op] = r);
      // An object only the other libraries measured is one this job's
      // library does not support.
      return { obj, engines, asyncEngines, baselines, ops, cells, oneSided, unsupported: !own.length };
    });
  }

  // A further figure is measured when runs reported it, also at zero.
  const unmeasured = M => !M || (M.Loose ? !M.Runs : M.Base === 0 && M.Head === 0);
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
      const mAll = m;
      m = metricFor(mAll, b, ref);
      if (!ref || unmeasured(ref[m.key])) { m = mAll; return ''; }
      const ours = (isAsync(b) ? mx.asyncEngines : mx.engines).map(e => {
        const r = mx.cells[e + '/' + op];
        if (!r || unmeasured(r[m.key]) || !(ref[m.key].Head > 0)) return '';
        const x = r[m.key].Head / ref[m.key].Head;
        return `<span class="${x < 0.95 ? 'better' : x > 1.05 ? 'worse' : 'muted'}">${engName(e)} ${x.toFixed(2)}×</span>`;
      }).filter(Boolean).join(' ');
      const line = `<div class="ref"><span class="muted">${engName(b)}${isAsync(b) ? ' (async)' : ''}${m !== mAll ? ' (time)' : ''}</span><b>${m.fmt(ref[m.key].Head)}</b><span class="ours">${ours}</span></div>`;
      m = mAll;
      return line;
    }).filter(Boolean).join('');
    return lines || '<span class="muted">-</span>';
  }

  function matrixTable(mx, m, jobID) {
    const head = `<tr><th>Operation</th>${mx.engines.map(e => `<th class="grp">${engName(e)}${mx.oneSided ? '' : ' <span class="muted" style="text-transform:none;letter-spacing:0">base → head</span>'}</th>`).join('')}${mx.baselines.length ? '<th class="grp">Other libraries <span class="muted" style="text-transform:none;letter-spacing:0">their value · ours ÷ theirs</span></th>' : ''}</tr>`;
    const rows = mx.ops.map(op => {
      const tds = mx.engines.map(e => {
        const r = mx.cells[e + '/' + op], ra = mx.cells[e + 'Async/' + op];
        if (mx.oneSided) {
          const one = (x, label) => { const xm = metricFor(m, x.Engine, x), body = `${label ? `<span class="muted small">${label}${xm !== m ? ' (time)' : ''}</span> ` : ''}${cellAbs(x, xm)}`; return jobID ? `<a href="#/job/${jobID}/leaf/${x.Engine}/${x.Object}/${x.Op}">${body}</a>` : body; };
          return `<td class="grp cell">${r ? one(r) : '<span class="muted">-</span>'}${ra ? one(ra, 'async') : ''}</td>`;
        }
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
        title: items => { const c = items[0].dataset.meta[items[0].dataIndex]; return `${engName(c.engine)} · ${items[0].label}`; },
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
        label: engName(e), type: 'bar', backgroundColor: color + 'aa', borderColor: color, borderWidth: 1,
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
        label: engName(e) + (ref ? ' (library)' : ''), type: 'bar', backgroundColor: color + (ref ? '55' : 'aa'), borderColor: color, borderWidth: 1,
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

  // sectionsHTML renders one section per object: its chart and its table.
  // drawSections draws the charts once the page is in place.
  function sectionsHTML(mxs, m, mode, jobID, subject) {
    return mxs.map((mx, i) => { if (mx.unsupported) return `<h2>${objName(mx.obj)}</h2><p class="muted">not supported: ${esc(subject || 'this library')} cannot express this object, so the job has no measurement of it</p>`; const bars = mode === 'abs' ? absRowBars(mx, m) : mx.engines.length, rows = chartRows(mx, m, mode === 'abs').length; return `<h2>${objName(mx.obj)}</h2><div class="chart" style="height:${rows * Math.max(30, bars * 14 + 8) + 80}px"><canvas id="dc${i}"></canvas></div><p class="note">${mode === 'abs' ? `One bar per engine${chartLibs ? ' and reference library' : ''} per operation, in measured ${m.label.toLowerCase()}: the thick body is the middle half of the single runs' values and the thin line reaches to the lowest and the highest run, the white tick and the number are their mean, the grey tick on our own engines is the base's mean. The axis is logarithmic because the operations of one object span several orders of magnitude, so a bar is short when the runs agree.${chartLibs ? ' Paler bars are the other libraries, from the latest reference job.' : ''}` : `One candle per engine (colours in the legend) per operation. The thick body is the middle half of the per-pass ${m.label.toLowerCase()} deltas (each pass links head and base with another function layout and measures them minutes apart), the thin line reaches to the lowest and the highest pass, the white tick and the number are the median. A single pass far off shows as a long thin line and leaves the body and the scale alone; an arrow head means the line continues beyond the chart. The grey band behind each row is what does not count as a change there: the operation's noise floor on this machine, widened where the passes scatter. A result is a change when its median lies outside the band and at least three quarters of the passes agree. The async engines have a row of their own below the operation they run, in the colour of their engine. On the counter tabs that row shows the cycles and instructions of all their threads together (the total work, not the latency), or time for a job measured before all threads were counted.`}</p>${matrixTable(mx, m, jobID)}`; }).join('') || '<p class="muted">no results yet</p>';
  }
  function drawSections(mxs, m, mode, jobID) {
    mxs.forEach((mx, i) => mx.unsupported || chart('dc' + i, (mode === 'abs' ? absChartCfg : deltaChartCfg)(mx, m, jobID)));
  }

  async function viewJob(id) {
    const d = await get('/api/job/' + id);
    repo = d.Repo;
    leafNoise = (d.Noise && d.Noise.PerLeaf) || {};
    const j = d.Job, m = curMetric(true);
    d.Results.forEach(withExtras);
    const mxs = buildMatrices(d.Results);
    // A one-sided job has values only: its charts show them, never deltas.
    const measured = mxs.filter(mx => !mx.unsupported);
    const oneSided = measured.length > 0 && measured.every(mx => mx.oneSided);
    const mode = oneSided ? 'abs' : chartMode;
    const r = d.Runner, p = (r && r.Progress) || {};
    const live = d.Live ? `<div class="card wide"><div class="sub">${r ? esc(r.Runner) + ': ' + esc(r.Phase) : 'running on ' + esc(j.Runner) + ' (no live report yet)'}</div>${r && p.Leaves ? `<div class="bar"><div style="width:${r.Percent}%"></div></div><div class="sub">pass ${p.Pass}${p.PlannedPasses ? ' of ~' + p.PlannedPasses : ''} · leaf ${p.Leaf}/${p.Leaves} · ${dur(p.Measured / 1e9)} measured · running ${dur(r.Running)} · provisional results from the samples so far, refreshes every 20 s</div>` : ''}</div>` : '';
    const runs = '';
    app.innerHTML = `<h1>Job #${j.ID} ${chip(j)}${d.Live ? '<span class="chip running">live</span>' : ''} <a class="agent" href="/job/${j.ID}.md" title="this page as text, for an agent">text</a></h1>
      ${live}
      <div class="cards">
        <div class="card"><h3>Head</h3><div class="mono">${commitLink(j.HeadSHA, j.Subject)} ${esc(j.HeadDesc).replace(/^[0-9a-f]{7} /, '')}</div><div class="sub mono">${esc(j.Branch)}${prLink(j)}</div></div>
        <div class="card"${j.BaseSHA ? '' : ' style="display:none"'}><h3>Base (${esc(j.BaseRef)})</h3><div class="mono">${commitLink(j.BaseSHA)} ${esc(j.BaseDesc).replace(/^[0-9a-f]{7} /, '')}</div>${j.BaseSHA !== j.HeadSHA ? `<div class="sub">diff${ghLink(`${repo}/compare/${j.BaseSHA}...${j.HeadSHA}`, 'the diff on GitHub')}</div>` : ''}</div>
        <div class="card"><h3>Measurement</h3><div class="mono">${j.Passes} passes${j.Seconds ? ', ' + dur(j.Seconds) : ''}${runs}</div><div class="sub">runner ${esc(j.Runner || '-')}</div><div class="sub">${esc(j.GoVersion)} · harness ${j.Harness}${d.Steal ? ` · ${d.Steal} steal ticks (wake-ups, within the limit)` : ' · no steal'}</div><div class="sub">queued ${when(j.Created)} · finished ${when(j.Finished)}</div></div>
        ${buildCard(d.Builds)}
        <div class="card"${oneSided ? ' style="display:none"' : ''}><h3>Ratio per engine <span class="muted" style="text-transform:none">(cycles when counted, else time)</span></h3><div class="chips">${engineTotals(d.Summaries || []).map(x => `<span class="chip" title="geomean of head/base ${x.Cycles ? 'cycles' : 'time'} over ${x.N} operations of every object">${engName(x.Engine)} <b>${pct(x.Geomean, 1)}</b></span>`).join('') || '<span class="muted">-</span>'}</div><details class="small" style="margin-top:6px"><summary>per object</summary><div class="chips" style="margin-top:4px">${objectTotals(d.Summaries || []).map(x => `<span class="chip" title="geomean over ${x.N} operations">${engName(x.Engine)}·${x.Object} <b>${pct(x.Geomean, 1)}</b></span>`).join('')}</div></details></div>
      </div>
      ${j.Note ? `<p class="note">${esc(j.Note)}</p>` : ''}${mxs.some(mx => mx.baselines.length) ? `<p class="note">Other libraries: their pooled values at their latest release, measured by their own jobs on the same machine and payload (see <a href="#/ops">Operations</a>). They cannot hash Gloas objects unless they implement progressive merkleization.</p>` : ''}${j.Error ? `<pre class="err">${esc(j.Error)}</pre>` : ''}
      <div class="toolbar">${metricTabs(true)}<div class="tabs" id="modeTabs"${oneSided ? ' style="display:none"' : ''}><button data-mode="rel" class="${mode === 'rel' ? 'active' : ''}" title="charts show head against base in percent">change in %</button><button data-mode="abs" class="${mode === 'abs' ? 'active' : ''}" title="charts show the measured values">measured values</button></div>${mode === 'abs' ? `<label class="check"><input type="checkbox" id="chartLibs" ${chartLibs ? 'checked' : ''}> with the other libraries</label>` : ''}${oneSided ? `<span class="muted">${m.label} ${m.unit} of this commit, measured on its own (nothing to compare against)</span>` : ''}<span class="muted"${oneSided ? ' style="display:none"' : ''}>${m.label} ${m.unit}: base → head and Δ per engine and operation. Δ is the median over the passes. Green/red: a change (outside the band, most passes agree); bold: |Δ| ≥ 5%; grey: no change. Click a bar or cell for every sample.</span></div>
      ${sectionsHTML(mxs, m, mode, j.ID, d.Subject)}
      ${d.Noise && d.Noise.Jobs ? `<p class="note">Noise floor over ${d.Noise.Jobs} self-comparisons: median |Δ time| ${d.Noise.MedianAbs.toFixed(2)}%, p95 ${d.Noise.P95Abs.toFixed(2)}%.</p>` : ''}
      <details><summary>Raw files</summary><p class="mono">${(d.Files || []).map(f => `<a href="/raw/${j.ID}/${f}" target="_blank">${f}</a>`).join(' · ')}</p></details>`;
    bindMetricTabs(() => viewJob(id));
    destroyCharts();
    drawSections(mxs, m, mode, j.ID);
    document.querySelectorAll('#modeTabs button').forEach(b => b.onclick = () => { chartMode = b.dataset.mode; localStorage.setItem('chartMode', chartMode); viewJob(id); });
    const libs = document.getElementById('chartLibs');
    if (libs) libs.onchange = () => { chartLibs = libs.checked; localStorage.setItem('chartLibs', chartLibs ? '1' : '0'); viewJob(id); };
    if (d.Live) scheduleRefresh(20000);
  }

  /* ---------- commit: every run of one commit against a chosen base ---------- */
  const commitHref = (subject, sha, base) => `#/commit/${subject}/${sha}${base !== undefined && base !== null ? '?base=' + encodeURIComponent(base) : ''}`;
  async function viewCommit(subject, sha, params) {
    const baseSel = params.get('base');
    const d = await get(`/api/commit/${subject}/${sha}` + (baseSel !== null ? '?base=' + encodeURIComponent(baseSel) : ''));
    leafNoise = (d.Noise && d.Noise.PerLeaf) || {};
    const m = curMetric(true);
    d.Results.forEach(withExtras);
    const mxs = buildMatrices(d.Results);
    const measured = mxs.filter(mx => !mx.unsupported);
    const oneSided = measured.length > 0 && measured.every(mx => mx.oneSided);
    const mode = oneSided ? 'abs' : chartMode;
    const again = () => viewCommit(subject, sha, params);
    const options = [`<option value="none" ${d.Base ? '' : 'selected'}>nothing: the values of this commit alone</option>`]
      .concat(d.Bases.map(b => `<option value="${b.SHA}" ${b.SHA === d.Base ? 'selected' : ''}>${esc(b.Label)} · ${shortRef(b.SHA)} · ${b.Runs ? b.Runs + ' job' + (b.Runs === 1 ? '' : 's') : 'not measured'}</option>`));
    app.innerHTML = `<h1>Commit ${commitLink(d.SHA, d.Subject)} <span class="muted small">${repoLink(d.Subject)}</span> <a class="agent" href="/commit/${d.Subject}/${d.SHA}.md${d.Base ? '?base=' + d.Base : ''}" title="this page as text, for an agent">text</a></h1>
      <div class="cards">
        <div class="card"><h3>Commit</h3><div class="mono">${esc((d.Desc || '').replace(/^[0-9a-f]{7,12} /, ''))}</div><div class="sub">everything measured of this commit: ${d.Jobs.length} job${d.Jobs.length === 1 ? '' : 's'}, harness ${esc(d.Harness)}</div></div>
        <div class="card"><h3>Compared against</h3><select id="commitBase">${options.join('')}</select>
          <div class="sub">${d.Base ? (d.NoBase ? '<span class="worse">this base has no runs with this harness version: the values stand alone</span>' : `${commitLink(d.Base, d.Subject)} · ${d.BaseRuns} job${d.BaseRuns === 1 ? '' : 's'} measured it. Runs of both commits are paired by layout seed; a seed only one of them was run under does not count.`) : 'no base chosen'}</div></div>
      </div>
      <div class="toolbar">${metricTabs(true)}<div class="tabs" id="modeTabs"${oneSided ? ' style="display:none"' : ''}><button data-mode="rel" class="${chartMode === 'rel' ? 'active' : ''}">change in %</button><button data-mode="abs" class="${chartMode === 'abs' ? 'active' : ''}">measured values</button></div>${mode === 'abs' ? `<label class="check"><input type="checkbox" id="chartLibs" ${chartLibs ? 'checked' : ''}> with the other libraries</label>` : ''}</div>
      ${measured.length ? sectionsHTML(mxs, m, mode, null, d.Subject) : '<p class="muted">Nothing is measured of this commit yet: no job of it has finished.</p>'}
      <h2>${measured.length ? 'Jobs that measured this commit' : 'Jobs of this commit'}</h2>${jobRows(d.Jobs)}
      ${d.BaseJobs.length ? `<h2>Jobs that measured the base</h2>${jobRows(d.BaseJobs)}` : ''}`;
    bindMetricTabs(again);
    destroyCharts();
    drawSections(mxs, m, mode, null);
    document.querySelectorAll('#modeTabs button').forEach(b => b.onclick = () => { chartMode = b.dataset.mode; localStorage.setItem('chartMode', chartMode); again(); });
    const libs = document.getElementById('chartLibs');
    if (libs) libs.onchange = () => { chartLibs = libs.checked; localStorage.setItem('chartLibs', chartLibs ? '1' : '0'); again(); };
    document.getElementById('commitBase').onchange = ev => { location.hash = commitHref(subject, sha, ev.target.value); };
  }

  async function viewLeaf(id, engine, object, op) {
    const d = await get(`/api/job/${id}/leaf/${engine}/${object}/${op}`);
    const r = d.Result, m = curMetric(true);
    const samples = d.Samples.filter(s => !(s.Extra && s.Extra.diag));
    const diag = d.Samples.filter(s => s.Extra && s.Extra.diag);
    const X = r.Extra || {};
    const xfmt = k => EXTRAS[k].bytes ? fmtBytes : (EXTRAS[k].fmt || fmtNum);
    const xline = k => X[k] ? `<div class="sub mono">${EXTRAS[k].name}: ${r.Baseline || !X[k].Base ? xfmt(k)(X[k].Head) : `${xfmt(k)(X[k].Base)} → ${xfmt(k)(X[k].Head)}${X[k].Base > 0 ? ' ' + pct((X[k].Head - X[k].Base) / X[k].Base * 100, 1) : ''}`} <span class="muted">(${X[k].N} run${X[k].N === 1 ? '' : 's'} per side)</span></div>` : '';
    const counterCard = ['br-miss', 'l2-miss', 'fe-stall', 'l1d-miss'].some(k => X[k]) ? `<div class="card"><h3>More counters per op <span class="muted" style="text-transform:none">(median)</span></h3>${['br-miss', 'fe-stall', 'l1d-miss', 'l2-miss'].map(xline).join('')}${X.threads ? `<div class="sub">counted over all ${X.threads.Head} threads of the process</div>` : ''}</div>` : '';
    const memCard = X.stack || X.retained || X.faults ? `<div class="card"><h3>Memory and kernel</h3>${['retained', 'stack', 'faults', 'sys-ns'].map(xline).join('')}</div>` : '';
    const pairText = s => s.Extra ? Object.keys(EXTRAS).filter(k => !EXTRAS[k].bytes && !EXTRAS[k].kernel && s.Extra[k] !== undefined).map(k => `${EXTRAS[k].short} ${fmtNum(s.Extra[k])}`).join(' · ') : '';
    // offClock marks a run whose clock rate (cycles per ns) is more than
    // 1% off the median of all runs shown.
    const clocks = samples.filter(s => s.Cycles && s.Ns && !isAsync(engine)).map(s => s.Cycles / s.Ns).sort((a, b) => a - b);
    const clockMed = clocks.length ? clocks[Math.floor(clocks.length / 2)] : 0;
    const offClock = s => clockMed > 0 && s.Cycles && s.Ns && Math.abs(s.Cycles / s.Ns / clockMed - 1) > 0.01;
    const stat = (M, f) => r.Baseline
      ? `<div class="big">${f(M.Head)}</div><div class="sub">cv ${M.CVHead.toFixed(2)}%</div>`
      : `<div class="big">${badge(M)}</div><div class="sub">${f(M.Base)} → ${f(M.Head)} · 95% [${pct(M.Lo, 1)}, ${pct(M.Hi, 1)}] · p ${M.P.toFixed(3)} · median Δ ${pct(M.MedDelta)} · cv ${M.CVBase.toFixed(2)}% / ${M.CVHead.toFixed(2)}%</div>`;
    app.innerHTML = `<h1><span class="mono">${engName(engine)} / ${object} / ${opLabel(engine, op)}</span> <span class="muted small">job <a href="#/job/${id}">#${id}</a>${d.Runs.length > 1 ? `, pooled over ${d.Runs.length} runs` : ''}</span></h1>
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
      <div class="toolbar">${metricTabs(true)}<span class="muted">every sample, in measurement order (pass, seed, side); dashed lines are the side means</span></div>
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
    const val = s => m.extra ? (s.Extra || {})[m.extra] : s[m.key];
    const sides = ['base', 'head'];
    const datasets = sides.map((side, i) => ({
      label: side, type: 'scatter', pointRadius: 5, backgroundColor: COLORS[i] + 'cc', borderColor: COLORS[i],
      data: samples.map((s, k) => s.Side === side && val(s) !== undefined ? { x: k + 1, y: val(s), s } : null).filter(Boolean),
    }));
    sides.forEach((side, i) => {
      const vs = samples.filter(s => s.Side === side).map(val).filter(v => v !== undefined);
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
  // Which commit of every library the operations page shows: the head of
  // its main branch, or its latest release. Remembered per browser.
  let opsMode = localStorage.getItem('opsMode') === 'release' ? 'release' : 'master';
  const shortRef = sha => /^[0-9a-f]{40}$/.test(sha) ? sha.slice(0, 12) : sha;

  async function viewOps() {
    const d = await get('/api/ops?mode=' + opsMode);
    const rows = d.Rows || [], m = curMetric(false);
    // One column per engine or library; its async hashing has a row of
    // its own below the operation it runs.
    const all = sortEngines([...new Set((d.Engines || []).map(baseEngine))]);
    const engines = all.filter(e => !opsHidden.has(e));
    const cell = (r, e) => {
      const c = r.Cells[e];
      // An engine with the operation but not its async hashing: the
      // library, or this version of it, has no background hashing.
      if (!c && isAsync(e) && r.Cells[baseEngine(e)]) return '<td class="grp muted" title="no async hashing in this library at the commit shown: its job leaves the operation out">none</td>';
      if (!c) return '<td class="grp muted">-</td>';
      const em = metricFor(m, e, c), v = c[em.key];
      if (!(v > 0) && em.key !== 'Bytes' && em.key !== 'Allocs') return '<td class="grp muted">-</td>';
      const unit = em !== m ? ' <span class="ci">time</span>' : '';
      const rel = c.Rel && c.Rel[em.key] !== undefined ? c.Rel[em.key] : null;
      const cv = em.key === 'Ns' ? c.CVNs : em.key === 'Cycles' ? c.CVCycles : 0;
      return `<td class="grp"><b title="median of ${c.N} runs of this commit${cv ? `, spread ${cv.toFixed(1)}%` : ''}${c.Threads ? `, counted over all ${c.Threads} threads` : ''}">${em.fmt(v)}</b>${unit}${rel !== null ? ` <span class="ci ${Math.abs(rel) < 1 ? '' : rel < 0 ? 'better' : 'worse'}" title="master against the latest release of the same library">${pct(rel, 1)} vs release</span>` : ''}</td>`;
    };
    const asyncRow = r => Object.keys(r.Cells).some(isAsync)
      ? `<tr><td><span class="muted">${r.Object}</span></td><td class="mono muted">${r.Op} (async${(m.key === 'Cycles' || m.key === 'Instrs') ? ', all threads' : ''})</td>${engines.map(e => cell(r, e + 'Async')).join('')}</tr>` : '';
    const subjectChip = s => {
      const link = commitLink(s.SHA, s.Name);
      const state = !s.Jobs ? ' · <span class="muted">not measured</span>' : ` · <a href="${commitHref(s.Name, s.SHA)}" title="everything measured of this commit">${s.Jobs} job${s.Jobs === 1 ? '' : 's'}, ${s.Runs} runs</a>`;
      const wanted = s.Wanted ? ` · <span class="worse" title="the target points to ${esc(s.Wanted)}, which is not measured yet or does not build; an older commit is shown">stale</span>` : '';
      return `<div class="card"><h3>${esc(s.Name)}${s.Target === 'fixed' ? ' <span class="muted" style="text-transform:none">(fixed version)</span>' : ''}</h3><div class="sub">${esc(s.Label)} · ${link}${state}${wanted}</div></div>`;
    };
    app.innerHTML = `<h1>Operations</h1>
      <div class="toolbar">${metricTabs()}<div class="tabs" id="opsMode"><button data-mode="master" class="${opsMode === 'master' ? 'active' : ''}" title="every library at the head of its main branch">master</button><button data-mode="release" class="${opsMode === 'release' ? 'active' : ''}" title="every library at its latest release">release</button></div>
        <span class="muted">${opsMode === 'master' ? 'every library at the head of its main branch, with the change against its latest release' : 'every library at its latest release'}; a value is the median over all runs of that commit in every job that measured it, and idle time adds runs</span></div>
      <div class="cards">${d.Subjects.map(subjectChip).join('')}</div>
      <div class="toolbar chips" id="engsel">${all.map(e => `<a href="#" data-e="${e}" class="chip ${opsHidden.has(e) ? '' : 'commit'}" title="show or hide this column">${engName(e)}</a>`).join(' ')}<a href="#" data-e="*" class="chip">all</a><a href="#" data-e="-" class="chip">dynamic-ssz only</a></div>
      <div style="overflow-x:auto"><table><thead><tr><th>Object</th><th>Operation</th>${engines.map(e => `<th class="grp">${engName(e)}</th>`).join('')}</tr></thead><tbody>
      ${rows.map((r, i) => `<tr><td>${i > 0 && rows[i - 1].Object === r.Object ? `<span class="muted">${r.Object}</span>` : objName(r.Object)}</td><td class="mono"><a href="#/op/${r.Object}/${r.Op}">${r.Op}</a></td>${engines.map(e => cell(r, e)).join('')}</tr>${asyncRow(r)}`).join('')}
      </tbody></table></div>${rows.length ? '' : '<p class="muted">no commit of this mode has been measured yet</p>'}
      <p class="note">HashTreeRoot (async) is the hash tree root with background hashing workers, an operation of the engine above it; a library or version without that option has none.</p>`;
    bindMetricTabs(viewOps);
    document.querySelectorAll('#opsMode button').forEach(b => b.onclick = () => { opsMode = b.dataset.mode; localStorage.setItem('opsMode', opsMode); viewOps(); });
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
    const m = curMetric(false);
    const chips = sums => engineTotals(sums).map(x => `<span class="chip" title="geomean over ${x.N} operations (${x.Cycles ? 'cycles, time for the async hashing' : 'time'})">${engName(x.Engine)} <b>${pct(x.Geomean, 1)}</b></span>`).join('') || '<span class="muted">-</span>';
    const measured = d.Rows.filter(r => r.Job && r.Job.State === 'done');
    const leaves = [...new Set(measured.flatMap(r => r.Results.map(x => x.Object + '/' + x.Op)))].sort((a, b) => { const [ao, ap] = a.split('/'), [bo, bp] = b.split('/'); return rank(OBJECTS, ao) - rank(OBJECTS, bo) || rank(OPS, ap) - rank(OPS, bp); });
    let leaf = leaves.includes(prLeaf) ? prLeaf : '';
    if (prMode === 'abs' && !leaf) leaf = leaves.includes('FuluState/HashTreeRoot') ? 'FuluState/HashTreeRoot' : (leaves[0] || '');
    const count = d.Rows.filter(r => !r.Detached).length;
    app.innerHTML = `<h1>Pull request #${d.PR}${ghLink(`${repo}/pull/${d.PR}`, 'the pull request on GitHub')} <span class="mono muted" style="font-size:13px;font-weight:400">${esc(d.Branch || '')}</span></h1>
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
        label: engName(e) + (isAsync(e) ? ' (async)' : '') + (metricFor(m, e) !== m && leaf ? ' (time)' : ''), borderColor: COLORS[rank(ENGINES, e) % COLORS.length], backgroundColor: COLORS[rank(ENGINES, e) % COLORS.length], borderDash: isAsync(e) ? [5, 3] : undefined, tension: 0, pointRadius: 4, spanGaps: true,
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
    const m = curMetric(false);
    const main = (await get('/api/status')).MainBranch;
    // One card and one column per engine; the async hashing of an engine
    // (its *Async engine of the harness) is a line inside them.
    const engines = sortEngines([...new Set(v.Engines.map(baseEngine))]);
    const asyncOf = e => v.Engines.includes(e + 'Async') ? e + 'Async' : null;
    const latest = e => { const l = v.Latest[e]; return l ? `${fmtNs(l.Ns.Head)}` : ''; };
    app.innerHTML = `<h1><span class="mono">${object} / ${op}</span>${objInfo(object)}</h1>
      <div class="cards">${engines.map((e, i) => {
        const l = v.Latest[e], t = v.Trends[e], n = v.Noise[e], a = asyncOf(e), la = a && v.Latest[a], ta = a && v.Trends[a];
        if (!l) return '';
        return `<div class="card"><h3><i class="legend"><i style="background:${COLORS[i]}"></i></i>${engName(e)}</h3><div class="big">${fmtNs(l.Ns.Head)}</div><div class="sub">${fmtBytes(l.Bytes.Head)} · ${fmtNum(l.Allocs.Head)} allocs/op · latest head (job <a href="#/job/${l.JobID}">#${l.JobID}</a>)</div>${t && t.N >= 3 ? `<div class="sub">master trend ${pct(t.SlopePct30d, 1)} / 30d over ${t.N} points (R² ${t.R2.toFixed(2)}), projected ${fmtNs(t.Projected30d)}</div>` : ''}${n ? `<div class="sub">noise floor p95 |Δ| ${n.toFixed(2)}%</div>` : ''}${la ? `<div class="sub">async: <b>${latest(a)}</b>${ta && ta.N >= 3 ? `, trend ${pct(ta.SlopePct30d, 1)} / 30d` : ''}${v.Noise[a] ? `, noise floor p95 ${v.Noise[a].toFixed(2)}%` : ''}</div>` : ''}</div>`;
      }).join('')}</div>
      <div class="toolbar">${metricTabs()}<span class="muted">master history: head value of every ${esc(main)} job, newest right</span></div>
      <div class="chart h300"><canvas id="hc"></canvas></div>
      <h2>Every job</h2>
      <table><thead><tr><th>Job</th><th>Kind</th><th>Repository</th><th>Branch</th><th>Head</th><th>Base</th>${engines.map(e => `<th class="grp">${engName(e)}</th>`).join('')}</tr></thead><tbody>
      ${v.History.map(row => `<tr><td><a href="#/job/${row.Job.ID}">#${row.Job.ID}</a></td><td>${kindChips(row.Job)}</td><td>${repoLink(row.Job.Subject)}</td><td class="mono">${esc(row.Job.Branch)}</td><td>${commitLink(row.Job.HeadSHA, row.Job.Subject)}</td><td>${commitLink(row.Job.BaseSHA, row.Job.Subject)}</td>${engines.map(e => {
        const r = row.Results[e], ra = asyncOf(e) && row.Results[asyncOf(e)];
        const one = (x, label) => { const xm = metricFor(m, x.Engine, x), body = `${label ? `<span class="muted small">${label}${xm !== m ? ' (time)' : ''}</span> ` : ''}${cellAbs(x, xm)}`; return `<a href="#/job/${row.Job.ID}/leaf/${x.Engine}/${x.Object}/${x.Op}">${body}</a>`; };
        const show = (x, label) => x.Baseline ? one(x, label) : cellDelta(x, m, row.Job.ID, label);
        return `<td class="grp cell">${r ? show(r) : '<span class="muted">-</span>'}${ra ? show(ra, 'async') : ''}</td>`;
      }).join('')}</tr>`).join('')}
      </tbody></table>`;
    bindMetricTabs(() => viewOp(object, op));
    destroyCharts();
    const pts = v.History.slice().reverse().filter(row => row.Job.Branch === main && row.Job.Kind !== 'noise' && row.Job.Finished);
    // The async hashing is a dashed line in the colour of its engine; on a
    // counter tab a job that counted one thread of it only leaves a gap.
    const datasets = v.Engines.filter(e => pts.some(row => row.Results[e])).map(e => { const i = engines.indexOf(baseEngine(e)); return {
      label: engName(e) + (isAsync(e) ? ' (async)' : ''), borderColor: COLORS[i % COLORS.length], backgroundColor: COLORS[i % COLORS.length], borderDash: isAsync(e) ? [5, 3] : undefined, pointRadius: 3, borderWidth: 1.5, tension: 0.1, spanGaps: true,
      data: pts.map(row => { const r = row.Results[e]; return r && metricFor(m, e, r) === m ? { x: when(row.Job.Finished), y: r[m.key].Head, j: row.Job } : null; }).filter(Boolean),
    }; });
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
    const m = curMetric(false);
    const key = { ns: ['Ns', 'NsDelta'], cycles: ['Cycles', 'CyclesDelta'], instrs: ['Instrs', 'InstrsDelta'], bytes: ['Bytes', 'BytesDelta'], allocs: ['Allocs', 'AllocsDelta'] }[METRICS[metric] && !METRICS[metric].extra ? metric : 'cycles'];
    let body = '';
    if (a && b) {
      if (d.AJob && d.BJob) {
        body = `<div class="cards">
          <div class="card"><h3>A</h3><div class="mono">${commitLink(d.A)}</div><div class="sub">job <a href="#/job/${d.AJob.ID}">#${d.AJob.ID}</a> (${d.ASide}) · ${when(d.AJob.Finished)}</div></div>
          <div class="card"><h3>B</h3><div class="mono">${commitLink(d.B)}</div><div class="sub">job <a href="#/job/${d.BJob.ID}">#${d.BJob.ID}</a> (${d.BSide}) · ${when(d.BJob.Finished)}</div></div>
          <div class="card"><h3>Source</h3><div class="big">${d.AJob.ID === d.BJob.ID ? 'same job' : 'different jobs'}</div><div class="sub">${d.AJob.ID === d.BJob.ID ? 'interleaved measurement' : 'measured at different times; the noise floor applies on top'}</div></div></div>
          <div class="toolbar">${metricTabs()}</div>
          <table><thead><tr><th>Engine</th><th>Object</th><th>Operation</th><th class="num">A</th><th class="num">B</th><th class="num">Δ</th></tr></thead><tbody>
          ${d.Rows.map(r => { const dl = r[key[1]]; return `<tr><td>${engName(r.Engine)}</td><td>${r.Object}</td><td class="mono"><a href="#/op/${r.Object}/${r.Op}">${opLabel(r.Engine, r.Op)}</a></td><td class="num">${m.fmt(r.A[key[0]])}</td><td class="num">${m.fmt(r.B[key[0]])}</td><td class="num"><span class="badge ${Math.abs(dl) < 1 ? 'same' : dl < 0 ? 'better' : 'worse'}">${pct(dl)}</span></td></tr>`; }).join('')}
          </tbody></table>`;
      } else body = '<p class="err">One of the commits has no finished measurement.</p>';
    }
    app.innerHTML = `<h1>Compare two commits</h1>
      <form id="cmp" class="toolbar"><label>A <input type="text" name="a" value="${esc(a)}" placeholder="sha, branch or tag"></label><label>B <input type="text" name="b" value="${esc(b)}" placeholder="sha, branch or tag"></label><button class="btn" type="submit">Compare</button></form>
      <p class="note">Values come from the newest finished job that measured each commit, as head or base.</p>
      ${body}
      <h2>Measured commits <span class="muted small">click a pair to compare its two sides</span></h2>
      <table><thead><tr><th>Job</th><th>Kind</th><th>Branch</th><th>Head</th><th>Base</th><th>Finished</th><th></th></tr></thead><tbody>
      ${d.Jobs.filter(j => j.State === 'done').map(j => `<tr><td><a href="#/job/${j.ID}">#${j.ID}</a></td><td><span class="chip ${j.Kind}">${j.Kind}</span></td><td class="mono">${esc(j.Branch)}</td><td>${commitLink(j.HeadSHA)} <span class="muted">${esc(j.HeadDesc).replace(/^[0-9a-f]{7} /, '')}</span></td><td>${commitLink(j.BaseSHA, j.Subject)} <span class="muted">${esc(j.BaseRef)}</span></td><td class="muted">${when(j.Finished)}</td><td><a href="#/compare?a=${j.BaseSHA}&b=${j.HeadSHA}">compare</a></td></tr>`).join('')}
      </tbody></table>`;
    document.getElementById('cmp').onsubmit = ev => { ev.preventDefault(); const f = ev.target; location.hash = `#/compare?a=${encodeURIComponent(f.a.value.trim())}&b=${encodeURIComponent(f.b.value.trim())}`; };
    bindMetricTabs(() => viewCompare(params));
  }

  async function viewNoise() {
    const d = await get('/api/noise');
    const m = curMetric(false);
    const st = r => r[m.key];
    let sortKey = 'leaf';
    const render = () => {
      const rows = d.Rows.slice();
      if (sortKey === 'p95') rows.sort((a, b) => st(b).P95Abs - st(a).P95Abs);
      else if (sortKey === 'cv') rows.sort((a, b) => st(b).CV - st(a).CV);
      else rows.sort(sortLeaf);
      return `<table><thead><tr><th>Runner</th><th>Engine</th><th>Object</th><th>Operation</th><th class="num">n</th><th class="num">median |Δ|</th><th class="num sortable" data-k="p95">p95 |Δ| ▾</th><th class="num">max |Δ|</th><th class="num sortable" data-k="cv">median CV ▾</th><th class="num">steal</th></tr></thead><tbody>
        ${rows.map(r => { const s = st(r); return `<tr><td class="mono muted">${esc(r.Runner)}</td><td>${engName(r.Engine)}</td><td>${r.Object}</td><td class="mono"><a href="#/op/${r.Object}/${r.Op}">${opLabel(r.Engine, r.Op)}</a></td><td class="num">${r.N}</td><td class="num">${s.MedianAbs.toFixed(2)}%</td><td class="num"><span class="badge ${s.P95Abs < 1 ? 'better' : s.P95Abs < 3 ? 'same' : 'worse'}">${s.P95Abs.toFixed(2)}%</span></td><td class="num">${s.MaxAbs.toFixed(2)}%</td><td class="num">${s.CV.toFixed(2)}%</td><td class="num">${r.Steal}</td></tr>`; }).join('')}
        </tbody></table>`;
    };
    const draw = () => {
      app.innerHTML = `<h1>Noise floor</h1>
        <div class="cards">
          <div class="card"><h3>Self-comparisons</h3><div class="big">${d.Jobs}</div><div class="sub">the newest pairs of runs of one commit under the same layout seeds in different jobs (idle refinement repeats them), and jobs that measured one commit on both sides</div></div>
          ${Object.entries(d.PerRunner || {}).map(([name, st]) => `<div class="card"><h3>|Δ time| on ${esc(name)}</h3><div class="big">${st.MedianAbs.toFixed(2)}% <span class="muted">median</span> · ${st.P95Abs.toFixed(2)}% <span class="muted">p95</span></div><div class="sub">over all operations and self-comparisons of this machine</div></div>`).join('')}
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
    hideCallout();
    const hash = location.hash || '#/';
    const [path, query] = hash.slice(1).split('?');
    const params = new URLSearchParams(query || '');
    const parts = path.split('/').filter(Boolean);
    document.querySelectorAll('header nav a').forEach(a => a.classList.toggle('active', a.dataset.nav === (parts[0] || 'dash') || (['jobs', 'job', 'pr'].includes(parts[0]) && a.dataset.nav === 'dash') || (['repo', 'commit'].includes(parts[0]) && a.dataset.nav === 'repos') || (parts[0] === 'op' && a.dataset.nav === 'ops')));
    try {
      if (parts.length === 0) { await viewDash(); scheduleRefresh(30000); }
      else if (parts[0] === 'jobs') await viewJobs(params.get('kind'));
      else if (parts[0] === 'job' && parts[2] === 'leaf') await viewLeaf(parts[1], parts[3], parts[4], parts.slice(5).join('/'));
      else if (parts[0] === 'job') await viewJob(parts[1]);
      else if (parts[0] === 'repos') await viewRepos();
      else if (parts[0] === 'repo') await viewRepo(decodeURIComponent(parts[1] || ''), params.get('page'));
      else if (parts[0] === 'ops') await viewOps();
      else if (parts[0] === 'commit') await viewCommit(parts[1], parts[2], params);
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
  // The status names the repositories the pages link to: it comes first.
  refreshLive().then(route);
})();
