# Security policy

chrome-history reads browser history, which is personal data, so security and privacy reports are taken seriously.

## Reporting a vulnerability

Please report vulnerabilities privately using GitHub private vulnerability reporting: open the **Security** tab of [github.com/Jubblin/chrome-history](https://github.com/Jubblin/chrome-history) and choose **Report a vulnerability**.

- **Never open a public issue, pull request or discussion for a vulnerability.**
- **Never include real browsing history** in a report: no real URLs, page titles, `History` files, `Local State` files or profile directories. Reproduce the problem with the synthetic fixture generator instead (`make fixture`, see [CONTRIBUTING.md](CONTRIBUTING.md)) and describe the steps.
- Do include the commit you tested, your OS and browser, what you did, what you expected and what happened.

## Supported versions

There are no tagged releases yet. Only the latest commit on `main` is supported; please check that the problem still exists there before reporting.

## What to expect

This is a hobby project, so these are best-effort targets, not guarantees:

- acknowledgement within 7 days;
- a status update within 30 days;
- coordinated disclosure: please give us a reasonable chance to fix the problem before publishing details, and we will agree a disclosure date with you;
- credit in the fix or advisory if you would like it.

## Scope

In scope:

- the localhost HTTP server (`internal/server`): the `Host` header check against DNS rebinding, profile-ID handling and path traversal, access from other web pages (CORS and CSRF-style requests), and response headers;
- reading of profile files (`internal/history`): the read-only guarantee, temporary-copy handling and cleanup, and symlink and path handling;
- rendering of untrusted history strings (URLs and titles) in the UI, for example cross-site scripting;
- CSV export, for example spreadsheet formula injection;
- the rules-file parser (`internal/classify`);
- bypasses of the "hide sensitive details" masking, where flagged text, URLs or domains still reach the page, the CSV export or the API results when masking is on.

Out of scope:

- attacks that need local code execution or access to the same user account: the tool reads files that user can already read;
- findings in third-party dependencies without a demonstrated impact on this project (please report those upstream);
- inaccuracy of the heuristic category lists (missed or wrongly flagged sites). Ordinary bugs can be reported as public issues, and users can adjust the lists with `add` and `allow` rules (see the README).
