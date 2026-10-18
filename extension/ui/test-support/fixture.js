// TEST SUPPORT ONLY (fixture mode, ?fixture=1). Synthetic data; not shipped in the zip.
import { createMockHistory } from '../../src/history/index.js';
import { makeFixtureEntries } from '../../scripts/make-fixture.js';

const DAY = 864e5;
export function fixtureHistory({ visits = 12000 } = {}) {
  const end = Date.now() - 3600e3;
  const entries = makeFixtureEntries({ visits, endTime: end, days: 90 });
  const t = (daysAgo, h) => end - daysAgo * DAY - h * 3600e3;
  const v = (when, transition = 'link') => ({ visitTime: when, transition });
  entries.push(
    { url: 'https://site1.example.test/<img src=x onerror=alert(1)>', title: '<img src=x onerror="document.title=\'pwned\'">XSS title', visits: [v(t(0, 1)), v(t(0, 1.1))] },
    { url: 'javascript:alert(1)', title: '=HYPERLINK("http://evil.example","click")', visits: [v(t(0, 1.2))] },
    { url: 'https://site2.example.test/formula', title: '@SUM(1+1)', visits: [v(t(1, 2))] },
    { url: 'https://bet365.com/slots', title: 'Secret casino slots', visits: [v(t(0, 2)), v(t(2, 3)), v(t(5, 1))] },
    { url: 'https://www.aetna.com/appointments', title: 'Clinic appointments', visits: [v(t(0, 2.2)), v(t(3, 4))] },
    { url: 'https://site3.example.test/symptom-checker', title: 'Symptom checker', visits: [v(t(1, 5))] },
    { url: 'https://bankofamerica.com/accounts', title: 'Bank accounts overview', visits: [v(t(0, 5)), v(t(10, 2))] },
    { url: 'https://bbc.co.uk/politics/story', title: 'Election story', visits: [v(t(4, 6))] },
    { url: 'https://astrill.com/', title: 'VPN download', visits: [v(t(6, 7))] },
  );
  return createMockHistory(entries);
}
