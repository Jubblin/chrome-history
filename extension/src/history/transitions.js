// Core transition types reported by chrome.history.getVisits.
export const TRANSITIONS = Object.freeze([
  'link', 'typed', 'auto_bookmark', 'auto_subframe', 'manual_subframe',
  'generated', 'auto_toplevel', 'form_submit', 'reload', 'keyword', 'keyword_generated',
]);

// Returns the transition if it is a known core type, else 'link' (Chrome's
// default) so downstream code only ever sees values from TRANSITIONS.
export function normalizeTransition(t) {
  return TRANSITIONS.includes(t) ? t : 'link';
}
