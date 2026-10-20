- Fixed classifier parser natively (commits d0af495, 8af7f7a): hyphen no longer a language-token boundary (live-DB false 'sa' gone); new guarded episode forms (standalone E376/EP01, Spanish Cap.NNN with season decode, bracket '- 11 (720p)', leading 'S03E06 - Title' via loose fallback); +17 real extractions / 0 noise on 200-name sample; two-scout review closed; not deployed.

- Researched native Go release-name parsers and verified source activity/releases, licensing and Go requirements; shortlist and classifier-fit caveats saved in .scratch/classifier-parser-research/RESEARCH.md; no dependency or classifier changes.
- SF-01 complete: fresh reload/manual Refresh, retained criteria and error recovery, synchronous request/controller teardown; commits 60464ce and 5ab56ea; 84 tests, lint/build/display smoke passed; final Standards/Spec 0 findings; not deployed.
- Settled refresh behavior: manual control only; browser reload must fetch newly committed searchable torrents; drafted search-freshness spec.
- Completed six-candidate classifier/freshness architecture report in /tmp; opened and visually verified six before/after pairs and two Mermaid diagrams.
- Reproduced stale live search with cached/bypassed/cached reads; verified enabled ten-minute backend cache without live data mutations.
- Explored non-LLM classification and database-to-screen flow with two read-only scouts; ranked evidence, parsing, matching, lifecycle and dashboard opportunities.
- Initialized STATUS.md, TODO.md, and COMPLETE.md; recorded the current branch and starting commit.
