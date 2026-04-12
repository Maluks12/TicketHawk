/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './app/**/*.{js,jsx,ts,tsx}',
    './components/**/*.{js,jsx,ts,tsx}',
    './screens/**/*.{js,jsx,ts,tsx}',
    './styles/**/*.{js,jsx,ts,tsx}',
  ],
  presets: [require('nativewind/preset')],
  theme: {
    extend: {
      colors: {
        // App surface colours
        primary:  '#4F46E5',
        accent:   '#B46EF5',
        surface:  '#0A0A0A',
        card:     '#1A1A1A',
        border:   '#2A2A2A',
        // Home screen brand palette
        navy:     '#002047',
        'brand-pink': '#ff3c6e',
        gold:     '#f5c842',
      },
    },
  },
  plugins: [],
};