#!/usr/bin/env node
/**
 * SSZ Benchmark SVG Streaming Chart Generator
 * Generates static SVG images for streaming benchmarks (MarshalWriter, UnmarshalReader)
 * Optimized for GitHub README (light and dark mode)
 */

const fs = require('fs');
const path = require('path');
const { LIBRARIES, PayloadTypes, getLibraryFiles } = require('../libraries.js');

// Streaming operations only
const OPERATIONS = ['UnmarshalReader', 'MarshalWriter'];
const OPERATION_LABELS = {
    'UnmarshalReader': 'Unmarshal (Stream)',
    'MarshalWriter': 'Marshal (Stream)'
};
const TYPES = ['Block', 'State'];
const METRICS = [
    { key: 'time', label: 'Time', field: 'ns_op', format: formatTime },
    { key: 'memory', label: 'Memory', field: 'bytes', format: formatMemory }
];

// Libraries that support streaming
const STREAMING_LIBRARIES = ['dynamicssz-codegen', 'dynamicssz-reflection', 'karalabessz'];

// Dimensions scaled for GitHub README width
const CHART_WIDTH = 252;
const CHART_HEIGHT = 210;
const BAR_WIDTH = 36; // Wider bars since fewer libraries
const CHART_PADDING = { top: 36, right: 12, bottom: 72, left: 12 };
const CHART_GAP = 10;
const ROW_GAP = 12;

// Label dimensions for rotated text
const TYPE_LABEL_WIDTH = 24;    // Block/State (spans 2 rows)
const METRIC_LABEL_WIDTH = 28;  // Time/Memory

// Color schemes for light and dark modes
const COLOR_SCHEMES = {
    dark: {
        background: '#0d1117',
        chartBg: 'rgba(30, 41, 59, 0.8)',
        chartBorder: 'rgba(148, 163, 184, 0.2)',
        title: 'rgba(248, 250, 252, 0.95)',
        subtitle: 'rgba(148, 163, 184, 0.8)',
        chartTitle: 'rgba(248, 250, 252, 0.95)',
        rowLabel: 'rgba(248, 250, 252, 0.9)',
        rowSublabel: 'rgba(148, 163, 184, 0.7)',
        valueText: 'rgba(248, 250, 252, 0.9)',
        versionText: 'rgba(148, 163, 184, 0.7)',
        glowColor: 0.5
    },
    light: {
        background: '#ffffff',
        chartBg: 'rgba(241, 245, 249, 1)',
        chartBorder: 'rgba(71, 85, 105, 0.2)',
        title: 'rgba(15, 23, 42, 0.95)',
        subtitle: 'rgba(71, 85, 105, 0.9)',
        chartTitle: 'rgba(15, 23, 42, 0.95)',
        rowLabel: 'rgba(15, 23, 42, 0.9)',
        rowSublabel: 'rgba(71, 85, 105, 0.8)',
        valueText: 'rgba(15, 23, 42, 0.9)',
        versionText: 'rgba(71, 85, 105, 0.8)',
        glowColor: 0.7
    }
};

// Check if version is a Go pseudo-version (v0.0.0-TIMESTAMP-HASH)
function isPseudoVersion(version) {
    return /^v0\.0\.0-\d{14}-[a-f0-9]+$/.test(version);
}

// Extract timestamp from Go pseudo-version for comparison
function getPseudoVersionTimestamp(version) {
    const match = version.match(/^v0\.0\.0-(\d{14})-[a-f0-9]+$/);
    return match ? match[1] : null;
}

// Format version for display (truncate pseudo-versions)
function formatVersion(version) {
    if (!version) return '';
    // Truncate pseudo-versions: v0.0.0-TIMESTAMP-HASH -> v0.0.0-HASH[0:6]
    const match = version.match(/^(v\d+\.\d+\.\d+)-\d{14}-([a-f0-9]+)$/);
    if (match) {
        return `${match[1]}-${match[2].substring(0, 6)}`;
    }
    return version;
}

function parseSemver(version) {
    // Skip pseudo-versions for semver parsing
    if (isPseudoVersion(version)) return null;
    const match = version.match(/^v?(\d+)\.(\d+)\.(\d+)(?:-(.+))?$/);
    if (!match) return null;
    return {
        major: parseInt(match[1], 10),
        minor: parseInt(match[2], 10),
        patch: parseInt(match[3], 10),
        prerelease: match[4] || null
    };
}

