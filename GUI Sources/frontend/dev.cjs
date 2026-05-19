const { createServer } = require('vite');
const { svelte } = require('@sveltejs/vite-plugin-svelte');

createServer({
  root: __dirname,
  configFile: false,
  plugins: [svelte()]
}).then(async (server) => {
  await server.listen();
  server.printUrls();
}).catch((error) => {
  console.error(error);
  process.exit(1);
});
