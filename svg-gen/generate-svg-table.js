#!/usr/bin/env node
/**
 * SSZ Benchmark SVG Table Generator
 * Generates a compact table optimized for GitHub README (light and dark mode)
 * Layout: Libraries as rows, Operations as columns
 */

const fs = require('fs');
const path = require('path');
const { LIBRARIES, PayloadTypes, getLibraryFiles } = require('../libraries.js');

const OPERATIONS = ['Unmarshal', 'Marshal', 'HashTreeRoot'];
const TYPES = ['Block', 'State'];

// Color schemes for light and dark modes
const COLOR_SCHEMES = {
    dark: {
        background: '#0d1117',
        title: 'rgba(248, 250, 252, 0.95)',
        subtitle: 'rgba(148, 163, 184, 0.8)',
        headerBg: 'rgba(30, 41, 59, 0.9)',
        headerText: 'rgba(248, 250, 252, 0.9)',
        subHeaderText: 'rgba(148, 163, 184, 0.7)',
        typeBg: 'rgba(51, 65, 85, 0.7)',
        typeText: 'rgba(248, 250, 252, 0.85)',
        rowEvenBg: 'rgba(30, 41, 59, 0.5)',
        rowOddBg: 'rgba(30, 41, 59, 0.3)',
        libraryName: 'rgba(248, 250, 252, 0.9)',
        versionText: 'rgba(148, 163, 184, 0.7)',
        valueText: 'rgba(203, 213, 225, 0.9)',
        bestTimeIndicator: 'rgba(74, 222, 128, 0.6)',
        bestTimeText: 'rgba(134, 239, 172, 1)',
        bestMemIndicator: 'rgba(96, 165, 250, 0.6)',
        bestMemText: 'rgba(147, 197, 253, 1)',
        emptyText: 'rgba(100, 116, 139, 0.6)',
        divider: 'rgba(148, 163, 184, 0.25)',
        dividerLight: 'rgba(148, 163, 184, 0.15)'
    },
    light: {
        background: '#ffffff',
        title: 'rgba(15, 23, 42, 0.95)',
        subtitle: 'rgba(71, 85, 105, 0.9)',
        headerBg: 'rgba(241, 245, 249, 1)',
        headerText: 'rgba(15, 23, 42, 0.9)',
        subHeaderText: 'rgba(71, 85, 105, 0.8)',
        typeBg: 'rgba(226, 232, 240, 1)',
        typeText: 'rgba(15, 23, 42, 0.85)',
        rowEvenBg: 'rgba(248, 250, 252, 1)',
        rowOddBg: 'rgba(241, 245, 249, 1)',
        libraryName: 'rgba(15, 23, 42, 0.9)',
        versionText: 'rgba(71, 85, 105, 0.8)',
        valueText: 'rgba(51, 65, 85, 0.9)',
        bestTimeIndicator: 'rgba(34, 197, 94, 0.7)',
        bestTimeText: 'rgba(22, 163, 74, 1)',
        bestMemIndicator: 'rgba(59, 130, 246, 0.7)',
        bestMemText: 'rgba(37, 99, 235, 1)',
        emptyText: 'rgba(148, 163, 184, 0.7)',
        divider: 'rgba(71, 85, 105, 0.2)',
        dividerLight: 'rgba(71, 85, 105, 0.1)'
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

function rgba(color, alpha) {
    return `rgba(${color[0]}, ${color[1]}, ${color[2]}, ${alpha})`;
}

function loadAggregationData(resultsDir) {
    const data = {};
    for (const lib of LIBRARIES) {
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

function generateTableSvg(data, colorScheme) {
    const colors = COLOR_SCHEMES[colorScheme];
    const libraries = Object.values(data);
    const numLibs = libraries.length;
    const numOps = OPERATIONS.length;

    // Dimensions scaled for 830px GitHub README width
    const libColWidth = 230;
    const opColWidth = 95;
    const cellHeight = 38;
    const headerHeight = 44;
    const typeHeaderHeight = 28;
    const subHeaderHeight = 18;
    const padding = 15;
    const sectionGap = 12;

    // Calculate dimensions
    const tableWidth = libColWidth + numOps * opColWidth * 2; // *2 for time+memory
    const rowsPerSection = numLibs;
    const sectionHeight = typeHeaderHeight + subHeaderHeight + rowsPerSection * cellHeight;
    const tableHeight = headerHeight + TYPES.length * sectionHeight + (TYPES.length - 1) * sectionGap;

    const totalWidth = tableWidth + padding * 2;
    const totalHeight = tableHeight + padding * 2 + 50; // Extra for title

    // Find best values per operation/type
    function getBestValues(operation, type) {
        const key = `${operation}Mainnet${type}`;
        let bestTime = Infinity, bestMem = Infinity;
        for (const lib of libraries) {
            if (lib.results && lib.results[key]) {
                bestTime = Math.min(bestTime, lib.results[key].ns_op[0]);
                bestMem = Math.min(bestMem, lib.results[key].bytes[0]);
            }
        }
        return { bestTime, bestMem };
    }

    let svg = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${totalWidth}" height="${totalHeight}" viewBox="0 0 ${totalWidth} ${totalHeight}">
  <rect width="100%" height="100%" fill="${colors.background}"/>
`;

    // Title
    const now = new Date();
    const generatedDate = now.toISOString().slice(0, 16).replace('T', ' ');
    svg += `<text x="${totalWidth / 2}" y="24" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="18" font-weight="700" fill="${colors.title}">SSZ Benchmark Results</text>`;
    svg += `<text x="${totalWidth / 2}" y="44" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="11" fill="${colors.subtitle}">Mainnet • Latest Stable • Lower is Better • ● = Best • Generated: ${generatedDate}</text>`;

    const tableX = padding;
    const tableY = 56;

    // Header row - Operation names
    svg += `<rect x="${tableX}" y="${tableY}" width="${libColWidth}" height="${headerHeight}" fill="${colors.headerBg}" rx="6 0 0 0"/>`;
    svg += `<text x="${tableX + libColWidth / 2}" y="${tableY + headerHeight / 2 + 5}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="13" font-weight="600" fill="${colors.headerText}">Library</text>`;

    OPERATIONS.forEach((op, opIdx) => {
        const x = tableX + libColWidth + opIdx * opColWidth * 2;
        const isLast = opIdx === OPERATIONS.length - 1;
        svg += `<rect x="${x}" y="${tableY}" width="${opColWidth * 2}" height="${headerHeight}" fill="${colors.headerBg}" ${isLast ? 'rx="0 6 0 0"' : ''}/>`;
        svg += `<text x="${x + opColWidth}" y="${tableY + 18}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="12" font-weight="600" fill="${colors.headerText}">${op === 'HashTreeRoot' ? 'HTR' : op}</text>`;
        // Sub-headers for Time/Memory
        svg += `<text x="${x + opColWidth / 2}" y="${tableY + 36}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="10" fill="${colors.subHeaderText}">Time</text>`;
        svg += `<text x="${x + opColWidth * 1.5}" y="${tableY + 36}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="10" fill="${colors.subHeaderText}">Mem</text>`;
        // Operation divider (between operations, not between time/mem)
        if (opIdx > 0) {
            svg += `<line x1="${x}" y1="${tableY}" x2="${x}" y2="${tableY + headerHeight}" stroke="${colors.divider}"/>`;
        }
    });

    let currentY = tableY + headerHeight;

    TYPES.forEach((type, typeIdx) => {
        if (typeIdx > 0) currentY += sectionGap;

        // Type section header with metadata (no dividers on type row)
        const metadata = getPayloadTypeMetadata(type, 'Mainnet');
        const metaText = metadata ? ` (${metadata.fork} · ${formatMemory(metadata.size)})` : '';
        svg += `<rect x="${tableX}" y="${currentY}" width="${tableWidth}" height="${typeHeaderHeight}" fill="${colors.typeBg}"/>`;
        svg += `<text x="${tableX + tableWidth / 2}" y="${currentY + typeHeaderHeight / 2 + 4}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="12" font-weight="600" fill="${colors.typeText}">${type}<tspan font-weight="400" font-size="10" fill="${colors.subHeaderText}">${metaText}</tspan></text>`;
        currentY += typeHeaderHeight;

        // Library rows
        libraries.forEach((lib, libIdx) => {
            const rowY = currentY + libIdx * cellHeight;
            const isEven = libIdx % 2 === 0;
            const isLast = libIdx === libraries.length - 1 && typeIdx === TYPES.length - 1;

            // Row background
            svg += `<rect x="${tableX}" y="${rowY}" width="${tableWidth}" height="${cellHeight}" fill="${isEven ? colors.rowEvenBg : colors.rowOddBg}" ${isLast ? 'rx="0 0 4 4"' : ''}/>`;

            // Library name cell with color indicator and version
            svg += `<rect x="${tableX + 6}" y="${rowY + 8}" width="10" height="20" fill="${rgba(lib.baseColor, 0.9)}" rx="2"/>`;
            svg += `<text x="${tableX + 22}" y="${rowY + 16}" font-family="system-ui, -apple-system, sans-serif" font-size="11" fill="${colors.libraryName}">${lib.displayName}</text>`;
            svg += `<text x="${tableX + 22}" y="${rowY + 28}" font-family="system-ui, -apple-system, sans-serif" font-size="8" fill="${colors.versionText}">${lib.version}</text>`;

            // Operation cells
            OPERATIONS.forEach((op, opIdx) => {
                const key = `${op}Mainnet${type}`;
                const cellX = tableX + libColWidth + opIdx * opColWidth * 2;
                const result = lib.results && lib.results[key];
                const { bestTime, bestMem } = getBestValues(op, type);

                if (result) {
                    const timeVal = result.ns_op[0];
                    const memVal = result.bytes[0];
                    const isBestTime = (timeVal - bestTime) <= 2000; // Within 2µs
                    const isBestMem = Math.abs(memVal - bestMem) < 0.01;

                    // Time value
                    if (isBestTime) {
                        svg += `<circle cx="${cellX + opColWidth / 2 - 22}" cy="${rowY + cellHeight / 2}" r="4" fill="${colors.bestTimeIndicator}"/>`;
                    }
                    svg += `<text x="${cellX + opColWidth / 2}" y="${rowY + cellHeight / 2 + 4}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="11" font-weight="${isBestTime ? '600' : '400'}" fill="${isBestTime ? colors.bestTimeText : colors.valueText}">${formatTime(timeVal)}</text>`;

                    // Memory value
                    if (isBestMem) {
                        svg += `<circle cx="${cellX + opColWidth * 1.5 - 22}" cy="${rowY + cellHeight / 2}" r="4" fill="${colors.bestMemIndicator}"/>`;
                    }
                    svg += `<text x="${cellX + opColWidth * 1.5}" y="${rowY + cellHeight / 2 + 4}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="11" font-weight="${isBestMem ? '600' : '400'}" fill="${isBestMem ? colors.bestMemText : colors.valueText}">${formatMemory(memVal)}</text>`;
                } else {
                    svg += `<text x="${cellX + opColWidth}" y="${rowY + cellHeight / 2 + 4}" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="11" fill="${colors.emptyText}">—</text>`;
                }

                // Operation divider (between operations, not between time/mem)
                if (opIdx > 0) {
                    svg += `<line x1="${cellX}" y1="${rowY}" x2="${cellX}" y2="${rowY + cellHeight}" stroke="${colors.dividerLight}"/>`;
                }
            });
        });

        currentY += libraries.length * cellHeight;
    });

    svg += `</svg>`;
    return svg;
}

function main() {
    const resultsDir = path.join(__dirname, '..', 'results');
    const baseOutputPath = process.argv[2] || path.join(__dirname, '..', 'benchmark-table.svg');

    console.log('Loading aggregation data...');
    const data = loadAggregationData(resultsDir);

    const libraryCount = Object.keys(data).length;
    if (libraryCount === 0) {
        console.error('No aggregation data found!');
        process.exit(1);
    }

    console.log(`Found ${libraryCount} libraries with benchmark data`);
    for (const [name, info] of Object.entries(data)) {
        console.log(`  - ${info.displayName}: ${info.version}`);
    }

    // Generate dark mode version
    console.log('Generating dark mode SVG table...');
    const darkSvg = generateTableSvg(data, 'dark');
    const darkOutputPath = baseOutputPath.replace('.svg', '.svg');
    fs.writeFileSync(darkOutputPath, darkSvg);
    console.log(`Dark mode table saved to: ${darkOutputPath}`);

    // Generate light mode version
    console.log('Generating light mode SVG table...');
    const lightSvg = generateTableSvg(data, 'light');
    const lightOutputPath = baseOutputPath.replace('.svg', '-light.svg');
    fs.writeFileSync(lightOutputPath, lightSvg);
    console.log(`Light mode table saved to: ${lightOutputPath}`);
}

main();