function compareSemver(a, b) {
    if (!a && !b) return 0;
    if (!a) return -1;
    if (!b) return 1;
    if (a.major !== b.major) return a.major - b.major;
    if (a.minor !== b.minor) return a.minor - b.minor;
    if (a.patch !== b.patch) return a.patch - b.patch;
    if (!a.prerelease && b.prerelease) return 1;
    if (a.prerelease && !b.prerelease) return -1;
    return 0;
}

function getLatestStableVersion(aggregations) {
    // Filter out dev versions (indicated by dev: true property)
    const stableVersions = aggregations.filter(agg => agg.dev !== true);
    if (stableVersions.length === 0) return null;

    // First, try to find the latest real semver (non-pseudo) version
    let latest = null;
    let latestSemver = null;
    for (const agg of stableVersions) {
        if (isPseudoVersion(agg.version)) continue;
        const semver = parseSemver(agg.version);
        if (compareSemver(semver, latestSemver) > 0) {
            latest = agg;
            latestSemver = semver;
        }
    }

    // If we found a real semver version, return it
    if (latest) return latest;

    // Otherwise, fall back to the latest pseudo-version by timestamp
    let latestTimestamp = null;
    for (const agg of stableVersions) {
        if (!isPseudoVersion(agg.version)) continue;
        const timestamp = getPseudoVersionTimestamp(agg.version);
        if (!latestTimestamp || timestamp > latestTimestamp) {
            latest = agg;
            latestTimestamp = timestamp;
        }
    }

    return latest;
}

function formatTime(ns) {
    if (ns >= 1e9) return (ns / 1e9).toFixed(1) + 's';
    if (ns >= 1e6) return (ns / 1e6).toFixed(1) + 'ms';
    if (ns >= 1e3) return (ns / 1e3).toFixed(0) + 'µs';
    return ns.toFixed(0) + 'ns';
}

function formatMemory(bytes) {
    if (bytes >= 1e9) return (bytes / 1e9).toFixed(1) + 'GB';
    if (bytes >= 1e6) return (bytes / 1e6).toFixed(1) + 'MB';
    if (bytes >= 1e3) return (bytes / 1e3).toFixed(0) + 'KB';
    return bytes.toFixed(0) + 'B';
}

function getPayloadTypeMetadata(typeName, presetName) {
    const payloadType = PayloadTypes.find(t => t.name === typeName);
    if (!payloadType) return null;
    const preset = payloadType.presets.find(p => p.name === presetName);
    return {
        fork: payloadType.fork,
        size: preset ? preset.size : null
    };
}

function rgb(color) {
    return `rgb(${color[0]}, ${color[1]}, ${color[2]})`;
}

function rgba(color, alpha) {
    return `rgba(${color[0]}, ${color[1]}, ${color[2]}, ${alpha})`;
}

function loadAggregationData(resultsDir) {
    const data = {};
    // Only load streaming libraries
    for (const lib of LIBRARIES) {
        if (!STREAMING_LIBRARIES.includes(lib.name)) continue;

        const files = getLibraryFiles(lib);
        const filePath = path.join(resultsDir, files.aggregationFile);
        if (fs.existsSync(filePath)) {
            try {
                const content = JSON.parse(fs.readFileSync(filePath, 'utf8'));
                const latestVersion = getLatestStableVersion(content.aggregations);
                if (latestVersion) {
                    data[lib.name] = {
                        ...lib,
                        baseColor: lib.svgColor, // Use SVG-optimized colors
                        version: latestVersion.version,
                        results: latestVersion.results
                    };
                }
            } catch (e) {
                console.warn(`Failed to load ${files.aggregationFile}: ${e.message}`);
            }
        }
    }
    return data;
}

