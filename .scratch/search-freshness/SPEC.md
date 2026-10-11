# Search freshness and manual refresh

Status: approved; SF-01 in progress.

## Problem Statement

Live torrent searches can return an older backend snapshot even after new torrent content has committed. Browser reload currently requests that snapshot again. The search display has no general Refresh control, failed requests can leave loading active, and leaving the display does not dispose every owned request subscription.

## Solution

Every search load, including a browser reload, reads current committed searchable torrent content. An accessible manual Refresh control repeats the current search without changing filters, ordering or pagination. Loading settles after success, failure or cancellation, and leaving search stops owned work. No automatic refresh.

## User Stories

1. As a user, I want browser reload to show newly committed searchable torrents without waiting for cache expiry.
2. As a user, I want a Refresh control so I can fetch new results without reloading the application.
3. As a user, I want Refresh to preserve my query, filters, ordering and page.
4. As a user, I want a failed request to stop loading and show the existing error message.
5. As a user, I want to retry after an error without losing previously displayed results.
6. As a user, I want only the latest search request to update my results.
7. As a user, I want leaving search to stop its requests and subscriptions.
8. As a keyboard user, I want an accessible Refresh button with a visible loading/disabled state.
9. As a user, I want results to remain stationary until I change the search or explicitly refresh.

## Implementation Decisions

- Deepen the existing search datasource module; preserve its interface and Apollo adapter seam.
- Normal loads and manual refresh bypass backend result-cache reads and writes using the existing uncached query behavior. Browser result caching remains disabled.
- No backend schema changes or global cache shutdown. Existing cache callers outside live torrent search are unchanged.
- The datasource owns request cancellation, result publication, loading settlement and subscription disposal. The search display owner invokes disposal when destroyed, regardless of whether its table has rendered.
- Add one manual Refresh control using existing toolbar styling and translation conventions; disable it while loading.
- Preserve current filter/page/order inputs and existing result-selection behavior.
- Search freshness concerns committed searchable torrent content, not raw torrents still awaiting processing. Budgeted counts and facets may remain approximate under their existing contract.
- No production deployment or live configuration edits are included.

## Testing Decisions

Approved existing seams:

- Search datasource interface through its real RxJS behavior and a controlled Apollo adapter: observe results and loading, not private fields. Exercise updated backend data on reload/refresh, preserved criteria, failure then successful retry, request replacement and disposal.
- Actual built search display through the Refresh button and browser reload against controlled read responses. Observe request behavior and rendered rows, including a newly committed row, without mutating production.
- Prior art: existing Angular/Jasmine search display tests and existing datasource/reactive request patterns.
- Keep a small deterministic regression check for consumer-visible lifecycle behavior; use a throwaway browser smoke for the built display.
- The prior live cached/bypassed/cached comparison already proves the stale-result symptom; no redundant production reproduction is necessary.

## Out of Scope

Automatic polling, WebSockets, global cache invalidation, changes to classifier accuracy, changing estimated counts, bulk reclassification and deployment to Unraid.

## Further Notes

Live enabled backend cache TTL was ten minutes. All three paired search requests returned HTTP 200; uncached results were newer while repeated cached results remained identical. The local and live revisions matched at the recorded starting commit. Frontend dependencies were installed for implementation; no language server is configured. No new production module or adapter interface is needed.
