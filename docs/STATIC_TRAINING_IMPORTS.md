# Static training imports

New **Import Site** operations create visibly labelled, inert training pages.
This is intentionally not a working clone of a remote application.

## Supported content

Text (including Unicode and literal template-like braces), basic layout/table
elements and an allowlist of inline CSS properties are retained. Forms and
controls become non-editable visual placeholders. Field values, names, event
handlers, scripts, frames, executable/vector content, external stylesheets and
active links are removed. Relative image URLs are resolved against the final
fetched page URL, not a remote HTML base element.

Dynamic applications and designs relying on external stylesheets will differ
from the original. Do not promise pixel-identical imports of arbitrary websites.
Inline SVG, WebP, CSS background images, srcset, CID attachments and resources
requiring authentication are not supported by this static image workflow.

## Images and privacy

Import itself does not fetch subresources. **Load verified images** explicitly
requests up to 12 public HTTPS PNG/JPEG/GIF images through the existing
authenticated raster verifier. Its TLS, redirect, destination, concurrency,
timeout, byte and pixel limits remain in force. Private/internal destinations
are not enabled. Remote hosts can observe these server requests.

Verified images are re-encoded as PNG and embedded into this static page. They
survive Source/WYSIWYG editing and save/reopen without remote browser requests.
Failed images do not starve later batches and can be retried. Review the status
message for unavailable or unsupported resources.

Existing **Email Templates** retain their separate **Load external images**
preview workflow and original email URLs. This change does not add MIME/CID
attachment extraction or change email sending behavior.

## Safety and persistence

The JSON page property training_static opts into this restricted mode. The
resulting HTML carries a static-v1 body marker, so no database migration is
needed. Page reads expose the mode; updates to an already static page remain
static even if the caller removes its marker or sends training_static=false.
Copying a static page in the UI retains this mode.

Saving sanitizes the HTML again and clears capture flags and redirect URLs.
Rendering sanitizes again and writes literal HTML rather than executing Go
template expressions. Literal backslashes/braces therefore cannot cause a Go
template parser failure in static pages. Template substitutions are deliberately
unavailable in this mode.

Participant responses use a restrictive sandboxed CSP: no scripts, external
resources, forms or navigation targets, with embedded raster images and inline
styles only. Non-GET submissions are rejected before submission/credential event
handling. Ordinary page visits may still be measured by the existing campaign
click event; this is not a form-submission or credential-collection exercise.

Existing legacy/manual pages are not automatically converted or deleted.
Re-import as a new static page (or explicitly save with training_static=true)
to adopt this mode. Do not interpret this change as disabling all legacy
credential features throughout the installation.

## SMTP and interface notes

The redundant Workspace sidebar heading is removed. Import launch/confirmation
buttons use a contrast-tested deep teal without changing the classic layout.
SMTP 535 failures have clearer, escaped diagnostics; no authentication, retry,
TLS or sending behavior is changed. SMTP username requirements remain provider
specific, commonly the full mailbox address.
