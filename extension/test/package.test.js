import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, readdirSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname, posix } from 'node:path';
import { execFileSync } from 'node:child_process';
import { inflateRawSync } from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { lintManifest } from '../scripts/build-zip.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const script = join(root, 'scripts', 'build-zip.mjs');
const tmp = mkdtempSync(join(tmpdir(), 'pkg-test-'));

function build(dir) {
  execFileSync(process.execPath, [script, '--out', dir], { stdio: 'pipe' });
  const name = readdirSync(dir).find((f) => f.endsWith('.zip'));
  return join(dir, name);
}

// Minimal zip reader (central directory) so the test checks the real bytes.
function readZip(file) {
  const b = readFileSync(file);
  const end = b.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  const n = b.readUInt16LE(end + 10);
  let p = b.readUInt32LE(end + 16);
  const out = new Map();
  for (let i = 0; i < n; i++) {
    const method = b.readUInt16LE(p + 10), csize = b.readUInt32LE(p + 20), nlen = b.readUInt16LE(p + 28);
    const xlen = b.readUInt16LE(p + 30), clen = b.readUInt16LE(p + 32), off = b.readUInt32LE(p + 42);
    const name = b.toString('utf8', p + 46, p + 46 + nlen);
    const ds = off + 30 + b.readUInt16LE(off + 26) + b.readUInt16LE(off + 28);
    const raw = b.subarray(ds, ds + csize);
    out.set(name, method === 8 ? inflateRawSync(raw) : raw);
    p += 46 + nlen + xlen + clen;
  }
  return out;
}

const zipPath = build(join(tmp, 'a'));
const files = readZip(zipPath);
const names = [...files.keys()];

test('zip is reproducible: two builds are byte-identical', () => {
  assert.deepEqual(readFileSync(build(join(tmp, 'b'))), readFileSync(zipPath));
});

test('zip name carries the manifest version and manifest is at the root', () => {
  const m = JSON.parse(files.get('manifest.json'));
  assert.ok(zipPath.endsWith(`history-timeline-extension_v${m.version}.zip`));
  assert.deepEqual(lintManifest(m), []);
});

test('zip excludes tests, scripts, store assets, test-support and dev files', () => {
  for (const n of names) {
    assert.ok(!/^(test|scripts|store)\//.test(n), n);
    assert.ok(!/test-support|node_modules|package\.json|README|\.map$|rules\.json$|(^|\/)\./.test(n), n);
  }
  assert.ok(names.includes('manifest.json') && names.includes('timeline.html') && names.includes('ui/main.js'));
});

test('fixture mode is unreachable from the shipped zip (excluded by construction)', () => {
  for (const [n, data] of files) {
    if (!/\.(js|html|css)$/.test(n)) continue;
    const s = data.toString('utf8');
    assert.ok(!/fixture|test-support|mock-history|stub-classifier/i.test(s), `${n} mentions fixture/test-support`);
    if (/^(ui|src)\/.*\.js$/.test(n)) assert.ok(!/location\.(search|hash)|URLSearchParams/.test(s), `${n} reads URL parameters`);
  }
});

test('every relative import and page reference in the zip resolves inside the zip', () => {
  for (const [n, data] of files) {
    const s = data.toString('utf8');
    const refs = [];
    if (n.endsWith('.js')) for (const m of s.matchAll(/(?:from\s*|import\s*\(\s*|import\s+)['"](\.[^'"]+)['"]/g)) refs.push(m[1]);
    if (n.endsWith('.html')) for (const m of s.matchAll(/(?:src|href)="([^"#:]+)"/g)) refs.push(m[1]);
    for (const r of refs) assert.ok(files.has(posix.normalize(posix.join(posix.dirname(n), r))), `${n} -> ${r}`);
  }
  const m = JSON.parse(files.get('manifest.json'));
  for (const f of [m.background.service_worker, ...Object.values(m.icons), ...Object.values(m.action.default_icon)]) assert.ok(files.has(f), f);
});

test('manifest lint rejects extra permissions and a loose CSP', () => {
  const m = JSON.parse(files.get('manifest.json'));
  assert.ok(lintManifest({ ...m, permissions: ['history', 'storage', 'tabs'] }).length > 0);
  assert.ok(lintManifest({ ...m, host_permissions: ['<all_urls>'] }).length > 0);
  assert.ok(lintManifest({ ...m, content_security_policy: { extension_pages: "script-src 'self' 'unsafe-eval'" } }).length > 0);
});

test('--tag must match the manifest version', () => {
  assert.throws(() => execFileSync(process.execPath, [script, '--out', join(tmp, 'c'), '--tag', 'v9.9.9'], { stdio: 'pipe' }));
});

test.after(() => rmSync(tmp, { recursive: true, force: true }));
