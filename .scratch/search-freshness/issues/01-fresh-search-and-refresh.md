# SF-01: Fresh search loads and manual refresh

**What to build:** Browser reload and manual Refresh display current committed searchable torrents without waiting for backend cache expiry. Preserve search criteria and page, recover cleanly from failed requests, and stop owned work when leaving search.

**Blocked by:** None (can start immediately).

**Status:** complete

- [x] Initial loads and browser reload bypass stale backend and browser result caching.
- [x] Accessible manual Refresh fetches current results without changing filters, sorting or pagination; disabled while loading.
- [x] Failures settle loading, retain prior results, show the existing error and permit retry.
- [x] Replaced requests cannot publish stale results; leaving search disposes subscriptions and in-flight work even before table rendering.
- [x] No polling, new backend contract, global cache shutdown or production writes.
- [x] Deterministic checks at approved datasource seam and smoke of actual built display cover refresh, reload, errors and disposal.
- [x] Commit and two-axis Standards/Spec review complete; findings resolved or explicitly deferred.

Verification: final embedded build and changed-file lint passed; focused search suite 8 passed and final full frontend suite 84 passed, both with clean exits. Built-display smoke passed initial load, manual refresh, browser reload, preserved page/filter/order, disabled loading state, failure retention and retry, no polling and 390px layout. Screenshots visually checked. Controlled read adapter only; no production mutations or deployment.

Review of implementation `60464ce` against fixed point `236c129d745fca499252fc40167ea1d806302e51`: Standards 0 findings; Spec 2 P2 lifecycle findings. Both fixed in `5ab56ea`: cancel the controller's owned debounce before completing its subjects; assign the datasource subscriber before synchronous emissions, invalidate disposed requests, and recheck disposal after loading publication.

Review regressions failed before fixes: second synchronous result overwrote the first after disposal, request teardown did not run, loading disposal still launched a query, and pending page update published after display destruction. Final committed review of `5ab56ea`: Standards 0 findings; Spec 0 findings, both prior P2 gaps confirmed resolved; none deferred. One earlier full-suite run reported 84 successes but stayed alive until timeout; the final standalone run with dots reporter exited cleanly.
