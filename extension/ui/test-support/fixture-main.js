// Test-only entry: runs the timeline on synthetic history. Not shipped.
import { start } from '../app.js';
import { fixtureHistory } from './fixture.js';
start(fixtureHistory());
