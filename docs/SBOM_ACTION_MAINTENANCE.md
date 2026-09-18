# Node24 SBOM action maintenance

The v0.14 publisher emitted a Node20 runtime deprecation annotation for the
previous pinned SBOM action. Both publishers now pin upstream
`anchore/sbom-action` v0.24.2 at commit
`3ad7283483fc7af8ff2b4ea19663c2d5ca935e26`. Upstream introduced Node24 in
[v0.24.0](https://github.com/anchore/sbom-action/releases/tag/v0.24.0);
[v0.24.2](https://github.com/anchore/sbom-action/releases/tag/v0.24.2)
identifies the adopted commit. GitHub reports that commit's signature valid.
The annotated tag itself is unsigned; no signed-tag claim is made.

Existing path, SPDX format and output-file inputs remain supported and unchanged.
Artifact upload and release upload remain disabled; dependency snapshot upload
is explicitly disabled too (the upstream default was already false). The upstream
runtime, bundled implementation and Syft dependencies change; new SBOM bytes are
not expected to be identical to older SBOMs. No existing release is regenerated.

Historical native recovery must recognize its literal action step name. The
gate admits exactly one successful completed SBOM step from either the old
v0.20.6 pin or the newly reviewed v0.24.2 pin. Unknown pins, floating tags,
duplicates, failures and renamed steps fail closed. The remaining source,
review, five-platform build, smoke, checksum and publication gates are unchanged.
Future pin upgrades must update this explicit compatibility boundary and tests.

A separate read-only PR/main smoke workflow invokes the exact action on a
dependency-only fixture and validates the resulting SPDX. It has contents-read
permission, no privileged trigger, no persisted checkout credentials and all
three upload paths disabled. It cannot publish a release or dependency snapshot.

This is automation-only maintenance: no VERSION change, changelog fragment,
application feature, release-triggering version bump, tag/asset replacement or
production operation. Previous public releases and partial drafts are preserved.
