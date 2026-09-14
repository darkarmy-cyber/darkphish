# Resume the verified v0.7.1 release

This is the final, explicitly maintainer-authorized recovery stage for v0.7.1.
It must preserve the immutable release source and reuse the original verified
artifact bytes. It must not delete duplicate historical drafts, replace assets,
move tags, bypass branch protection or fabricate review evidence.

The selected draft is 388329244, source
`73bf5948ed19cd918638453d478ef3f1cbe83d89`, checksum manifest
`d41dea173bb71a7230b26bd10b60209285a9e9892a07e7267dd8e46c63a8e7b7`.
All eight files were downloaded and verified against SHA256, the publication
receipt and GitHub attestations from recovery run 34835562399 attempt 1, signed
by workflow commit `d8d524727401327689ffd8ce113d830ac64a4fa4`.
That run completed all five native builds, tests and audit smoke. Exact-source
CI and CodeQL completed before the recovery run. The Windows archive reports
version 0.7.1 and the exact release source; release PR31 maintainer review was
independently verified. This recorded evidence is not itself permission to
publish: the finalizer must revalidate the live state.

PR58 is the sole finalization exception, tied to base
`88377d6951ede7352acb257777250bdfecd2052b` and its named branch. It removes only
the temporary normalization hold while leaving the pending-publication freeze
for ordinary engineering and dependency merges intact. All existing review,
CI, CodeQL and protected synchronous merge gates remain mandatory.

The resumption workflow executes only immutable protected-main code after CI or
CodeQL completion and uses the shared `protected-main-mutation` concurrency
group. It additionally requires main to be the exact reviewed PR58 merge and
verifies release PR31, source/tag identity, canonical notes, receipt, all asset
bytes and their original workflow attestations. It repeats checks before and
after changing only the selected release's draft state. No artifact is uploaded
or replaced. Historical draft 388031151 remains untouched.

The original publication timestamp must remain `2026-09-14T11:01:32Z`, so all
existing downstream publication-provenance guards remain authoritative. If
GitHub changes that timestamp or any other post-publication check fails, the
workflow withdraws this exact release back to draft and fails closed. It does
not invent historical timing evidence or relax any existing provenance check.

Both generic native/recovery publishers are explicitly held while VERSION is
0.7.1; they cannot race resumption by rebuilding another draft. Normal behavior
returns for the next version. Later main commits skip the one-shot resumption.
Failed publication withdrawal retries PATCH plus direct-ID, release collection
and tag probes, with a critical failure if private state cannot be confirmed.

A staggered 15-minute default-main schedule is the durable retry path if GitHub
replaces a pending concurrency-group member. It uses immutable `github.sha`,
the same authorization/verification rules and the same serialization group;
later main commits no-op. There is no branch-dispatchable privileged entry point.
