/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./index.html",
    "./src/renderer/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        canvas: {
          light: '#FBFBFA',
          dark: '#121413',
        },
        surface: {
          light: '#FFFFFF',
          dark: '#1A1D1B',
        },
        surfaceSubtle: {
          light: '#F4F4F1',
          dark: '#151716',
        },
        border: {
          light: '#E5E5E0',
          dark: '#272B28',
        },
        primary: {
          light: '#1A1D1A',
          dark: '#EDEDEB',
        },
        secondary: {
          light: '#656A65',
          dark: '#8C928C',
        },
        forest: {
          DEFAULT: '#1E4D2B',
          hover: '#265F36',
          dark: '#388E52',
          lightTint: '#EDF5EF',
          darkTint: '#16281C',
        },
      },
      fontFamily: {
        sans: [
          '-apple-system',
          'BlinkMacSystemFont',
          '"SF Pro Text"',
          '"SF Pro Display"',
          'system-ui',
          'sans-serif',
        ],
        mono: [
          '"SF Mono"',
          'Menlo',
          'Monaco',
          'Consolas',
          'monospace',
        ],
      },
      boxShadow: {
        subtle: '0 1px 2px 0 rgba(0, 0, 0, 0.04)',
        card: '0 1px 3px 0 rgba(0, 0, 0, 0.05), 0 1px 2px -1px rgba(0, 0, 0, 0.05)',
        dropdown: '0 10px 15px -3px rgba(0, 0, 0, 0.08), 0 4px 6px -4px rgba(0, 0, 0, 0.05)',
      },
    },
  },
  plugins: [],
};
