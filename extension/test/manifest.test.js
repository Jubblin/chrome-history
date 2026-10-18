import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';

const root = new URL('../', import.meta.url);
const m = JSON.parse(readFileSync(new URL('manifest.json', root), 'utf8'));

test('manifest is MV3 with only the history and storage permissions', () => {
  assert.equal(m.manifest_version, 3);
  assert.deepEqual(m.permissions, ['history', 'storage']);
  for (const k of ['host_permissions', 'optional_permissions', 'optional_host_permissions', 'content_scripts', 'externally_connectable', 'web_accessible_resources', 'incognito']) {
    assert.ok(!(k in m), `${k} must be absent`);
  }
});

test('manifest has required fields and existing files', () => {
  for (const k of ['name', 'version', 'description']) assert.equal(typeof m[k], 'string');
  assert.match(m.version, /^\d+(\.\d+){0,3}$/);
  assert.ok(m.description.length <= 132);
  assert.ok(existsSync(new URL(m.background.service_worker, root)));
  assert.ok(existsSync(new URL('timeline.html', root)));
});

test('strict extension-page CSP', () => {
  assert.equal(
    m.content_security_policy.extension_pages,
    "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'none'; base-uri 'none'; form-action 'none'",
  );
});

test('no network, eval or remote code in source', () => {
  const files = ['src/background.js', 'ui/app.js', 'ui/model.js', 'ui/classify-import.js', 'src/history/load.js', 'src/history/index.js', 'src/classify/storage.js', 'src/classify/index.js', 'src/classify/classify.js', 'timeline.html'];
  for (const f of files) {
    const s = readFileSync(new URL(f, root), 'utf8');
    assert.ok(!/\b(fetch|XMLHttpRequest|WebSocket|eval|importScripts)\b|new Function|https?:\/\/[^'"\s]*\.js/.test(s), f);
  }
});

test('timeline.html has no inline script, style or handlers (CSP)', () => {
  const s = readFileSync(new URL('timeline.html', root), 'utf8');
  assert.ok(!/<script(?![^>]*\bsrc=)/i.test(s) && !/<style/i.test(s) && !/\son\w+=|style=/i.test(s));
});
