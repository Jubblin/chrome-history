# Chrome Web Store submission checklist

Status as of this branch: the package, assets and text are ready; **nothing has been submitted** and nothing below under "You must do" has been done.

## Done in this repository
- [x] Manifest V3, minimal permissions (`history`, `storage`), no host permissions, no remote code, strict CSP.
- [x] Reproducible zip: `make extension-zip` (allowlist; excludes tests, scripts, store assets, fixtures; fixture mode is not in the zip).
- [x] Manifest lint (`make extension-lint`), also run by the zip build.
- [x] Icons 16/32/48/128 (original artwork, `icon.svg`).
- [x] 4 screenshots at 1280x800 from synthetic fixture data (`screenshots/`).
- [x] Privacy policy text (`PRIVACY.md`) consistent with the CSP and permissions.
- [x] Listing text, single-purpose statement and permission justifications (`LISTING.md`).
- [x] Release workflow attaches the zip and checksum to the draft release.

## You must do (not done)
- [ ] Register a Chrome Web Store developer account and pay the one-time registration fee (check the current amount in the dashboard); verify the account (2-step verification is required).
- [ ] Publish `PRIVACY.md` at a public URL (for example its GitHub page on the default branch once merged) and enter that URL in the dashboard's Privacy tab. The store requires a URL, not a file.
- [ ] Decide the publisher identity (individual or group) and the support contact shown on the listing. No personal names or emails are in this repository; add them in the dashboard only if you choose to.
- [ ] Upload the zip from the release (check `sha256sum -c`), fill the Store listing and Privacy practices tabs from `LISTING.md`, upload icons/screenshots (store icon is `icons/icon-128.png`; a small promo tile 440x280 is optional and not created).
- [ ] Choose visibility (public, unlisted or private), regions and pricing (free).
- [ ] Submit for review. Review of an extension that requests `history` can take longer and may ask questions; answer with the justification above. Expect to resubmit with a higher version number after any change (the manifest `version` must increase for each upload).
- [ ] Test the uploaded package from the store (or the zip) on a real, branded Chrome profile and on ChromeOS if you want to claim it works there; only Chromium via automation has been tested so far.

## Not done / known gaps
- No promotional tile or marquee image.
- Not tested in branded Google Chrome (it ignores `--load-extension` automation); tested in Playwright's Chromium and by loading the unpacked zip contents.
- Chromebook/ChromeOS behaviour and managed-device policy blocking are untested.
- No Firefox/Edge listing; Edge accepts Chrome extensions via its own add-ons portal, not covered here.
