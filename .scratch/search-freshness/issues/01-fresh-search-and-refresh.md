# SF-01: Fresh search loads and manual refresh

**What to build:** Browser reload and manual Refresh display current committed searchable torrents without waiting for backend cache expiry. Preserve search criteria and page, recover cleanly from failed requests, and stop owned work when leaving search.

**Blocked by:** None (can start immediately).

**Status:** in-progress

- [x] Initial loads and browser reload bypass stale backend and browser result caching.
- [x] Accessible manual Refresh fetches current results without changing filters, sorting or pagination; disabled while loading.
- [x] Failures settle loading, retain prior results, show the existing error and permit retry.
- [x] Replaced requests cannot publish stale results; leaving search disposes subscriptions and in-flight work even before table rendering.
- [x] No polling, new backend contract, global cache shutdown or production writes.
- [x] Deterministic checks at approved datasource seam and smoke of actual built display cover refresh, reload, errors and disposal.
- [ ] Commit and two-axis Standards/Spec review complete; findings resolved or explicitly deferred.

Verification: embedded build passed; two lifecycle regressions passed (error regression failed before fix); full frontend suite 82 passed; built-display smoke passed initial load, manual refresh, browser reload, preserved page/filter/order, disabled loading state, failure retention and retry, no polling and 390px layout. Screenshots visually checked. Controlled read adapter only; no production mutations.
