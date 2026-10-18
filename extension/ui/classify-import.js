// The single integration point with the classifier (contract: src/classify/index.js).
// PACKAGING: exclude ui/test-support/ (stub classifier, synthetic fixture) from the shipped zip.
export { CATEGORIES, createClassifier, parseUserRules, loadBundledRules } from '../src/classify/index.js';
