// SSZ Benchmark Visualization - Details Page
// Uses LIBRARIES from libraries.js (loaded via script tag in HTML)

// Generate RGBA color string
function rgba(color, alpha) {
    return `rgba(${color[0]}, ${color[1]}, ${color[2]}, ${alpha})`;
}

// Generate lighter color variant for dev builds
function getDevColor(baseColor) {
    // Lighten the color for dev builds
    const factor = 0.6;
    return [
        Math.round(baseColor[0] + (255 - baseColor[0]) * (1 - factor)),
        Math.round(baseColor[1] + (255 - baseColor[1]) * (1 - factor)),
        Math.round(baseColor[2] + (255 - baseColor[2]) * (1 - factor))
    ];
}

let rawData = {};
let allOperations = new Set();
let currentLibrary = 'all';
let currentOperation = 'all';
let currentMetric = 'time';
let showDevBuilds = false;
let detailsChart = null;

const METRIC_CONFIG = {
    time: {
        index: 0,
        title: 'Time Over Benchmark Runs (ns/op)',
        axisLabel: 'Time (ns/op)',
        formatter: formatTimeValue
    },
    memory: {
        index: 1,
        title: 'Memory Over Benchmark Runs (bytes/op)',
        axisLabel: 'Memory (bytes/op)',
        formatter: formatBytes
    },
    allocs: {
        index: 2,
        title: 'Allocations Over Benchmark Runs',
        axisLabel: 'Allocations',
        formatter: formatNumber
    }
};

// Load all raw benchmark files
async function loadData() {
    const results = {};

    for (const lib of LIBRARIES) {
        const files = getLibraryFiles(lib);
        try {
            const response = await fetch(`results/${files.rawFile}`);
            if (response.ok) {
                const data = await response.json();
                results[lib.name] = {
                    displayName: lib.displayName,
                    baseColor: lib.baseColor,
                    data: data.benchmarks
                };

                // Collect all operations
                data.benchmarks.forEach(benchmark => {
                    Object.keys(benchmark.results).forEach(op => allOperations.add(op));
                });
            }
        } catch (error) {
            console.warn(`Failed to load ${files.rawFile}:`, error);
        }
    }

    return results;
}

// Filter benchmarks by dev status (for table display)
function filterBenchmarks(benchmarks, includeDevOverride = null) {
    const includeDev = includeDevOverride !== null ? includeDevOverride : showDevBuilds;
    if (includeDev) {
        return benchmarks;
    }
    return benchmarks.filter(b => !b.dev);
}

// Format timestamp
function formatTime(timestamp) {
    const date = new Date(timestamp * 1000);
    return date.toLocaleString();
}

// Format number
function formatNumber(num) {
    if (num === null || num === undefined) return 'N/A';

    if (num >= 1e9) return (num / 1e9).toFixed(2) + ' G';
    if (num >= 1e6) return (num / 1e6).toFixed(2) + ' M';
    if (num >= 1e3) return (num / 1e3).toFixed(2) + ' K';
    return num.toFixed(2);
}

// Format time value
function formatTimeValue(ns) {
    if (ns >= 1e9) return (ns / 1e9).toFixed(2) + ' s';
    if (ns >= 1e6) return (ns / 1e6).toFixed(2) + ' ms';
    if (ns >= 1e3) return (ns / 1e3).toFixed(2) + ' us';
    return ns.toFixed(2) + ' ns';
}

// Format bytes value
function formatBytes(bytes) {
    if (bytes >= 1e9) return (bytes / 1e9).toFixed(2) + ' GB';
    if (bytes >= 1e6) return (bytes / 1e6).toFixed(2) + ' MB';
    if (bytes >= 1e3) return (bytes / 1e3).toFixed(2) + ' KB';
    return bytes.toFixed(2) + ' B';
}

