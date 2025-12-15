// SSZ Benchmark Visualization - Main Comparison Page
// Uses LIBRARIES from libraries.js (loaded via script tag in HTML)

// Base operations (always 3 charts)
const OPERATIONS = ['Unmarshal', 'Marshal', 'HashTreeRoot'];

// Mapping from base operation to streaming operation
const STREAM_OPERATION_MAP = {
    'Unmarshal': 'UnmarshalReader',
    'Marshal': 'MarshalWriter',
    'HashTreeRoot': null  // No streaming version
};

const METRICS = ['time', 'memory', 'alloc'];

// Libraries that support streaming
const STREAMING_LIBRARIES = ['dynamicssz-codegen', 'dynamicssz-reflection', 'karalabessz'];

// Mode selection: 'buffer', 'stream', or both
const MODES = [
    { name: 'buffer', displayName: 'Buffer', description: 'Standard buffer-based operations' },
    { name: 'stream', displayName: 'Stream', description: 'Streaming operations (Reader/Writer)' }
];

let aggregatedData = {};
let currentTab = 'mainnet';
let currentType = 'Block';
let currentTimelineOp = 'Unmarshal';
let currentTimelineMetric = 'time';
let currentTimelineRange = '30'; // days or 'all'
let showDevVersions = false;
let charts = {};
let timelineChart = null;
let selectedLibraries = new Set(LIBRARIES.filter(l => l.name !== 'ztyp').map(l => l.name)); // ZTYP excluded by default due to scale
let selectedModes = new Set(['buffer']); // Default to buffer mode only

const DAY_IN_SECONDS = 86400;

// Format bytes to human readable
function formatBytes(bytes) {
    if (bytes >= 1e9) return (bytes / 1e9).toFixed(2) + ' GB';
    if (bytes >= 1e6) return (bytes / 1e6).toFixed(2) + ' MB';
    if (bytes >= 1e3) return (bytes / 1e3).toFixed(2) + ' KB';
    return bytes + ' B';
}

// Get current payload type metadata
function getPayloadTypeMetadata() {
    const preset = currentTab === 'mainnet' ? 'Mainnet' : 'Minimal';
    const payloadType = PayloadTypes.find(t => t.name === currentType);
    if (!payloadType) return null;

    const presetData = payloadType.presets.find(p => p.name === preset);
    return {
        fork: payloadType.fork,
        size: presetData ? presetData.size : null
    };
}

// Update metadata display
function updateMetadataDisplay() {
    const metadata = getPayloadTypeMetadata();
    const forkEl = document.getElementById('metadata-fork');
    const sizeEl = document.getElementById('metadata-size');

    if (metadata) {
        forkEl.textContent = metadata.fork;
        sizeEl.textContent = metadata.size ? formatBytes(metadata.size) : '-';
    } else {
        forkEl.textContent = '-';
        sizeEl.textContent = '-';
    }
}

// Get the cutoff timestamp based on selected range
function getTimeRangeCutoff() {
    if (currentTimelineRange === 'all') return 0;
    const days = parseInt(currentTimelineRange, 10);
    const now = Math.floor(Date.now() / 1000);
    return now - (days * DAY_IN_SECONDS);
}

// Parse semver string and return comparable object
function parseSemver(version) {
    // Handle formats like "v1.0.0", "v0.0.0-20251126100127", "v1.2.3-beta"
    const match = version.match(/^v?(\d+)\.(\d+)\.(\d+)(?:-(.+))?$/);
    if (!match) return null;

    const prerelease = match[4] || null;
    // Check if prerelease is a Go pseudo version timestamp (YYYYMMDDHHMMSS format)
    const isTimestamp = prerelease && /^\d{14}$/.test(prerelease);

    return {
        major: parseInt(match[1], 10),
        minor: parseInt(match[2], 10),
        patch: parseInt(match[3], 10),
        prerelease: prerelease,
        timestamp: isTimestamp ? parseInt(prerelease, 10) : null,
        original: version
    };
}

