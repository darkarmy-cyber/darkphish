# Email image previews

The administrative Content Security Policy intentionally blocks remote images.
Importing a raw email preserves its HTML image URLs, but does not automatically
contact external image hosts. A broken image in the editor does not by itself
mean its URL was lost or that it will be broken in the recipient's email client.

In **Email Templates → HTML**, choose **Load external images** to request
temporary previews. This works for both imported and existing templates. The
request comes from the DarkPhish server, without your browser cookies,
credentials or referrer. It still reveals the server's address to the image
host, and a tracking image URL may register an open. Only load content you are
authorized to inspect. No images are requested just by opening/importing a
template or switching editor modes.

The preview is a validated, re-encoded PNG displayed through the existing
`img-src 'self' data:` policy. The original `src` is retained by CKEditor and
restored in Source and saved HTML. Previews stay in memory for the current
editing session; they are not stored in the database or attached to outgoing
email. Closing or replacing a template discards the cache and ignores late
responses. Previously approved previews survive Source/HTML switching without
another network request.

## Limits and failures

- Any public HTTPS image host, port 443, with no domain allowlist or URL
  credentials. Each redirect is resolved and validated separately. Connections
  are pinned to that hop's public numeric DNS answers (mixed public/private
  answers are rejected), with the original hostname retained for HTTP virtual
  hosting and TLS certificate validation. No second hostname lookup is used
  when connecting. Internal-host exceptions configured for site imports do not
  apply to image previews.
- Valid PNG, JPEG or GIF input, at most 1 MiB and 1 million pixels per image.
  Animated GIF previews show the first frame. Re-encoded output is also bounded
  to 1 MiB. SVG, HTML and other formats are not accepted.
- At most 12 distinct URLs per request, an eight-second per-image timeout and
  a twenty-second total deadline. Two preview batches can run concurrently.
  The authenticated sensitive-operation rate limit also applies.
- Unavailable, authenticated, expired or blocked remote URLs remain unavailable;
  DarkPhish does not bypass image-host access controls.
- CID attachments, relative URLs, CSS background images and `srcset` are not
  converted by this preview operation. Import the complete raw email for MIME
  parsing; this change does not add extraction of CID attachments.

The API endpoint is `POST /api/import/email/images` with
`{"urls":["https://example.test/image.png"]}`. It requires the existing API
authentication and write restrictions; PATs need `templates:write`. Results and
errors use `Cache-Control: no-store`. The server does not log upstream response
bodies, URL query values or network diagnostics. No public image proxy or
anonymous GET endpoint is provided.
