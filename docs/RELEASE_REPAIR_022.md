# v0.22.0 publication recovery

The immutable `v0.22.0` tag points to generated release merge
`5a39984942ca8920aca2bf2fc31ca3201a4173be`. GitHub Release
`399452243` retains the complete eight-asset draft after the recovery guard
withdrew its publication. The recovery workflow cannot currently identify a
qualifying Native release run, so ordinary engineering merges remain frozen.

The source's exact-SHA CI run `36616282423` completed at 19:08:29 UTC via
`workflow_dispatch`, and CodeQL run `36616297389` completed at 19:04:10 UTC
via `repository_dispatch`. The successful second attempt of Native release run
`36616244777` began its metadata job at 19:12:18 UTC and published at 19:18.
The existing recovery verifier compares those checks with the run's first
creation time, 19:01:14 UTC, instead of the successful attempt. Its publication
guard also accepts only `push` checks, even though the trusted release workflow
already allows the two dispatched main-check events.

This repair accepts only exact-source, completed canonical CI and CodeQL runs.
Dispatched runs must have the immutable GitHub Actions bot identity; the
successful release attempt must have a metadata job bound to the same run ID,
attempt and source commit. Required check completion must precede that job.
Existing generated-PR review, protected-main, tag, asset digest, receipt and
attestation checks remain in force. The repair neither moves the tag nor
replaces an existing asset.

Merge this change only through a separately authorized, reviewed release
repair. Once it is on protected `main`, let the normal recovery workflow
rebuild and verify the draft, then confirm the public release and all eight
assets before resuming ordinary merges.
