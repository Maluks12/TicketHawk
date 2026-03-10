/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './app/**/*.{js,jsx,ts,tsx}',
    './components/**/*.{js,jsx,ts,tsx}',
    './screens/**/*.{js,jsx,ts,tsx}',
  ],
  presets: [require('nativewind/preset')],
  theme: {
    extend: {
      colors: {
        // Match your Go backend's category colour scheme
        primary:  '#4F46E5',
        accent:   '#B46EF5',
        surface:  '#0A0A0A',
        card:     '#1A1A1A',
        border:   '#2A2A2A',
      },
    },
  },
  plugins: [],
};

