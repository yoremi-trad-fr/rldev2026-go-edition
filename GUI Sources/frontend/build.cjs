const path = require('path');
const { build } = require('vite');
const { svelte } = require('@sveltejs/vite-plugin-svelte');

build({
  root: __dirname,
  configFile: false,
  plugins: [svelte()],
  build: {
    outDir: path.join(__dirname, 'dist'),
    emptyOutDir: true
  }
}).catch((error) => {
  console.error(error);
  process.exit(1);
});
