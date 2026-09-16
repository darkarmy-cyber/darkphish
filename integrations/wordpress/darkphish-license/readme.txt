=== Darkphish Community Licensing ===
Requires at least: 6.8
Requires PHP: 8.2
Stable tag: 0.1.0
License: MIT

Email-verified Community registration and Ed25519 activation leases.

== Installation ==

Use a staging site first. Upload this folder as a ZIP in Plugins > Add New.
Requires HTTPS, PHP sodium, MySQL/MariaDB InnoDB and single-site WordPress.

Generate the signing key with tools/create-key.php on the hosting server.
The destination must be outside its public document root, mode 0600, readable
by the PHP user. Configure DARKPHISH_LICENSE_KEY_FILE in wp-config.php.
Only the public verification keyring may be copied to Darkphish clients.

Configure DARKPHISH_TURNSTILE_SITE_KEY and DARKPHISH_TURNSTILE_SECRET in
wp-config.php. Never send the secret key or private signing file through chat.

Create a page with [darkphish_license]. Set its HTTPS URL, Community terms URL
and terms version in Settings > Darkphish licensing. Verify actual mail delivery.

The complete Slovak deployment guide is INSTALL.md in this package and
integrations/wordpress/README.md in the source repository.

== Operational limits ==

Revocation blocks new leases. Previously signed offline leases remain valid
until their signed grace deadline (at most 60 days from issuance). Resetting
an installation does not invalidate its previously issued offline lease.

Deactivation retains license records and audit events. No uninstall erasure
is automatic. Exclude API request/response bodies from logs, analytics and caches.

This staging candidate has not been deployed or validated on fsociety.sk.
