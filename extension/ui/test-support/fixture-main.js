// Test-only entry: runs the timeline on synthetic history. Not shipped.
import { start } from '../app.js';
import { fixtureHistory } from './fixture.js';
// ?clean=1 omits the adversarial (XSS/formula) rows; used for Store screenshots.
start(fixtureHistory({ adversarial: !new URLSearchParams(location.search).has('clean') }));
