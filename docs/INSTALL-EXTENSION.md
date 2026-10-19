# Installing the Browsing History Timeline extension (from a release)

This guide installs the released extension zip into Chrome with **Load unpacked**. The extension is not on the Chrome Web Store yet. Chrome's screens may look slightly different depending on your Chrome version; the steps describe them in general terms.

## 1. What you get

A timeline of the browsing history of the Chrome profile it is installed in: days and sessions, an activity chart, top sites, filters, flags for sensitive categories (with a "Hide sensitive details" switch) and CSV export.

- It reads history only through Chrome's own `history` feature, for the profile it is installed in.
- It makes **no network requests**. Its only permissions are `history` and `storage` (the second one keeps your optional custom rules on this device).
- Incognito and Guest browsing is not recorded by Chrome, so it never appears.

See [`extension/store/PRIVACY.md`](../extension/store/PRIVACY.md) for the full privacy policy.

## 2. Before you start

- Chrome, or another Chromium browser such as Edge or Brave, on ChromeOS, Windows, macOS or Linux. (Steps below say "Chrome"; the other browsers have the same `chrome://extensions` page, or an `edge://extensions` / `brave://extensions` equivalent.)
- If the device is managed by a school or employer, Developer mode or installing extensions may be blocked, and you will see that the option is missing, greyed out, or says it is managed by your organisation. Don't try to get around it; ask the administrator instead.

## 3. Download and verify

1. Open the [release page for the version you are installing](https://github.com/Jubblin/history-timeline/releases/tag/vX.Y.Z). Under **Assets**, download:
   - `history-timeline-extension_<version>.zip` (for example, for v0.3.0: `history-timeline-extension_v0.3.0.zip`)
   - `SHA256SUMS`
2. Put both files in the same folder (for example Downloads) and check the zip was not damaged or altered. `SHA256SUMS` lists every release file, so only the extension line is checked.

   Linux / ChromeOS (Linux terminal):
   ```
   grep 'history-timeline-extension_vX.Y.Z.zip' SHA256SUMS | sha256sum -c
   ```
   macOS:
   ```
   grep 'history-timeline-extension_vX.Y.Z.zip' SHA256SUMS | shasum -a 256 -c
   ```
   Windows PowerShell:
   ```
   Get-FileHash .\history-timeline-extension_vX.Y.Z.zip -Algorithm SHA256
   Select-String history-timeline-extension .\SHA256SUMS
   ```
   Linux/ChromeOS and macOS print `history-timeline-extension_vX.Y.Z.zip: OK`. On Windows, compare the two hashes by eye; they must be identical.
3. Optional, stronger check (GitHub CLI 2.49 or newer; older versions have no `attestation` command). It proves the zip was built by this repository's release workflow for the release tag:
   ```
   gh attestation verify history-timeline-extension_vX.Y.Z.zip -R Jubblin/history-timeline --source-ref refs/tags/vX.Y.Z
   ```

## 4. Unzip into an empty, permanent folder

Chrome runs the extension **directly from this folder**, so choose a place you will keep, such as `Documents/history-timeline-extension`. The files are at the root of the zip, so use a **new, empty** folder.

- **ChromeOS (Files app):** right-click the zip and choose Extract all (or open the zip and copy its contents into a new folder).
- **macOS:** double-click the zip, then move the resulting folder somewhere permanent.
- **Windows:** right-click the zip, choose Extract All and pick a new folder.
- **Linux:** `mkdir ~/history-timeline-extension && unzip history-timeline-extension_vX.Y.Z.zip -d ~/history-timeline-extension`

The folder should now contain `manifest.json`, `timeline.html`, and the `icons`, `src` and `ui` folders.

**Do not move, rename or delete this folder afterwards.** Doing so breaks the extension.

## 5. Load it in Chrome

1. Go to `chrome://extensions` (type it in the address bar).
2. Turn on **Developer mode** (toggle at the top right). This is a browser setting on that page that lets you load extensions from a folder. It is unrelated to ChromeOS's own "Developer mode" device setting, which is not needed here and which you should not turn on for this.
3. Click **Load unpacked** and select the folder that **directly contains `manifest.json`** (not its parent folder).
4. Success looks like a new card named **Browsing History Timeline**, the version you downloaded (for example **0.3.0**), switched on. The extension asks for access to your browsing history; the exact wording depends on your Chrome version.
5. To keep the button handy, click the puzzle-piece (Extensions) icon in the toolbar and use the pin option next to the extension.

## 6. First use

Click the extension's toolbar button. The timeline opens in a new tab (or focuses it if already open).

- **Loading:** a progress bar and a **Cancel loading** button show while history loads. The first rows appear quickly and totals fill in as loading finishes. Very large histories can take a few seconds.
- **Browse:** pick a date range (From / To), search by title or URL, filter by domain, and open **Transition types** for more filters.
- **Sensitive flags:** visits to categories such as gambling or adult sites are badged. Turn on **Hide sensitive details** before sharing your screen; it masks those titles and URLs (and the CSV export).
- **Export CSV:** downloads what the current filters show.
- **Settings:** add your own rules as JSON (`add` flags more sites, `allow` removes matches). Details are in the [extension README](../extension/README.md).

## 7. Several profiles

Chrome extensions are per profile. Install once in each profile you want to audit; each shows only that profile's history. Incognito and Guest windows are not covered.

## 8. Updating

There is no automatic update for an unpacked extension. For a new release:

1. Download and verify the new zip (step 3).
2. Either extract it over the **same** folder (replace the old files; extract into a fresh folder first and copy across if unsure), or extract into a new empty folder, remove the old extension and load the new folder.
3. On `chrome://extensions`, click the **reload** icon on the extension card. Check the version shown.

Your custom rules survive a reload and an in-place update. They are lost if you remove the extension.

## 9. Removing

On `chrome://extensions`, click **Remove** on the card, then delete the folder. This deletes your saved custom rules and the "Hide sensitive details" setting. Your browsing history is not touched by the extension.

## 10. Troubleshooting

| Problem | Likely cause and fix |
| --- | --- |
| No **Load unpacked** button, or Developer mode is greyed out | Developer mode is off, or a school/work policy blocks it. Turn it on; if it is locked, contact the administrator. |
| "Manifest file is missing or unreadable" | You picked the wrong folder or the zip is not extracted. Select the folder that directly contains `manifest.json`. |
| "Could not load extension" or other load errors | The folder is incomplete or mixed with other files. Re-extract the zip into a new empty folder, and check the checksum. |
| Extension is switched off or disappears after a Chrome restart/update | The folder was moved or deleted (restore it, or load it again). Some systems also show a notice about developer-mode extensions; if it appears, choose to keep the extension, and only if your organisation allows it. |
| Timeline is empty or short | History was cleared, or Chrome only keeps recent history (about 90 days by default). Incognito visits never appear. |
| Takes a long time to load | Very large histories take several seconds. Narrow the date range or use **Cancel loading** to keep what has loaded. |
| Clicking the icon does nothing | Reload the extension on `chrome://extensions`. Look for an error on the extension card. |
| Using Safari or Firefox | Not supported. Use a Chromium browser. |

## 11. Chromebooks

This is the way to audit the history of ChromeOS's own Chrome, which the Linux version of the app cannot read. Follow the steps above in ChromeOS Chrome. See the [Chromebook section of the README](../README.md#chromebook-chromeos) for the Linux-container route, which suits other uses.
