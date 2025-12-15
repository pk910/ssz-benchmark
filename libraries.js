/**
 * SSZ Benchmark - Library Configuration
 * Centralized configuration for all SSZ libraries used across the benchmark visualizations.
 */

const LIBRARIES = [
    {
        name: 'fastssz-v1',
        displayName: 'FastSSZ v1',
        shortName: 'Fast v1',
        baseColor: [37, 99, 235],       // Blue - for web pages
        svgColor: [96, 165, 250]        // Lighter blue - for SVG dark mode
    },
    {
        name: 'fastssz-v2',
        displayName: 'FastSSZ v2',
        shortName: 'Fast v2',
        baseColor: [59, 130, 246],      // Lighter Blue
        svgColor: [147, 197, 253]
    },
    {
        name: 'dynamicssz-codegen',
        displayName: 'DynamicSSZ Codegen',
        shortName: 'Dyn Code',
        baseColor: [34, 197, 94],       // Green
        svgColor: [74, 222, 128]
    },
    {
        name: 'dynamicssz-reflection',
        displayName: 'DynamicSSZ Reflection',
        shortName: 'Dyn Refl',
        baseColor: [74, 222, 128],      // Lighter Green
        svgColor: [134, 239, 172]
    },
    {
        name: 'karalabessz',
        displayName: 'Karalabe SSZ',
        shortName: 'Karalabe',
        baseColor: [249, 115, 22],      // Orange
        svgColor: [251, 146, 60]
    },
    {
        name: 'ztyp',
        displayName: 'ZTYP',
        shortName: 'ZTYP',
        baseColor: [168, 85, 247],      // Purple
        svgColor: [192, 132, 252]
    }
];

const PayloadTypes = [
    {
        name: 'Block',
        fork: 'Deneb',
        presets: [
            {
                name: 'Mainnet',
                size: 129952,
            },
            {
                name: 'Minimal',
                size: 130124,
            }
        ]
    },
    {
        name: 'State',
        fork: 'Deneb',
        presets: [
            {
                name: 'Mainnet',
                size: 16784725,
            },
            {
                name: 'Minimal',
                size: 13913173,
            }
        ]
    }
];

// Helper to get file names from library name
function getLibraryFiles(lib) {
    return {
        aggregationFile: `${lib.name}-aggregation.json`,
        rawFile: `${lib.name}.json`
    };
}

// Export for Node.js (svg generators)
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { LIBRARIES, PayloadTypes, getLibraryFiles };
}
