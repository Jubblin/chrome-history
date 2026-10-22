import test from 'node:test';
import assert from 'node:assert/strict';
import { binOf, binRange } from '../../shared/ui-common.js';

test('binOf week starts Monday, crosses month/year', () => {
  assert.equal(binOf('2024-01-01', 'week'), '2024-01-01'); // Monday
  assert.equal(binOf('2024-01-07', 'week'), '2024-01-01'); // Sunday
  assert.equal(binOf('2025-01-01', 'week'), '2024-12-30');
  assert.equal(binOf('2024-03-15', 'month'), '2024-03');
  assert.equal(binOf('2024-03-15', 'day'), '2024-03-15');
});
test('binRange', () => {
  assert.deepEqual(binRange('2024-12-30', 'week'), ['2024-12-30', '2025-01-05']);
  assert.deepEqual(binRange('2024-02', 'month'), ['2024-02-01', '2024-02-29']);
  assert.deepEqual(binRange('2024-05-02', 'day'), ['2024-05-02', '2024-05-02']);
});
