# Email image import

DarkPhish imports supported image resources while an administrator explicitly imports a complete raw RFC 5322/MIME message in **Email Templates → Import Email Source**. There is no separate **Load external images** action.

## What is imported

- MIME inline PNG, JPEG and GIF resources referenced through `cid:` URLs are decoded, validated, normalized to PNG, stored as template attachments and rewritten to stable DarkPhish CID names.
- Public HTTPS PNG, JPEG and GIF resources referenced by `src`, supported `srcset` entries, inline CSS `url(...)` values or style blocks are fetched by the DarkPhish server during the explicit import operation. They are validated, normalized to PNG, stored as embedded template attachments and rewritten to CID references.
- Safe base64 PNG/JPEG/GIF data images are normalized and converted to CID attachments.
- Ordinary MIME attachments with filenames are preserved subject to the normal template attachment limits.

The editor renders imported CID image attachments as local `data:` previews. Before the template is saved, those local previews are restored to CID references, so an actually sent message uses embedded MIME resources rather than browser-only preview data.

## Network and content safety

Automatic image retrieval retains the existing restricted network boundary:

- HTTPS only, port 443, without URL credentials.
- Every hostname is resolved once per redirect hop and connections are pinned to the validated public numeric addresses.
- Private, loopback, link-local, metadata and mixed public/private DNS answers are rejected.
- Redirects are revalidated and bounded.
- Browser cookies, administrator credentials, proxy environment variables and referrers are not forwarded.
- Per-image byte, pixel and request deadlines remain bounded. The overall import also has a deadline and total attachment/resource limits.
- Active SVG and unsupported image formats are not executed or silently trusted. Unsupported or unavailable resources remain unchanged and are reported as import warnings.

Importing a message can contact the image hosts referenced by that message and can therefore register an image request or tracking event. This happens only when an authenticated operator explicitly starts the import.

## Limits and compatibility

The importer is intentionally fail-safe. Unsupported, authenticated, expired, private or oversized images are not fetched around their access controls. The raw source and MIME structure must fit the configured request/attachment limits. External image failures do not discard the rest of the email; the import completes with a concise warning.

The previous authenticated endpoint `POST /api/import/email/images` and its manual preview UI were removed in 0.22. Image localization is now part of the normal `POST /api/import/email` operation, which continues to require template write permission.
