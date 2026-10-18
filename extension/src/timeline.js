// Placeholder page; the real UI replaces this. Shows the data layer works.
import { loadVisits } from './history/index.js';

const status = document.getElementById('status');
const end = Date.now();
const visits = await loadVisits(chrome.history, { startTime: end - 7 * 864e5, endTime: end });
status.textContent = `${visits.length} visits in the last 7 days`;
