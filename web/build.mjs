// Builds the desktop interface and the mobile interface into dist/.
import * as esbuild from 'esbuild';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { verifyStatisticsSources } from './statistics-source-integrity.mjs';

// Verify every pinned package before replacing working build assets. Downloads
// are build-only; the product mounts an embedded read-only WORKERFS image.
const packageCache = process.env.MNELAB_R_PACKAGE_CACHE || '.statistics-package-cache';
const packageImage = path.join(packageCache, 'image');
execFileSync('go', ['run', '../scripts/statistics_packages.go', '-manifest', '../scripts/statistics-packages.json', '-cache', packageCache, '-out', packageImage], { stdio: 'inherit' });

// Verify exact pinned source bytes before replacing working assets.
verifyStatisticsSources('src/lib/vendor/mbess');

const dist = 'dist';
fs.rmSync(dist, { recursive: true, force: true });
fs.mkdirSync(dist, { recursive: true });
fs.writeFileSync(path.join(dist, '.gitkeep'), '');

const common = {
  bundle: true,
  minify: true,
  sourcemap: false,
  target: ['chrome100', 'edge100'],
  jsx: 'automatic',
  jsxImportSource: 'preact',
  loader: { '.woff2': 'file', '.svg': 'text', '.R': 'text' },
  entryNames: 'assets/[name]-[hash]',
  assetNames: 'assets/[name]-[hash]',
  metafile: true,
  legalComments: 'none',
  outdir: dist,
  define: { 'process.env.NODE_ENV': '"production"' },
};

async function build(entry, publicPath) {
  const r = await esbuild.build({ ...common, entryPoints: { [entry.name]: entry.file }, publicPath });
  const outs = Object.keys(r.metafile.outputs);
  const js = outs.find((o) => o.endsWith('.js'));
  const css = outs.find((o) => o.endsWith('.css'));
  return { js: path.relative(dist, js), css: css && path.relative(dist, css) };
}

const app = await build({ name: 'app', file: 'src/main.tsx' }, '/');
const mob = await build({ name: 'm', file: 'src/mobile/main.tsx' }, '/m/');

// The statistical runtime is embedded with the application. Only the worker,
// R runtime and lazy filesystem are needed; no REPL or external package server.
const runtime = path.join(dist, 'statistics-engine');
fs.mkdirSync(runtime, { recursive: true });
for (const name of ['R.js', 'R.wasm', 'libRblas.so', 'libRlapack.so', 'webr-worker.js', 'webr.mjs', 'vfs']) {
  fs.cpSync(path.join('node_modules/webr/dist', name), path.join(runtime, name), { recursive: true });
}
// The product never downloads R packages. Its offline calculations do not
// need the upstream public TLS trust store; omit unnecessary PEM material.
fs.rmSync(path.join(runtime, 'vfs/etc/ssl/cert.pem'), { force: true });
fs.copyFileSync('node_modules/webr/LICENSE.md', path.join(runtime, 'LICENSE.md'));
fs.copyFileSync('../docs/STATISTICAL_PACKAGES_LICENSES.md', path.join(runtime, 'PACKAGES_LICENSES.md'));
fs.cpSync('src/lib/vendor/mbess',path.join(runtime,'mbess-source'),{recursive:true});
for (const name of ['packages.data.gz', 'packages.metadata.json', 'packages.manifest.json']) {
  fs.copyFileSync(path.join(packageImage, name), path.join(runtime, name));
}

const icon = fs.readFileSync('../assets/brand/mnelab-icon.svg', 'utf8');
fs.writeFileSync(path.join(dist, 'icon.svg'), icon);
fs.copyFileSync('../assets/brand/png/icon-192.png', path.join(dist, 'icon-192.png'));
fs.copyFileSync('../assets/brand/png/icon-512.png', path.join(dist, 'icon-512.png'));
fs.copyFileSync('../assets/brand/png/icon-180.png', path.join(dist, 'apple-touch-icon.png'));

const page = (title, base, files, extra = '') => `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="color-scheme" content="dark light">
<meta name="theme-color" content="#0B0F14">
<meta name="referrer" content="no-referrer">
<title>${title}</title>
<link rel="icon" href="${base}icon.svg" type="image/svg+xml">
${extra}${files.css ? `<link rel="stylesheet" href="${base}${files.css}">` : ''}
<script type="module" src="${base}${files.js}"></script>
</head>
<body><div id="app"></div></body>
</html>
`;
fs.writeFileSync(path.join(dist, 'index.html'), page('MNE Lab', '/', app));
fs.writeFileSync(path.join(dist, 'm.html'), page('MNE Lab', '/m/', mob,
  '<link rel="apple-touch-icon" href="/m/apple-touch-icon.png">\n<link rel="manifest" href="/m/manifest.webmanifest">\n<meta name="apple-mobile-web-app-capable" content="yes">\n'));
fs.writeFileSync(path.join(dist, 'manifest.webmanifest'), JSON.stringify({
  name: 'MNE Lab', short_name: 'MNE Lab', start_url: '/m', display: 'standalone',
  background_color: '#0B0F14', theme_color: '#0B0F14',
  icons: [{ src: '/m/icon-192.png', sizes: '192x192', type: 'image/png' }, { src: '/m/icon-512.png', sizes: '512x512', type: 'image/png' }],
}));
const size = (f) => (fs.statSync(path.join(dist, f)).size / 1024).toFixed(0) + ' KiB';
console.log('app', app.js, size(app.js), app.css ? size(app.css) : '');
console.log('mobile', mob.js, size(mob.js), mob.css ? size(mob.css) : '');