// Compare two semver objects, returns positive if a > b
function compareSemver(a, b) {
    if (!a && !b) return 0;
    if (!a) return -1;
    if (!b) return 1;

    if (a.major !== b.major) return a.major - b.major;
    if (a.minor !== b.minor) return a.minor - b.minor;
    if (a.patch !== b.patch) return a.patch - b.patch;

    // Both have same major.minor.patch - check prereleases
    // Versions without prerelease are greater than those with prerelease
    if (!a.prerelease && b.prerelease) return 1;
    if (a.prerelease && !b.prerelease) return -1;

    if (a.prerelease && b.prerelease) {
        // If both are Go pseudo version timestamps, compare numerically
        if (a.timestamp && b.timestamp) {
            return a.timestamp - b.timestamp;
        }
        // If only one is a timestamp, the timestamp is newer (pseudo versions are dev builds)
        if (a.timestamp && !b.timestamp) return 1;
        if (!a.timestamp && b.timestamp) return -1;
        // Otherwise compare as strings
        return a.prerelease.localeCompare(b.prerelease);
    }

    return 0;
}

// Get latest non-dev version from aggregations
function getLatestVersion(aggregations) {
    let latest = null;
    let latestSemver = null;

    for (const agg of aggregations) {
        // Skip dev versions
        if (agg.dev === true) continue;

        const semver = parseSemver(agg.version);
        if (compareSemver(semver, latestSemver) > 0) {
            latest = agg;
            latestSemver = semver;
        }
    }

    return latest;
}

// Load all aggregation and raw files
async function loadData() {
    const results = {};

    for (const lib of LIBRARIES) {
        const files = getLibraryFiles(lib);
        try {
            // Load aggregation file
            const aggResponse = await fetch(`results/${files.aggregationFile}`);
            if (aggResponse.ok) {
                const aggData = await aggResponse.json();
                results[lib.name] = {
                    displayName: lib.displayName,
                    baseColor: lib.baseColor,
                    aggregations: aggData.aggregations,
                    rawBenchmarks: []
                };

                // Load raw file
                try {
                    const rawResponse = await fetch(`results/${files.rawFile}`);
                    if (rawResponse.ok) {
                        const rawData = await rawResponse.json();
                        results[lib.name].rawBenchmarks = rawData.benchmarks || [];
                    }
                } catch (rawError) {
                    console.warn(`Failed to load raw file ${files.rawFile}:`, rawError);
                }
            }
        } catch (error) {
            console.warn(`Failed to load ${files.aggregationFile}:`, error);
        }
    }

    return results;
}

// Generate color with alpha
function rgba(color, alpha) {
    return `rgba(${color[0]}, ${color[1]}, ${color[2]}, ${alpha})`;
}

// Generate color variant for different versions of same library
function getVersionColor(baseColor, versionIndex, totalVersions) {
    // Lighten the color for older versions
    const factor = 1 - (versionIndex / (totalVersions + 1)) * 0.5;
    return [
        Math.round(baseColor[0] + (255 - baseColor[0]) * (1 - factor)),
        Math.round(baseColor[1] + (255 - baseColor[1]) * (1 - factor)),
        Math.round(baseColor[2] + (255 - baseColor[2]) * (1 - factor))
    ];
}

// Format number for display
function formatNumber(num, metric) {
    if (num === null || num === undefined) return 'N/A';

    if (metric === 'time') {
        if (num >= 1e9) return (num / 1e9).toFixed(2) + ' s';
        if (num >= 1e6) return (num / 1e6).toFixed(2) + ' ms';
        if (num >= 1e3) return (num / 1e3).toFixed(2) + ' us';
        return num.toFixed(2) + ' ns';
    }

    if (metric === 'memory') {
        if (num >= 1e9) return (num / 1e9).toFixed(2) + ' GB';
        if (num >= 1e6) return (num / 1e6).toFixed(2) + ' MB';
        if (num >= 1e3) return (num / 1e3).toFixed(2) + ' KB';
        return num.toFixed(2) + ' B';
    }

    return num.toFixed(2);
}

