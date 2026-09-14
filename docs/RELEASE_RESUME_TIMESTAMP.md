# Verified release resumption timestamp correction

The maintainer approved narrow, reviewed recovery of v0.7.1 without protection bypasses or replacement artifacts. PR58 merged as 697fe42eb20f3f5197be9545ed5b3bf835b730c0 with all ten checks and both authentic reviews. Main CI and CodeQL passed.

Run 34857439793 attempt 2 then verified the original release and published it. GitHub changed `published_at` from 2026-09-14T11:01:32Z to 2026-09-14T14:45:24Z. The fixed-time assertion failed; withdrawal succeeded and release 388329244 is draft again. No asset or tag was changed.

The timestamp is mutable publication metadata, not artifact authorization. The selected eight original assets remain authorized only by their immutable identities, pinned manifest, receipt, full original recovery-run attestations, source PR proof, current-main checks and authentic reviews. This correction must accept GitHub's legitimate publication timestamp transition while rejecting malformed/backdated/future timestamps and changes during closing validation. It must never fabricate or backdate API metadata.

The repair remains a one-shot exact-base, exact-branch, allowlisted-path operation. All main protections, ten checks, zero-alert gate and both authentic exact-head reviews remain mandatory. No production deployment, tag movement, new artifacts, or other release mutation is authorized.