// Generate a single vertical bar chart
function generateVerticalBarChart(title, data, formatFn, chartIndex, colors) {
    const innerWidth = CHART_WIDTH - CHART_PADDING.left - CHART_PADDING.right;
    const innerHeight = CHART_HEIGHT - CHART_PADDING.top - CHART_PADDING.bottom;

    const sortedData = [...data].sort((a, b) => a.value - b.value);
    const maxValue = Math.max(...sortedData.map(d => d.value));
    const barSpacing = innerWidth / data.length;
    const actualBarWidth = Math.min(BAR_WIDTH, barSpacing - 2);

    let svg = '';

    // Semi-transparent background
    svg += `<rect x="0" y="0" width="${CHART_WIDTH}" height="${CHART_HEIGHT}" fill="${colors.chartBg}" rx="8" stroke="${colors.chartBorder}"/>`;

    // Title
    svg += `<text x="${CHART_WIDTH / 2}" y="22" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="13" font-weight="600" fill="${colors.chartTitle}">${title}</text>`;

    // Find best (lowest) value
    const bestValue = sortedData[0]?.value;
    const labelAreaY = CHART_PADDING.top + innerHeight + 8;

    // Bars (in original order for consistent positioning)
    data.forEach((item, i) => {
        const x = CHART_PADDING.left + i * barSpacing + (barSpacing - actualBarWidth) / 2;
        const barHeight = maxValue > 0 ? (item.value / maxValue) * innerHeight : 0;
        const y = CHART_PADDING.top + innerHeight - barHeight;
        const barCenterX = x + actualBarWidth / 2;

        // Highlight if within 2µs (2000ns) of the best value
        const isBest = (item.value - bestValue) <= 2000;

        // Bar with glow effect for best
        if (isBest) {
            svg += `<rect x="${x - 1}" y="${y - 1}" width="${actualBarWidth + 2}" height="${barHeight + 2}" fill="none" stroke="${rgba(item.color, colors.glowColor)}" stroke-width="2" rx="2" filter="url(#glow-${chartIndex})"/>`;
        }
        svg += `<rect x="${x}" y="${y}" width="${actualBarWidth}" height="${Math.max(barHeight, 2)}" fill="${rgba(item.color, 0.85)}" rx="2"/>`;

        // Value on top of bar
        svg += `<text x="${barCenterX}" y="${y - 4}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="9" font-weight="500" fill="${colors.valueText}">${formatFn(item.value)}</text>`;

        // Library name and version at bottom (diagonal - rotated 55 degrees for readability)
        const labelX = barCenterX;
        const labelY = labelAreaY;
        svg += `<g transform="translate(${labelX}, ${labelY}) rotate(-55)">`;
        svg += `<text x="0" y="0" text-anchor="end" font-family="system-ui, -apple-system, sans-serif" font-size="12" font-weight="500" fill="${rgba(item.color, 0.95)}">${item.shortName}</text>`;
        svg += `<text x="0" y="13" text-anchor="end" font-family="system-ui, -apple-system, sans-serif" font-size="9" fill="${colors.versionText}">${item.version}</text>`;
        svg += `</g>`;
    });

    return svg;
}