// Get bar chart data for a specific operation and metric
// Shows both buffer and stream data points when both modes are selected
function getBarChartData(operation, metric) {
    const prefix = currentTab === 'mainnet' ? 'Mainnet' : 'Minimal';
    const bufferKey = `${operation}${prefix}${currentType}`;
    const streamOp = STREAM_OPERATION_MAP[operation];
    const streamKey = streamOp ? `${streamOp}${prefix}${currentType}` : null;

    // HashTreeRoot has no streaming version, so always show buffer data for it
    const hasStreamingVersion = streamKey !== null;
    const showBuffer = selectedModes.has('buffer') || !hasStreamingVersion;
    const showStream = selectedModes.has('stream') && hasStreamingVersion;
    const showBoth = showBuffer && showStream;

    const labels = [];
    const data = [];
    const colors = [];
    const metadata = [];

    LIBRARIES.forEach((lib, index) => {
        if (!selectedLibraries.has(lib.name)) return;

        const libData = aggregatedData[lib.name];
        if (!libData || !libData.aggregations) return;

        const latest = getLatestVersion(libData.aggregations);
        if (!latest) return;

        const canStream = supportsStreaming(lib.name);

        // Add buffer data point
        if (showBuffer) {
            const result = latest.results[bufferKey];
            if (result) {
                // Label includes (Buffer) suffix only when showing both modes
                const label = showBoth ? `${libData.displayName} (Buf)` : libData.displayName;
                labels.push(label);
                colors.push(rgba(lib.baseColor, 0.8));

                let value, min, max;
                switch (metric) {
                    case 'time':
                        value = result.ns_op[0];
                        min = result.ns_op[1];
                        max = result.ns_op[2];
                        break;
                    case 'memory':
                        value = result.bytes[0];
                        min = result.bytes[1];
                        max = result.bytes[2];
                        break;
                    case 'alloc':
                        value = result.alloc[0];
                        min = result.alloc[1];
                        max = result.alloc[2];
                        break;
                }

                data.push(value);
                metadata.push({
                    value,
                    min,
                    max,
                    samples: result.samples,
                    version: latest.version,
                    mode: 'buffer'
                });
            }
        }

        // Add stream data point (only for libraries that support streaming)
        if (showStream && canStream) {
            const result = latest.results[streamKey];
            if (result) {
                // Label includes (Stream) suffix only when showing both modes
                const label = showBoth ? `${libData.displayName} (Str)` : libData.displayName;
                // Use a slightly different shade for streaming
                const streamColor = [
                    Math.min(255, lib.baseColor[0] + 40),
                    Math.min(255, lib.baseColor[1] + 40),
                    Math.min(255, lib.baseColor[2] + 40)
                ];
                labels.push(label);
                colors.push(rgba(showBoth ? streamColor : lib.baseColor, 0.8));

                let value, min, max;
                switch (metric) {
                    case 'time':
                        value = result.ns_op[0];
                        min = result.ns_op[1];
                        max = result.ns_op[2];
                        break;
                    case 'memory':
                        value = result.bytes[0];
                        min = result.bytes[1];
                        max = result.bytes[2];
                        break;
                    case 'alloc':
                        value = result.alloc[0];
                        min = result.alloc[1];
                        max = result.alloc[2];
                        break;
                }

                data.push(value);
                metadata.push({
                    value,
                    min,
                    max,
                    samples: result.samples,
                    version: latest.version,
                    mode: 'stream'
                });
            }
        }
    });

    return {
        labels,
        datasets: [{
            data,
            backgroundColor: colors,
            borderColor: colors.map(c => c.replace('0.8', '1')),
            borderWidth: 1,
            metadata
        }]
    };
}

