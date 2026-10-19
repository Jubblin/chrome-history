// Minimal MV3 service worker: opens the timeline page in a tab when the
// toolbar action is clicked. It holds no state and makes no network requests.
const TIMELINE = 'timeline.html';

chrome.action.onClicked.addListener(async () => {
  const url = chrome.runtime.getURL(TIMELINE);
  const [existing] = await chrome.tabs.query({ url });
  if (existing) {
    await chrome.tabs.update(existing.id, { active: true });
    await chrome.windows.update(existing.windowId, { focused: true });
  } else {
    await chrome.tabs.create({ url });
  }
});