function generateChartsSvg(data, colorScheme) {
    const colors = COLOR_SCHEMES[colorScheme];
    const charts = [];

    // Build chart data: for each type, for each metric, show streaming operations in a row
    for (const type of TYPES) {
        for (const metric of METRICS) {
            const rowCharts = [];
            for (const operation of OPERATIONS) {
                const chartData = [];
                for (const [libName, libData] of Object.entries(data)) {
                    const key = `${operation}Mainnet${type}`;
                    if (libData.results && libData.results[key]) {
                        const result = libData.results[key];
                        chartData.push({
                            name: libName,
                            shortName: libData.shortName,
                            version: formatVersion(libData.version),
                            color: libData.baseColor,
                            value: result[metric.field][0]
                        });
                    }
                }
                if (chartData.length > 0) {
                    rowCharts.push({
                        title: OPERATION_LABELS[operation] || operation,
                        data: chartData,
                        formatFn: metric.format
                    });
                }
            }
            if (rowCharts.length > 0) {
                charts.push({
                    type,
                    metric: metric.label,
                    charts: rowCharts
                });
            }
        }
    }

    // Layout: 2 charts per row (UnmarshalReader, MarshalWriter), 4 rows (Block Time, Block Memory, State Time, State Memory)
    const chartsPerRow = 2;
    const labelAreaWidth = TYPE_LABEL_WIDTH + METRIC_LABEL_WIDTH; // 52px total
    const rowWidth = chartsPerRow * CHART_WIDTH + (chartsPerRow - 1) * CHART_GAP;
    const totalWidth = rowWidth + labelAreaWidth; // Narrower than buffer charts
    const rowHeight = CHART_HEIGHT + ROW_GAP;
    const totalHeight = charts.length * rowHeight + 60; // Header space

    let svg = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${totalWidth}" height="${totalHeight}" viewBox="0 0 ${totalWidth} ${totalHeight}">
  <defs>
    <filter id="glow-0" x="-50%" y="-50%" width="200%" height="200%">
      <feGaussianBlur stdDeviation="2" result="blur"/>
      <feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>
    </filter>
  </defs>
  <rect width="100%" height="100%" fill="${colors.background}"/>
`;

    // Header
    const now = new Date();
    const generatedDate = now.toISOString().slice(0, 16).replace('T', ' ');
    svg += `<text x="${totalWidth / 2}" y="24" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="18" font-weight="700" fill="${colors.title}">SSZ Streaming Benchmark</text>`;
    svg += `<text x="${totalWidth / 2}" y="44" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="11" fill="${colors.subtitle}">Mainnet • Reader/Writer APIs • Lower is Better • Generated: ${generatedDate}</text>`;

    const startY = 60;
    const chartsStartX = labelAreaWidth;

    // Draw type labels (Block/State) with metadata - rotated 90°, centered between 2 rows each
    const typeLabelX = TYPE_LABEL_WIDTH / 2;
    TYPES.forEach((type, typeIdx) => {
        // Each type spans 2 rows (Time and Memory)
        const typeStartRow = typeIdx * 2;
        const typeY = startY + typeStartRow * rowHeight;
        const typeCenterY = typeY + rowHeight; // Center between 2 rows

        const metadata = getPayloadTypeMetadata(type, 'Mainnet');
        const metaText = metadata ? `${metadata.fork} · ${formatMemory(metadata.size)}` : '';

        svg += `<g transform="translate(${typeLabelX}, ${typeCenterY}) rotate(-90)">`;
        svg += `<text x="0" y="-6" text-anchor="middle" dominant-baseline="middle" font-family="system-ui, -apple-system, sans-serif" font-size="14" font-weight="600" fill="${colors.rowLabel}">${type}</text>`;
        svg += `<text x="0" y="10" text-anchor="middle" dominant-baseline="middle" font-family="system-ui, -apple-system, sans-serif" font-size="9" fill="${colors.rowSublabel}">${metaText}</text>`;
        svg += `</g>`;
    });

    // Draw rows with metric labels and charts
    charts.forEach((row, rowIdx) => {
        const y = startY + rowIdx * rowHeight;
        const rowCenterY = y + CHART_HEIGHT / 2;

        // Metric label (Time/Memory) - rotated 90°
        const metricLabelX = TYPE_LABEL_WIDTH + METRIC_LABEL_WIDTH / 2;
        svg += `<g transform="translate(${metricLabelX}, ${rowCenterY}) rotate(-90)">`;
        svg += `<text x="0" y="0" text-anchor="middle" dominant-baseline="middle" font-family="system-ui, -apple-system, sans-serif" font-size="11" fill="${colors.rowSublabel}">${row.metric}</text>`;
        svg += `</g>`;

        // Charts in row
        row.charts.forEach((chart, chartIdx) => {
            const x = chartsStartX + chartIdx * (CHART_WIDTH + CHART_GAP);
            svg += `<g transform="translate(${x}, ${y})">`;
            svg += generateVerticalBarChart(chart.title, chart.data, chart.formatFn, rowIdx * 2 + chartIdx, colors);
            svg += `</g>`;
        });
    });

    svg += `</svg>`;
    return svg;
}

function main() {
    const resultsDir = path.join(__dirname, '..', 'results');
    const baseOutputPath = process.argv[2] || path.join(__dirname, '..', 'benchmark-streaming.svg');

    console.log('Loading aggregation data for streaming libraries...');
    const data = loadAggregationData(resultsDir);

    const libraryCount = Object.keys(data).length;
    if (libraryCount === 0) {
        console.error('No streaming library data found!');
        process.exit(1);
    }

    console.log(`Found ${libraryCount} streaming libraries with benchmark data`);
    for (const [name, info] of Object.entries(data)) {
        console.log(`  - ${info.displayName}: ${info.version}`);
    }

    // Generate dark mode version
    console.log('Generating dark mode streaming SVG charts...');
    const darkSvg = generateChartsSvg(data, 'dark');
    const darkOutputPath = baseOutputPath.replace('.svg', '.svg');
    fs.writeFileSync(darkOutputPath, darkSvg);
    console.log(`Dark mode charts saved to: ${darkOutputPath}`);

    // Generate light mode version
    console.log('Generating light mode streaming SVG charts...');
    const lightSvg = generateChartsSvg(data, 'light');
    const lightOutputPath = baseOutputPath.replace('.svg', '-light.svg');
    fs.writeFileSync(lightOutputPath, lightSvg);
    console.log(`Light mode charts saved to: ${lightOutputPath}`);
}

main();
