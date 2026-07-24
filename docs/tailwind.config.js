/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./index.html', './guide.html', './app.js'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // GNOME Adwaita blues. `gnome` is the text-safe accent (4.6:1 on
        // white); `gnome-bright` is the classic Blue 3 for large/bold uses.
        gnome: '#1c71d8',
        'gnome-bright': '#3584e4',
        'gnome-dark': '#1a5fb4',
        'gnome-light': '#62a0ea',
        // Neutral gray ramp (no blue cast) aligned with the CSS tokens in
        // styles.css: 200 = light border, 700 = dark border, 800 = dark
        // surface, 900 = dark background.
        slate: {
          50: '#fafafa',
          100: '#f2f2f4',
          200: '#e6e6ea',
          300: '#d0d0d6',
          400: '#9b9ba3',
          500: '#6e6e76',
          600: '#57575f',
          700: '#3a3a41',
          800: '#28282d',
          900: '#1e1e22',
          950: '#141417',
        }
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
      animation: {
        'fade-in': 'fadeIn 0.5s ease-out forwards',
        'slide-up': 'slideUp 0.5s cubic-bezier(0.22, 1, 0.36, 1) forwards',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(16px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
      },
    },
  },
}
