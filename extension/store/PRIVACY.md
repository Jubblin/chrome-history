# Privacy policy: Browsing History Timeline

_Last updated: see the git history of this file._

Browsing History Timeline is a browser extension that shows you an audit timeline of your own browsing history, so you can see what a profile has recorded and which visits look sensitive.

## What it does with your data

- **Reads your browsing history, on your device only.** It uses the browser's history permission to read the URLs, page titles, visit times and transition types of the current browser profile. This happens inside the extension page and is shown only to you.
- **Makes no network requests.** The extension has no host permissions, no remote code, no analytics, no telemetry, no crash reporting and no accounts. Its content security policy (`default-src 'none'; connect-src 'none'`) forbids network connections from its pages, and its background script contains no network code.
- **Stores one thing: your own rules.** If you write custom classification rules (`add`/`allow` lists of domains and keywords) they are saved as text in the extension's local storage area, which stays in this browser profile. The `storage` permission exists only for this. The on/off setting "Hide sensitive details" is remembered in the extension page's own local storage, also inside this browser profile. Your history itself is never copied or stored by the extension: it is read each time the timeline opens and kept in memory only while the page is open.
- **Does not modify or delete history.** It only reads.
- **Classification is local.** Flagging categories such as gambling or health uses a rule list bundled with the extension and runs in your browser. No visit is sent anywhere to be classified.
- **CSV export is local.** "Export CSV" creates a file in your browser and saves it where you choose. Nothing is uploaded. With "Hide sensitive details" on, flagged values in the export are masked.

## What it does not do

It does not sell, share, transfer or disclose any data to anyone, because it never sends data anywhere. It does not use data for advertising, creditworthiness or any purpose other than showing you your own timeline.

## Limits

It reads only the current browser profile, and does not see incognito history. It cannot read history on other devices unless the browser has synced it into this profile.

## Removing your data

Uninstalling the extension deletes everything it stored. You can also clear your rules in the extension's Settings panel.

## Contact and changes

Questions or concerns: open an issue on the project's GitHub repository (never include real browsing history), or use the private vulnerability reporting described in `SECURITY.md` for security problems. Changes to this policy will be committed to this file with their date in the git history.
