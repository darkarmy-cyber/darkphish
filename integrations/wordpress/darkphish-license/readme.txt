=== Darkphish Licenses ===
Requires at least: 6.8
Requires PHP: 8.2
Stable tag: 0.2.2
License: MIT

Community registration, edition dashboards and manual Professional/Enterprise licensing with Ed25519 leases.

== Installation ==

Use a staging site first. Upload this folder as a ZIP in Plugins > Add New.
Requires HTTPS, PHP sodium, MySQL/MariaDB InnoDB and single-site WordPress.

Generate the signing key in Darkphish Licenses > Settings (version 0.1.1,
no SSH required), or with tools/create-key.php on the hosting server.
The destination must be outside its public document root, mode 0600, readable
by the PHP user. Configure DARKPHISH_LICENSE_KEY_FILE in wp-config.php.
Only the public verification keyring may be copied to Darkphish clients.

Configure DARKPHISH_TURNSTILE_SITE_KEY and DARKPHISH_TURNSTILE_SECRET in
wp-config.php. Never send the secret key or private signing file through chat.

Use the static-site registration page or a page with [darkphish_license]. For an external HTML site, configure DARKPHISH_LICENSE_REGISTRATION_ORIGIN in wp-config.php. Set the registration HTTPS URL, Community terms URL
and terms version in Darkphish Licenses > Settings. Verify actual mail delivery.

The complete Slovak deployment guide is INSTALL.md in this package and
integrations/wordpress/README.md in the source repository.

== Operational limits ==

Revocation blocks new leases. Previously signed offline leases remain valid
until their signed grace deadline (at most 60 days from issuance). Resetting
an installation does not invalidate its previously issued offline lease.

Deactivation retains license records and audit events. No uninstall erasure
is automatic. Exclude API request/response bodies from logs, analytics and caches.

Version 0.2.2 is an upgrade candidate; production SMTP delivery and commercial client activation still require deployment validation.