// Create bar chart
function createBarChart(canvasId, operation, metric) {
    const ctx = document.getElementById(canvasId).getContext('2d');
    const chartKey = `${metric}-${operation.toLowerCase()}`;

    if (charts[chartKey]) {
        charts[chartKey].destroy();
    }

    const data = getBarChartData(operation, metric);

    charts[chartKey] = new Chart(ctx, {
        type: 'bar',
        data: data,
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    display: false
                },
                tooltip: {
                    callbacks: {
                        label: function(context) {
                            const meta = context.dataset.metadata[context.dataIndex];
                            if (!meta) return 'N/A';

                            return [
                                'Value: ' + formatNumber(meta.value, metric),
                                'Min: ' + formatNumber(meta.min, metric),
                                'Max: ' + formatNumber(meta.max, metric),
                                'Samples: ' + meta.samples,
                                'Version: ' + meta.version
                            ];
                        }
                    }
                }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    ticks: {
                        callback: function(value) {
                            return formatNumber(value, metric);
                        }
                    }
                },
                x: {
                    ticks: {
                        maxRotation: 45,
                        minRotation: 45
                    }
                }
            }
        }
    });
}

// Check if a library supports streaming
function supportsStreaming(libName) {
    return STREAMING_LIBRARIES.includes(libName);
}

// Update all bar charts in the matrix (always 3 charts: Unmarshal, Marshal, HashTreeRoot)
function updateBarCharts() {
    METRICS.forEach(metric => {
        OPERATIONS.forEach(operation => {
            const canvasId = `chart-${metric}-${operation.toLowerCase()}`;
            createBarChart(canvasId, operation, metric);
        });
    });
}

// Get value from raw benchmark result based on current metric
function getRawValue(rawResult, key) {
    const data = rawResult[key];
    if (!data) return null;

    // Raw format: [ns_per_op, bytes_alloc, num_allocs]
    switch (currentTimelineMetric) {
        case 'time':
            return data[0];
        case 'memory':
            return data[1];
        case 'alloc':
            return data[2];
    }
    return null;
}

// Aggregate raw values locally
function aggregateRawValues(values) {
    if (values.length === 0) return null;

    const sum = values.reduce((a, b) => a + b, 0);
    const avg = sum / values.length;
    const min = Math.min(...values);
    const max = Math.max(...values);

    return { avg, min, max, samples: values.length };
}

// Generate daily data points for a version's active period
function generateDailyPoints(agg, result, key, rawBenchmarks) {
    const points = [];
    const first = agg.first || agg.last;
    const last = agg.last || agg.first;

    if (!first) return points;

    // Apply time range filter
    const cutoff = getTimeRangeCutoff();

    // Skip entirely if version ended before cutoff
    if (last < cutoff) return points;

    // Clip start to cutoff if needed
    const clippedFirst = first < cutoff ? cutoff : first;

    // Get aggregated fallback values
    let aggValue, aggMin, aggMax;
    switch (currentTimelineMetric) {
        case 'time':
            aggValue = result.ns_op[0];
            aggMin = result.ns_op[1];
            aggMax = result.ns_op[2];
            break;
        case 'memory':
            aggValue = result.bytes[0];
            aggMin = result.bytes[1];
            aggMax = result.bytes[2];
            break;
        case 'alloc':
            aggValue = result.alloc[0];
            aggMin = result.alloc[1];
            aggMax = result.alloc[2];
            break;
    }

    // Get raw benchmarks for this version
    const versionRawData = rawBenchmarks
        .filter(b => b.version === agg.version)
        .sort((a, b) => a.time - b.time);

    // Group raw data by day
    const rawByDay = new Map();
    versionRawData.forEach(b => {
        const value = getRawValue(b.results, key);
        if (value !== null) {
            const day = Math.floor(b.time / DAY_IN_SECONDS) * DAY_IN_SECONDS;
            if (!rawByDay.has(day)) {
                rawByDay.set(day, []);
            }
            rawByDay.get(day).push(value);
        }
    });

    // Generate exactly one point per day
    const startDay = Math.floor(clippedFirst / DAY_IN_SECONDS) * DAY_IN_SECONDS;
    const endDay = Math.floor(last / DAY_IN_SECONDS) * DAY_IN_SECONDS;

    for (let day = startDay; day <= endDay; day += DAY_IN_SECONDS) {
        // Skip days before the clipped start
        if (day < Math.floor(clippedFirst / DAY_IN_SECONDS) * DAY_IN_SECONDS) continue;

        const dayRawValues = rawByDay.get(day);
        const localAgg = dayRawValues ? aggregateRawValues(dayRawValues) : null;

        if (localAgg) {
            // Use locally aggregated raw values
            points.push({
                x: day * 1000,
                y: localAgg.avg,
                version: agg.version,
                samples: localAgg.samples,
                min: localAgg.min,
                max: localAgg.max,
                isDev: agg.dev === true,
                isRaw: true
            });
        } else {
            // Fall back to pre-computed aggregation
            points.push({
                x: day * 1000,
                y: aggValue,
                version: agg.version,
                samples: result.samples,
                min: aggMin,
                max: aggMax,
                isDev: agg.dev === true,
                isRaw: false
            });
        }
    }

    return points;
}

