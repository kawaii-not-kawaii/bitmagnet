- Closed out parser work (2026-10-23): hand-labeled 200-name sample (labels audited against names, 3 mis-anchored regions fixed); measured TV recall 0.079→0.526 at 1.0 precision, movie 0.40 precision (all 3 FPs pre-existing year/date tokens); report .scratch/classifier-parser-research/labeled/LABELED-CHECK.md. Cleaned webui/dist churn. Built+verified local image bitmagnet:local-8af7f7a (banner f505667), archive ~/bitmagnet-local-8af7f7a.tar (sha256 e0902c4b…473e5); deploy-wizard.sh prepared for the human-only Unraid steps (no SSH/UI credentials on this host).

- Researched native Go release-name parsers and verified source activity/releases, licensing and Go requirements; shortlist and classifier-fit caveats saved in .scratch/classifier-parser-research/RESEARCH.md; no dependency or classifier changes.
- SF-01 complete: fresh reload/manual Refresh, retained criteria and error recovery, synchronous request/controller teardown; commits 60464ce and 5ab56ea; 84 tests, lint/build/display smoke passed; final Standards/Spec 0 findings; not deployed.
- Settled refresh behavior: manual control only; browser reload must fetch newly committed searchable torrents; drafted search-freshness spec.
- Completed six-candidate classifier/freshness architecture report in /tmp; opened and visually verified six before/after pairs and two Mermaid diagrams.
- Reproduced stale live search with cached/bypassed/cached reads; verified enabled ten-minute backend cache without live data mutations.
- Explored non-LLM classification and database-to-screen flow with two read-only scouts; ranked evidence, parsing, matching, lifecycle and dashboard opportunities.
- Initialized STATUS.md, TODO.md, and COMPLETE.md; recorded the current branch and starting commit.