// Populate dropdowns
function populateDropdowns() {
    const librarySelect = document.getElementById('library-select');
    const operationSelect = document.getElementById('operation-select');

    // Add library options
    LIBRARIES.forEach(lib => {
        if (rawData[lib.name]) {
            const option = document.createElement('option');
            option.value = lib.name;
            option.textContent = lib.displayName;
            librarySelect.appendChild(option);
        }
    });

    // Add operation options (sorted)
    const sortedOps = Array.from(allOperations).sort();
    sortedOps.forEach(op => {
        const option = document.createElement('option');
        option.value = op;
        option.textContent = op;
        operationSelect.appendChild(option);
    });
}

// Get chart data for time series
function getTimeSeriesData() {
    const datasets = [];
    const libraries = currentLibrary === 'all'
        ? LIBRARIES.filter(l => rawData[l.name])
        : LIBRARIES.filter(l => l.name === currentLibrary && rawData[l.name]);

    const operations = currentOperation === 'all'
        ? Array.from(allOperations).sort()
        : [currentOperation];

    libraries.forEach((lib) => {
        const libData = rawData[lib.name];
        if (!libData) return;

        // Always get all data for chart - we separate dev/stable into different series
        const allData = libData.data;

        operations.forEach((op, opIndex) => {
            const stableDataPoints = [];
            const devDataPoints = [];
            const metricIndex = METRIC_CONFIG[currentMetric].index;

            allData.forEach((benchmark) => {
                const result = benchmark.results[op];
                if (result) {
                    const point = {
                        x: benchmark.time * 1000, // Convert to milliseconds for Chart.js time scale
                        y: result[metricIndex],
                        time: benchmark.time,
                        version: benchmark.version,
                        dev: benchmark.dev || false,
                        nsOp: result[0],
                        bytes: result[1],
                        alloc: result[2]
                    };

                    if (benchmark.dev) {
                        devDataPoints.push(point);
                    } else {
                        stableDataPoints.push(point);
                    }
                }
            });

            const stableColor = lib.baseColor;
            const devColor = getDevColor(lib.baseColor);

            // Add stable data series
            if (stableDataPoints.length > 0) {
                datasets.push({
                    label: `${lib.displayName} - ${op}`,
                    data: stableDataPoints,
                    borderColor: rgba(stableColor, 1),
                    backgroundColor: rgba(stableColor, 0.2),
                    fill: false,
                    tension: 0.1,
                    pointRadius: 5,
                    pointHoverRadius: 8
                });
            }

            // Add dev data series (dashed line, lighter color)
            if (devDataPoints.length > 0 && showDevBuilds) {
                datasets.push({
                    label: `${lib.displayName} - ${op} (dev)`,
                    data: devDataPoints,
                    borderColor: rgba(devColor, 1),
                    backgroundColor: rgba(devColor, 0.2),
                    borderDash: [5, 5],
                    fill: false,
                    tension: 0.1,
                    pointRadius: 5,
                    pointHoverRadius: 8,
                    pointStyle: 'triangle'
                });
            }
        });
    });

    return datasets;
}