// Get timeline chart data
// Uses mode selection to show buffer and/or stream data in the same chart
function getTimelineData() {
    const prefix = currentTab === 'mainnet' ? 'Mainnet' : 'Minimal';
    const baseOp = currentTimelineOp;
    const streamOp = STREAM_OPERATION_MAP[baseOp];
    const bufferKey = `${baseOp}${prefix}${currentType}`;
    const streamKey = streamOp ? `${streamOp}${prefix}${currentType}` : null;

    // Determine what to show based on mode selection
    const hasStreamingVersion = streamKey !== null;
    const showBuffer = selectedModes.has('buffer') || !hasStreamingVersion;
    const showStream = selectedModes.has('stream') && hasStreamingVersion;
    const showBoth = showBuffer && showStream;

    const datasets = [];

    // Helper function to add datasets for a specific operation key
    function addDatasetsForOperation(lib, libData, opKey, isStreamMode) {
        const rawBenchmarks = libData.rawBenchmarks || [];
        const canStream = supportsStreaming(lib.name);

        // Skip non-streaming libraries for stream mode
        if (isStreamMode && !canStream) return;

        // Filter versions based on dev toggle and sort by time
        const versions = libData.aggregations
            .filter(agg => {
                if (!agg.results[opKey]) return false;
                if (!showDevVersions && agg.dev === true) return false;
                return true;
            })
            .sort((a, b) => (a.first || 0) - (b.first || 0));

        const totalVersions = versions.length;

        versions.forEach((agg, versionIndex) => {
            const result = agg.results[opKey];
            if (!result) return;

            const dataPoints = generateDailyPoints(agg, result, opKey, rawBenchmarks);
            if (dataPoints.length === 0) return;

            let color = getVersionColor(lib.baseColor, versionIndex, totalVersions);

            // Use lighter shade for streaming when showing both
            if (isStreamMode && showBoth) {
                color = [
                    Math.min(255, color[0] + 40),
                    Math.min(255, color[1] + 40),
                    Math.min(255, color[2] + 40)
                ];
            }

            const isDev = agg.dev === true;
            const modeLabel = showBoth ? (isStreamMode ? ' (Str)' : ' (Buf)') : '';

            datasets.push({
                label: `${libData.displayName}${modeLabel} ${agg.version}`,
                data: dataPoints,
                borderColor: rgba(color, 1),
                backgroundColor: rgba(color, 0.3),
                fill: false,
                tension: 0,
                pointRadius: 4,
                pointHoverRadius: 7,
                borderWidth: 2,
                borderDash: isDev ? [5, 5] : [],
                isDev: isDev,
                isStream: isStreamMode
            });
        });
    }

    LIBRARIES.forEach((lib) => {
        if (!selectedLibraries.has(lib.name)) return;

        const libData = aggregatedData[lib.name];
        if (!libData || !libData.aggregations) return;

        // Add buffer datasets
        if (showBuffer) {
            addDatasetsForOperation(lib, libData, bufferKey, false);
        }

        // Add stream datasets
        if (showStream) {
            addDatasetsForOperation(lib, libData, streamKey, true);
        }
    });

    return { datasets };
}

