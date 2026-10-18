// The single integration point with the classifier (contract: extension/src/classify/index.js).
// INTEGRATION SWAP: when the classifier branch is merged, replace the line below with
//   export { CATEGORIES, createClassifier, parseUserRules, loadBundledRules } from '../src/classify/index.js';
// and keep ui/test-support/stub-classifier.js out of the shipped zip.
export { CATEGORIES, createClassifier, parseUserRules, loadBundledRules } from './test-support/stub-classifier.js';