// Update chart
function updateChart() {
    const config = METRIC_CONFIG[currentMetric];

    // Update chart title
    const chartTitle = document.querySelector('.details-chart-container h3');
    if (chartTitle) {
        chartTitle.textContent = config.title;
    }

    const ctx = document.getElementById('detailsTimeChart').getContext('2d');

    if (detailsChart) {
        detailsChart.destroy();
    }

    const datasets = getTimeSeriesData();

    if (datasets.length === 0) {
        document.querySelector('.details-chart-container').innerHTML =
            `<div class="chart-wrapper full-width"><h3>${config.title}</h3><p style="text-align: center; padding: 2rem; color: var(--text-muted);">No data available for the selected filters.</p></div>`;
        return;
    }

    detailsChart = new Chart(ctx, {
        type: 'line',
        data: { datasets },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    position: 'bottom',
                    labels: {
                        usePointStyle: true,
                        padding: 15
                    }
                },
                tooltip: {
                    callbacks: {
                        title: function(context) {
                            const point = context[0].raw;
                            return formatTime(point.time);
                        },
                        label: function(context) {
                            const point = context.raw;
                            const lines = [
                                context.dataset.label,
                                'Time: ' + formatTimeValue(point.nsOp),
                                'Memory: ' + formatBytes(point.bytes),
                                'Allocations: ' + formatNumber(point.alloc),
                                'Version: ' + point.version
                            ];
                            if (point.dev) {
                                lines.push('(Dev Build)');
                            }
                            return lines;
                        }
                    }
                }
            },
            scales: {
                x: {
                    type: 'time',
                    time: {
                        unit: 'day',
                        displayFormats: {
                            day: 'MMM d, yyyy'
                        },
                        tooltipFormat: 'MMM d, yyyy HH:mm'
                    },
                    title: {
                        display: true,
                        text: 'Date'
                    }
                },
                y: {
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: config.axisLabel
                    },
                    ticks: {
                        callback: function(value) {
                            return config.formatter(value);
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

// Update table with latest results
function updateTable() {
    const tbody = document.getElementById('results-body');
    tbody.innerHTML = '';

    const libraries = currentLibrary === 'all'
        ? LIBRARIES.filter(l => rawData[l.name])
        : LIBRARIES.filter(l => l.name === currentLibrary && rawData[l.name]);

    const operations = currentOperation === 'all'
        ? Array.from(allOperations).sort()
        : [currentOperation];

    libraries.forEach(lib => {
        const libData = rawData[lib.name];
        if (!libData || libData.data.length === 0) return;

        const filteredData = filterBenchmarks(libData.data);
        if (filteredData.length === 0) return;

        const latestBenchmark = filteredData[filteredData.length - 1];

        operations.forEach(op => {
            const result = latestBenchmark.results[op];
            if (!result) return;

            const row = document.createElement('tr');
            const versionDisplay = latestBenchmark.dev
                ? `${latestBenchmark.version} <span class="dev-badge">dev</span>`
                : latestBenchmark.version;
            row.innerHTML = `
                <td>${lib.displayName}</td>
                <td>${versionDisplay}</td>
                <td>${op}</td>
                <td class="number">${formatTimeValue(result[0])}</td>
                <td class="number">${formatBytes(result[1])}</td>
                <td class="number">${formatNumber(result[2])}</td>
                <td>${formatTime(latestBenchmark.time)}</td>
            `;
            tbody.appendChild(row);
        });
    });

    if (tbody.children.length === 0) {
        const row = document.createElement('tr');
        row.innerHTML = '<td colspan="7" style="text-align: center; color: var(--text-muted);">No data available for the selected filters.</td>';
        tbody.appendChild(row);
    }
}

// Initialize event listeners
function initEventListeners() {
    const librarySelect = document.getElementById('library-select');
    const operationSelect = document.getElementById('operation-select');
    const metricSelect = document.getElementById('metric-select');
    const showDevCheckbox = document.getElementById('show-dev-checkbox');

    librarySelect.addEventListener('change', () => {
        currentLibrary = librarySelect.value;
        updateChart();
        updateTable();
    });

    operationSelect.addEventListener('change', () => {
        currentOperation = operationSelect.value;
        updateChart();
        updateTable();
    });

    metricSelect.addEventListener('change', () => {
        currentMetric = metricSelect.value;
        updateChart();
    });

    showDevCheckbox.addEventListener('change', () => {
        showDevBuilds = showDevCheckbox.checked;
        updateChart();
        updateTable();
    });
}

// Main initialization
async function init() {
    try {
        rawData = await loadData();

        if (Object.keys(rawData).length === 0) {
            throw new Error('No data loaded');
        }

        populateDropdowns();
        initEventListeners();
        updateChart();
        updateTable();
    } catch (error) {
        console.error('Failed to initialize:', error);
        document.querySelector('main').innerHTML =
            '<div class="error">Failed to load benchmark data. Please ensure the results files are available.</div>';
    }
}

// Start when DOM is ready
document.addEventListener('DOMContentLoaded', init);