// Update timeline chart
function updateTimelineChart() {
    const ctx = document.getElementById('timeline-chart').getContext('2d');

    if (timelineChart) {
        timelineChart.destroy();
    }

    const data = getTimelineData();

    timelineChart = new Chart(ctx, {
        type: 'line',
        data: data,
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    display: false
                },
                tooltip: {
                    callbacks: {
                        title: function(context) {
                            const point = context[0].raw;
                            return new Date(point.x).toLocaleString();
                        },
                        label: function(context) {
                            const point = context.raw;
                            const sourceLabel = point.isRaw ? '(exact)' : '(aggregated)';
                            const lines = [
                                context.dataset.label + (point.isDev ? ' (dev)' : ''),
                                'Value: ' + formatNumber(point.y, currentTimelineMetric) + ' ' + sourceLabel,
                                'Min: ' + formatNumber(point.min, currentTimelineMetric),
                                'Max: ' + formatNumber(point.max, currentTimelineMetric),
                                'Samples: ' + point.samples
                            ];
                            return lines;
                        }
                    }
                }
            },
            scales: {
                x: {
                    type: 'time',
                    time: {
                        displayFormats: {
                            hour: 'MMM d, HH:mm',
                            day: 'MMM d',
                            week: 'MMM d',
                            month: 'MMM yyyy'
                        }
                    },
                    title: {
                        display: true,
                        text: 'Time'
                    }
                },
                y: {
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: currentTimelineMetric === 'time' ? 'Time (ns/op)' :
                              currentTimelineMetric === 'memory' ? 'Memory (bytes/op)' : 'Allocations'
                    },
                    ticks: {
                        callback: function(value) {
                            return formatNumber(value, currentTimelineMetric);
                        }
                    }
                }
            },
            interaction: {
                intersect: false,
                mode: 'nearest'
            }
        }
    });
}

// Update all charts
function updateAllCharts() {
    updateMetadataDisplay();
    updateBarCharts();
    updateTimelineChart();
}

// Initialize tabs
function initTabs() {
    // Preset tabs (Mainnet/Minimal)
    const presetBtns = document.querySelectorAll('.tab-btn[data-tab]');
    presetBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            presetBtns.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            currentTab = btn.dataset.tab;
            updateAllCharts();
        });
    });

    // Data type tabs (Block/State)
    const typeBtns = document.querySelectorAll('.tab-btn[data-type]');
    typeBtns.forEach(btn => {
        btn.addEventListener('click', () => {
            typeBtns.forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            currentType = btn.dataset.type;
            updateAllCharts();
        });
    });
}

// Initialize selectors
function initSelectors() {
    document.getElementById('timeline-range').addEventListener('change', (e) => {
        currentTimelineRange = e.target.value;
        updateTimelineChart();
    });

    document.getElementById('timeline-operation').addEventListener('change', (e) => {
        currentTimelineOp = e.target.value;
        updateTimelineChart();
    });

    document.getElementById('timeline-metric').addEventListener('change', (e) => {
        currentTimelineMetric = e.target.value;
        updateTimelineChart();
    });

    document.getElementById('show-dev-versions').addEventListener('change', (e) => {
        showDevVersions = e.target.checked;
        updateTimelineChart();
    });
}

