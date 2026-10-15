// Regenerates the raster brand icons from the SVG masters in assets/brand:
// the PNGs (16-32 px from mnelab-icon-small.svg, 40 px and larger from
// mnelab-icon.svg), the Windows icons and the copies internal/brand embeds.
// Rendering uses Chromium through playwright-core, a web devDependency.
//
//   cd web && npm ci && npx playwright-core install chromium
//   node ../scripts/brand_icons.mjs
//
// MNELAB_CHROMIUM may point to another Chromium build. The committed rasters
// were rendered with Chromium 141.0.7390.37; another build can differ in
// antialiasing by a few pixel values.
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const brand = path.join(root, 'assets/brand');
const { chromium } = createRequire(path.join(root, 'web/package.json'))('playwright-core');

const small = [16, 20, 24, 32];
const large = [40, 48, 64, 96, 128, 180, 192, 256, 512, 1024];
const appICO = [16, 20, 24, 32, 40, 48, 64, 96, 128, 256];
const trayICO = [16, 20, 24, 32, 48];

const png = (s) => path.join(brand, 'png', `icon-${s}.png`);

const browser = await chromium.launch({ executablePath: process.env.MNELAB_CHROMIUM || undefined, args: ['--no-sandbox'] });
try {
  const page = await browser.newPage({ deviceScaleFactor: 1 });
  for (const [svg, sizes] of [['mnelab-icon-small.svg', small], ['mnelab-icon.svg', large]]) {
    const src = 'data:image/svg+xml;base64,' + fs.readFileSync(path.join(brand, svg)).toString('base64');
    for (const s of sizes) {
      await page.setViewportSize({ width: s, height: s });
      await page.setContent(`<html><body style="margin:0;background:transparent"><img src="${src}" width="${s}" height="${s}" style="display:block"></body></html>`);
      await page.waitForFunction(() => document.images[0].complete);
      await page.screenshot({ path: png(s), omitBackground: true, clip: { x: 0, y: 0, width: s, height: s } });
    }
  }
} finally {
  await browser.close();
}

// rgba decodes an 8-bit RGBA, non-interlaced PNG (what Chromium writes).
function rgba(file) {
  const b = fs.readFileSync(file);
  let off = 8;
  let w = 0;
  let h = 0;
  const idat = [];
  while (off < b.length) {
    const len = b.readUInt32BE(off);
    const type = b.toString('latin1', off + 4, off + 8);
    const data = b.subarray(off + 8, off + 8 + len);
    if (type === 'IHDR') {
      w = data.readUInt32BE(0);
      h = data.readUInt32BE(4);
      if (data[8] !== 8 || data[9] !== 6 || data[12] !== 0) throw new Error(`${file}: not 8-bit RGBA non-interlaced`);
    } else if (type === 'IDAT') idat.push(data);
    off += 12 + len;
  }
  const raw = zlib.inflateSync(Buffer.concat(idat));
  const stride = w * 4;
  const px = Buffer.alloc(stride * h);
  for (let y = 0; y < h; y++) {
    const f = raw[y * (stride + 1)];
    const line = raw.subarray(y * (stride + 1) + 1, (y + 1) * (stride + 1));
    for (let x = 0; x < stride; x++) {
      const a = x >= 4 ? px[y * stride + x - 4] : 0;
      const up = y > 0 ? px[(y - 1) * stride + x] : 0;
      const c = x >= 4 && y > 0 ? px[(y - 1) * stride + x - 4] : 0;
      const p = a + up - c;
      const pa = Math.abs(p - a);
      const pb = Math.abs(p - up);
      const pc = Math.abs(p - c);
      const pred = [0, a, up, (a + up) >> 1, pa <= pb && pa <= pc ? a : pb <= pc ? up : c][f];
      if (pred === undefined) throw new Error(`${file}: bad filter`);
      px[y * stride + x] = (line[x] + pred) & 255;
    }
  }
  return { w, h, px };
}

