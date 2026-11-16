---
category: Fixed
version: 0.27.1
---
- Restored Import Site fidelity for modern pages such as the Google sign-in flow: sanitized imports now keep stylesheet blocks (with @import stripped and every url() resolved through the HTTPS-only policy), a curated passive SVG subset so logos survive, srcset image candidates, framework class names, and CSS custom properties such as var()/calc() in inline styles. Forms remain forced back to the landing-page endpoint, scripts, event handlers and active SVG content stay removed, and inline remote url() references stay blocked.
- Rendered (Chromium snapshot) imports work with Chromium 111+: the DevTools websocket handshake now announces a fixed loopback origin and the launcher passes --remote-allow-origins for exactly that origin, the render settle window was extended so heavy pages reach the load event, and the renderer now logs which stage failed (port, dial, setup, navigate, settle, capture) plus the Chromium stderr tail and observed response status to the application log, instead of a single opaque warning.
