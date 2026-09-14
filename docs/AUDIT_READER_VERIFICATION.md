# Reader-based audit export verification

`models.VerifyAuditExportReader(io.Reader, []byte)` is an additive verification API. The existing `VerifyAuditExport([]byte, []byte)` delegates to it. Export generation, signed manifest/JSON formats, database schema, HTTP export and CLI input handling are unchanged.

The verifier keeps one decoded event instead of a complete decoded event array. Its memory use still depends on the largest individual JSON value, decoder lookahead and caller-provided manifest. The legacy wrapper and current CLI retain their input byte slices. This is not a constant-memory end-to-end CLI/export claim and does not shorten writer lock duration.

The signed manifest is authenticated first. Every original input byte, including trailing whitespace, passes through SHA256. Success requires the end of the array (or legacy empty `null`), actual EOF, matching hash/count/range, complete event hash-chain continuity and the existing signed checkpoint linkage. Sequence wraparound is rejected. Reader errors after otherwise valid JSON fail verification. No partially verified result is reported as successful.

Callers own input lifetime, close operations, deadlines and cancellation; this function cannot interrupt an arbitrary blocked Reader. Manifest input remains the existing byte-slice contract without a new implicit size limit. Future CLI reader wiring and manifest limits require their own compatibility/IO tests. Transaction-spooled streaming export generation remains separate work.

Tests cover chunked/non-seekable readers, valid JSON plus reader errors, whitespace beyond decoder lookahead, malformed/trailing input, legacy empty representations, signed count/range/checkpoint corruption, page-boundary event tampering and integer sequence wraparound. All ordinary repository checks and both authentic exact-head reviews remain required before merge. This change does not authorize publishing a paused release.
