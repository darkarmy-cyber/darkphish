# Bounded release upload reconciliation

On 2026-09-17, native release run 35271305546 received HTTP 500 while
uploading `darkphish-v0.13.0-darwin-amd64.tar.gz`. GitHub asset 571073334
was nevertheless marked `uploaded`, with size 30886151 and a SHA-256 digest.
Its creation/update timestamps span 20:36:31Z to 20:38:29Z (118 seconds).
Recovery run 35271305559 attempts 1 and 2 stopped at their 30-second deadline.
All eight non-publication jobs passed in each attempt. This is not evidence
of a global GitHub outage or of a fault in the application binaries.

Both publishers now use one shared upload helper with a 180-second deadline.
The helper validates the exact GitHub upload repository/release endpoint,
forbids redirects, sets the byte length and cancels response bodies to release
connection resources. The latter follows the Node/Undici guidance at
https://github.com/nodejs/undici#garbage-collection; it is defensive cleanup,
not a demonstrated cause of this incident.

An upload POST is issued exactly once. After HTTP 201, a transport exception
or HTTP 5xx, at most three read-only asset-list probes (two-second intervals)
must establish a single completed asset with the exact name, byte length,
SHA-256 and immutable GitHub Actions uploader identity. Missing, incomplete,
mismatching, duplicate or unreadable results stop the operation. Other HTTP
statuses fail immediately. This does not retry a POST, delete a starter asset,
replace an archive, publish an unverified draft or change a tag.

Existing whole-release asset-set, source, tag, protected-main, review and
attestation checks remain required before publication. Asset-list checks are
not atomic with subsequent API mutations; final publication guards remain
essential. Persistent upload failure still requires investigation, not an
unbounded retry. The v0.13.0 tag/source stays immutable; this automation-only
repair runs from reviewed protected main when recovering that historical
source. It does not change VERSION or start new application development. The
required changelog fragment targets the next minor because the v0.13 source is
already immutable; it records the automation repair without rewriting that tag.
