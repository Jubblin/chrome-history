// User rules persistence in chrome.storage.local (needs the "storage" permission).
// The raw JSON text is stored so the user can edit exactly what they saved.
import { parseUserRules, Classifier } from './classify.js';
import bundled from './rules.js';

const KEY = 'userRulesText';

export async function saveUserRules(area, text) {
  parseUserRules(text, bundled);
  await area.set({ [KEY]: text });
}

export async function loadUserRulesText(area) {
  const r = await area.get(KEY);
  return typeof r[KEY] === 'string' ? r[KEY] : '';
}

/** Builds a classifier from bundled rules plus any stored (valid) user rules. */
export async function loadClassifier(area) {
  const text = await loadUserRulesText(area);
  return new Classifier(bundled, text ? parseUserRules(text, bundled) : null);
}

export async function clearUserRules(area) {
  await area.remove(KEY);
}
