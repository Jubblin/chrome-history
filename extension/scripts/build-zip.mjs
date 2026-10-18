// Builds the Chrome Web Store / release zip. Dependency-free and reproducible: files are
// taken from an ALLOWLIST (so dev files can never leak in), sorted, with fixed timestamps
// and modes, deflated at a fixed level. Same inputs => byte-identical zip.
//
//   node scripts/build-zip.mjs [--out DIR] [--tag vX.Y.Z]   build (default out: ../dist)
//   node scripts/build-zip.mjs --lint                        manifest lint only
import { readFileSync, readdirSync, statSync, mkdirSync, writeFileSync } from 'node:fs';
import { crc32 } from 'node:zlib';
import { join, resolve, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));

// Shipped paths (relative to extension/). Everything else (test/, scripts/, store/,
// ui/test-support/, README, package.json,) is excluded.
const INCLUDE_FILES = ['manifest.json', 'timeline.html'];
const INCLUDE_DIRS = ['icons', 'src', 'ui'];
const EXCLUDE = [/(^|\/)test-support(\/|$)/, /(^|\/)rules\.json$/, /\.map$/, /(^|\/)\./];

export function lintManifest(m) {
  const errs = [];
  if (m.manifest_version !== 3) errs.push('manifest_version must be 3');
  for (const k of ['name', 'version', 'description']) if (typeof m[k] !== 'string' || !m[k]) errs.push(`${k} missing`);
  if (!/^\d+(\.\d+){0,3}$/.test(m.version ?? '')) errs.push('version must be 1-4 dot-separated integers');
  if ((m.name ?? '').length > 75) errs.push('name longer than 75 characters');
  if ((m.description ?? '').length > 132) errs.push('description longer than 132 characters');
  if (JSON.stringify(m.permissions) !== '["history","storage"]') errs.push('permissions must be exactly ["history","storage"]');
  for (const k of ['host_permissions', 'optional_permissions', 'optional_host_permissions', 'content_scripts', 'externally_connectable', 'web_accessible_resources', 'incognito', 'update_url', 'key', 'oauth2', 'commands']) {
    if (k in m) errs.push(`${k} must be absent`);
  }
  const csp = m.content_security_policy?.extension_pages ?? '';
  if (!/default-src 'none'/.test(csp) || !/connect-src 'none'/.test(csp) || /unsafe-|https?:|\*/.test(csp)) errs.push('CSP is not the strict policy');
  for (const n of ['16', '32', '48', '128']) if (m.icons?.[n] !== `icons/icon-${n}.png`) errs.push(`icons.${n} missing`);
  return errs;
}

function walk(dir, out = []) {
  for (const name of readdirSync(join(root, dir)).sort()) {
    const rel = `${dir}/${name}`;
    if (statSync(join(root, rel)).isDirectory()) walk(rel, out); else out.push(rel);
  }
  return out;
}

export function shippedFiles() {
  const all = [...INCLUDE_FILES];
  for (const d of INCLUDE_DIRS) walk(d, all);
  return all.filter((f) => !EXCLUDE.some((re) => re.test(f))).sort();
}

export function buildZip(files) {
  const DOS_TIME = 0, DOS_DATE = (0 << 9) | (1 << 5) | 1; // 1980-01-01 00:00:00
  const locals = [], centrals = [];
  let offset = 0;
  for (const f of files) {
    const data = readFileSync(join(root, f));
    const comp = data; // stored, not deflated: output must not depend on the zlib version
    const name = Buffer.from(f, 'utf8');
    const crc = crc32(data);
    const lh = Buffer.alloc(30);
    lh.writeUInt32LE(0x04034b50, 0); lh.writeUInt16LE(20, 4); lh.writeUInt16LE(0x0800, 6); lh.writeUInt16LE(0, 8);
    lh.writeUInt16LE(DOS_TIME, 10); lh.writeUInt16LE(DOS_DATE, 12); lh.writeUInt32LE(crc, 14);
    lh.writeUInt32LE(comp.length, 18); lh.writeUInt32LE(data.length, 22); lh.writeUInt16LE(name.length, 26);
    locals.push(lh, name, comp);
    const ch = Buffer.alloc(46);
    ch.writeUInt32LE(0x02014b50, 0); ch.writeUInt16LE((3 << 8) | 20, 4); ch.writeUInt16LE(20, 6); ch.writeUInt16LE(0x0800, 8);
    ch.writeUInt16LE(0, 10); ch.writeUInt16LE(DOS_TIME, 12); ch.writeUInt16LE(DOS_DATE, 14); ch.writeUInt32LE(crc, 16);
    ch.writeUInt32LE(comp.length, 20); ch.writeUInt32LE(data.length, 24); ch.writeUInt16LE(name.length, 28);
    ch.writeUInt32LE((0o100644 << 16) >>> 0, 38); ch.writeUInt32LE(offset, 42);
    centrals.push(ch, name);
    offset += lh.length + name.length + comp.length;
  }
  const cd = Buffer.concat(centrals);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(files.length, 8); end.writeUInt16LE(files.length, 10);
  end.writeUInt32LE(cd.length, 12); end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, cd, end]);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const opt = (k) => { const i = args.indexOf(k); return i >= 0 ? args[i + 1] : undefined; };
  const m = JSON.parse(readFileSync(join(root, 'manifest.json'), 'utf8'));
  const errs = lintManifest(m);
  if (errs.length) { console.error('manifest lint failed:\n  ' + errs.join('\n  ')); process.exit(1); }
  const files = shippedFiles();
  for (const f of files) {
    if (!statSync(join(root, f)).isFile()) { console.error('not a regular file: ' + f); process.exit(1); }
  }
  if (args.includes('--lint')) { console.log(`manifest ok (version ${m.version}, ${files.length} files)`); process.exit(0); }
  const tag = opt('--tag');
  if (tag && tag.replace(/^v/, '') !== m.version) {
    console.error(`tag ${tag} does not match manifest version ${m.version}`); process.exit(1);
  }
  const out = resolve(opt('--out') ?? join(root, '..', 'dist'));
  mkdirSync(out, { recursive: true });
  const file = join(out, `chrome-history-extension_v${m.version}.zip`);
  writeFileSync(file, buildZip(files));
  console.log(relative(process.cwd(), file) || file, `(${files.length} files)`);
}
