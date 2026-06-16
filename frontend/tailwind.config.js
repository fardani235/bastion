/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: '#0b0f14',
        surface: '#121821',
        'surface-2': '#1a2230',
        border: '#1f2937',
        text: '#e5e7eb',
        muted: '#94a3b8',
        accent: '#5eead4',
        'accent-dim': '#2dd4bf',
        danger: '#f87171',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'monospace'],
      },
    },
  },
  plugins: [],
}
