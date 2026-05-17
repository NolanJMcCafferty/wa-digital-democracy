// Tailwind CSS v4 uses a single PostCSS plugin; no tailwind config file needed
// for the basic setup — directives are picked up directly from src/app/globals.css.
const config = {
  plugins: {
    "@tailwindcss/postcss": {},
  },
};

export default config;