// encode writes pixels as a PNG with maximum compression and, per row, the
// filter with the smallest sum of absolute values, independently of the
// encoder settings of the Chromium build that rendered them.
function encode(file, { w, h, px }) {
  const stride = w * 4;
  const rows = [];
  for (let y = 0; y < h; y++) {
    let best = null;
    let score = Infinity;
    for (let f = 0; f < 5; f++) {
      const line = Buffer.alloc(stride + 1);
      line[0] = f;
      let sum = 0;
      for (let x = 0; x < stride; x++) {
        const a = x >= 4 ? px[y * stride + x - 4] : 0;
        const up = y > 0 ? px[(y - 1) * stride + x] : 0;
        const c = x >= 4 && y > 0 ? px[(y - 1) * stride + x - 4] : 0;
        const p = a + up - c;
        const pa = Math.abs(p - a);
        const pb = Math.abs(p - up);
        const pc = Math.abs(p - c);
        const pred = [0, a, up, (a + up) >> 1, pa <= pb && pa <= pc ? a : pb <= pc ? up : c][f];
        const v = (px[y * stride + x] - pred) & 255;
        line[x + 1] = v;
        sum += v < 128 ? v : 256 - v;
      }
      if (sum < score) {
        score = sum;
        best = line;
      }
    }
    rows.push(best);
  }
  const chunk = (type, data) => {
    const out = Buffer.alloc(12 + data.length);
    out.writeUInt32BE(data.length, 0);
    out.write(type, 4, 'latin1');
    data.copy(out, 8);
    out.writeUInt32BE(zlib.crc32(out.subarray(4, 8 + data.length)), 8 + data.length);
    return out;
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;
  const idat = zlib.deflateSync(Buffer.concat(rows), { level: 9, memLevel: 9 });
  fs.writeFileSync(file, Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', ihdr), chunk('IDAT', idat), chunk('IEND', Buffer.alloc(0))]));
}
for (const s of [...small, ...large]) encode(png(s), rgba(png(s)));

// bmp is a 32-bit ICO image entry: BGRA rows bottom-up, then an AND mask
// that marks fully transparent pixels.
function bmp(file) {
  const { w, h, px } = rgba(file);
  const head = Buffer.alloc(40);
  head.writeUInt32LE(40, 0);
  head.writeInt32LE(w, 4);
  head.writeInt32LE(h * 2, 8);
  head.writeUInt16LE(1, 12);
  head.writeUInt16LE(32, 14);
  head.writeUInt32LE(w * h * 4, 20);
  const color = Buffer.alloc(w * h * 4);
  const maskStride = Math.ceil(w / 32) * 4;
  const mask = Buffer.alloc(maskStride * h);
  for (let y = 0; y < h; y++) {
    const row = h - 1 - y;
    for (let x = 0; x < w; x++) {
      const i = (row * w + x) * 4;
      const o = (y * w + x) * 4;
      color[o] = px[i + 2];
      color[o + 1] = px[i + 1];
      color[o + 2] = px[i];
      color[o + 3] = px[i + 3];
      if (px[i + 3] === 0) mask[y * maskStride + (x >> 3)] |= 0x80 >> (x & 7);
    }
  }
  return Buffer.concat([head, color, mask]);
}

// ico writes BMP entries below 256 px and the PNG itself at 256 px.
function ico(out, sizes) {
  const blobs = sizes.map((s) => (s >= 256 ? fs.readFileSync(png(s)) : bmp(png(s))));
  const head = Buffer.alloc(6 + 16 * sizes.length);
  head.writeUInt16LE(1, 2);
  head.writeUInt16LE(sizes.length, 4);
  let off = head.length;
  sizes.forEach((s, i) => {
    const d = 6 + 16 * i;
    head[d] = s >= 256 ? 0 : s;
    head[d + 1] = s >= 256 ? 0 : s;
    head.writeUInt16LE(1, d + 4);
    head.writeUInt16LE(32, d + 6);
    head.writeUInt32LE(blobs[i].length, d + 8);
    head.writeUInt32LE(off, d + 12);
    off += blobs[i].length;
  });
  fs.writeFileSync(out, Buffer.concat([head, ...blobs]));
}

ico(path.join(brand, 'mnelab.ico'), appICO);
ico(path.join(brand, 'mnelab-tray.ico'), trayICO);
fs.copyFileSync(path.join(brand, 'mnelab-tray.ico'), path.join(root, 'internal/brand/tray.ico'));
fs.copyFileSync(png(32), path.join(root, 'internal/brand/tray.png'));
fs.copyFileSync(png(256), path.join(root, 'internal/brand/icon-256.png'));
console.log('brand icons written');
