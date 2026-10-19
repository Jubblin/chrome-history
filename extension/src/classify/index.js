// Public interface of the classifier module (shared contract with the UI).
import { Classifier, parseUserRules as parseStrict } from './classify.js';
import bundled from './rules.js';

export const CATEGORIES = [
  { id: 'adult', label: 'Adult' },
  { id: 'gambling', label: 'Gambling' },
  { id: 'health', label: 'Health' },
  { id: 'finance', label: 'Finance' },
  { id: 'privacy', label: 'Privacy tools' },
  { id: 'news_politics', label: 'News & politics' },
];

/** @returns {{classify(url: string): string[]}} */
export function createClassifier({ bundledRules, userRules = null }) {
  const c = new Classifier(bundledRules, userRules);
  return {
    classify(url) {
      try {
        return c.classify(String(url));
      } catch {
        return [];
      }
    },
  };
}

/** @returns {{rules: object} | {error: string}} */
export function parseUserRules(text) {
  try {
    return { rules: parseStrict(text, bundled) };
  } catch (e) {
    return { error: String(e.message) };
  }
}

export async function loadBundledRules() {
  return bundled;
}