// Initialize library multiselect dropdown
function initLibrarySelector() {
    const container = document.getElementById('library-multiselect');
    const btn = document.getElementById('library-btn');
    const dropdown = document.getElementById('library-dropdown');

    // Build dropdown items
    LIBRARIES.forEach(lib => {
        const item = document.createElement('div');
        item.className = 'multiselect-item';

        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.id = `lib-${lib.name}`;
        checkbox.checked = selectedLibraries.has(lib.name);

        const colorDot = document.createElement('span');
        colorDot.className = 'color-dot';
        colorDot.style.backgroundColor = `rgb(${lib.baseColor.join(',')})`;

        const label = document.createElement('label');
        label.htmlFor = `lib-${lib.name}`;
        label.textContent = lib.displayName;

        item.appendChild(checkbox);
        item.appendChild(colorDot);
        item.appendChild(label);

        item.addEventListener('click', (e) => {
            if (e.target !== checkbox) {
                checkbox.checked = !checkbox.checked;
            }
            if (checkbox.checked) {
                selectedLibraries.add(lib.name);
            } else {
                selectedLibraries.delete(lib.name);
            }
            updateLibraryButtonText();
            updateAllCharts();
        });

        dropdown.appendChild(item);
    });

    // Toggle dropdown
    btn.addEventListener('click', (e) => {
        e.stopPropagation();
        container.classList.toggle('open');
    });

    // Close dropdown when clicking outside
    document.addEventListener('click', (e) => {
        if (!container.contains(e.target)) {
            container.classList.remove('open');
        }
    });

    updateLibraryButtonText();
}

// Update the button text based on selection
function updateLibraryButtonText() {
    const btn = document.getElementById('library-btn');
    const count = selectedLibraries.size;
    const total = LIBRARIES.length;

    if (count === 0) {
        btn.textContent = 'None selected';
    } else if (count === total) {
        btn.textContent = 'All Libraries';
    } else if (count <= 2) {
        const names = LIBRARIES
            .filter(l => selectedLibraries.has(l.name))
            .map(l => l.displayName)
            .join(', ');
        btn.textContent = names;
    } else {
        btn.textContent = `${count} of ${total} selected`;
    }
}

// Initialize mode multiselect dropdown
function initModeSelector() {
    const container = document.getElementById('mode-multiselect');
    const btn = document.getElementById('mode-btn');
    const dropdown = document.getElementById('mode-dropdown');

    // Build dropdown items
    MODES.forEach(mode => {
        const item = document.createElement('div');
        item.className = 'multiselect-item';

        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.id = `mode-${mode.name}`;
        checkbox.checked = selectedModes.has(mode.name);

        const label = document.createElement('label');
        label.htmlFor = `mode-${mode.name}`;
        label.textContent = mode.displayName;
        label.title = mode.description;

        item.appendChild(checkbox);
        item.appendChild(label);

        item.addEventListener('click', (e) => {
            if (e.target !== checkbox) {
                checkbox.checked = !checkbox.checked;
            }
            if (checkbox.checked) {
                selectedModes.add(mode.name);
            } else {
                selectedModes.delete(mode.name);
            }
            // Ensure at least one mode is selected
            if (selectedModes.size === 0) {
                selectedModes.add('buffer');
                document.getElementById('mode-buffer').checked = true;
            }
            updateModeButtonText();
            updateAllCharts();
        });

        dropdown.appendChild(item);
    });

    // Toggle dropdown
    btn.addEventListener('click', (e) => {
        e.stopPropagation();
        container.classList.toggle('open');
    });

    // Close dropdown when clicking outside
    document.addEventListener('click', (e) => {
        if (!container.contains(e.target)) {
            container.classList.remove('open');
        }
    });

    updateModeButtonText();
}

// Update the mode button text based on selection
function updateModeButtonText() {
    const btn = document.getElementById('mode-btn');
    const count = selectedModes.size;

    if (count === MODES.length) {
        btn.textContent = 'Buffer & Stream';
    } else if (selectedModes.has('buffer')) {
        btn.textContent = 'Buffer';
    } else if (selectedModes.has('stream')) {
        btn.textContent = 'Stream';
    } else {
        btn.textContent = 'Buffer'; // fallback
    }
}

// Main initialization
async function init() {
    try {
        aggregatedData = await loadData();

        if (Object.keys(aggregatedData).length === 0) {
            throw new Error('No data loaded');
        }

        initTabs();
        initSelectors();
        initLibrarySelector();
        initModeSelector();
        updateAllCharts();
    } catch (error) {
        console.error('Failed to initialize:', error);
        document.querySelector('main').innerHTML =
            '<div class="error">Failed to load benchmark data. Please ensure the results files are available.</div>';
    }
}

// Start when DOM is ready
document.addEventListener('DOMContentLoaded', init);
