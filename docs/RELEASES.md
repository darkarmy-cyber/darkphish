# Version and release policy

`VERSION` is the authoritative Darkphish SemVer value. Release tags add `v`
(for example, `0.3.0` becomes `v0.3.0`), while the product UI and CLI shorten
normal releases to `Darkphish 0.3`. Unreleased binaries identify themselves as
`0.3-dev`; release builds also contain the commit SHA and UTC build timestamp.

Darkphish has an independent version history. Normal engineering batches
advance the minor component (`0.2.0` to `0.3.0`); patch releases are reserved
for exceptional fixes to an already released minor. The project never derives
its next number from historical Gophish versions.

Every runtime pull request adds a structured file under `changes/`. It may
target the current unreleased version or exactly the next minor. CI validates
the fragment and requires one for runtime changes. After merge to a protected
main branch, release preparation advances `VERSION` when needed, aggregates all
fragments into `CHANGELOG.md`, removes the consumed files, and opens a release
pull request with auto-merge enabled. Required checks and configured human
approval remain authoritative; automation does not bypass branch protection.

After the release pull request merges, the native release workflow reruns tests,
builds the supported OS/architecture archives with embedded build metadata,
generates SHA-256 checksums and an SPDX SBOM, attests the artifacts, creates the
`vX.Y.Z` tag, and publishes the GitHub Release. No release is published when a
required stage fails, and container publication is not part of this process.
