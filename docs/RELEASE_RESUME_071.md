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
