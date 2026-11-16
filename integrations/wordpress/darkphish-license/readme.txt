=== DarkPhish ===
Requires at least: 6.8
Requires PHP: 8.2
Stable tag: 0.2.8
License: MIT

Community registration, edition dashboards and manual Professional/Enterprise licensing with Ed25519 leases.

== Installation ==

Use a staging site first. Upload this folder as a ZIP in Plugins > Add New.
Requires HTTPS, PHP sodium, MySQL/MariaDB InnoDB and single-site WordPress.

Generate the signing key in DarkPhish > Settings (version 0.1.1,
no SSH required), or with tools/create-key.php on the hosting server.
The destination must be outside its public document root, mode 0600, readable
by the PHP user. Configure DARKPHISH_LICENSE_KEY_FILE in wp-config.php.
Only the public verification keyring may be copied to Darkphish clients.

Configure DARKPHISH_TURNSTILE_SITE_KEY and DARKPHISH_TURNSTILE_SECRET in
wp-config.php. Never send the secret key or private signing file through chat.

Use the static-site registration page or a page with [darkphish_license]. For an external HTML site, configure DARKPHISH_LICENSE_REGISTRATION_ORIGIN in wp-config.php. Set the registration HTTPS URL, Community terms URL
and terms version in DarkPhish > Settings. Verify actual mail delivery.

The complete Slovak deployment guide is INSTALL.md in this package and
integrations/wordpress/README.md in the source repository.

== Operational limits ==

Revocation blocks new leases. Previously signed offline leases remain valid
until their signed grace deadline (at most 60 days from issuance). Resetting
an installation does not invalidate its previously issued offline lease.

Deactivation retains license records and audit events. No uninstall erasure
is automatic. Exclude API request/response bodies from logs, analytics and caches.

Version 0.2.7: edition-prefixed license IDs, permanent delete, paid artifact download; production SMTP delivery and commercial client activation still require deployment validation.

Version 0.2.8: default disposable-domain blocklist expanded from 21 to 10,274 domains (disposable-email-domains snapshot 2026-11-16); the 1,000-domain save limit removed (text bound 1,000,000 characters); all contact emails and documentation links switched from darkphish.sk to darkphish.eu.

= 0.2.8 =
Default temporary email protection now ships the full 10,274-domain snapshot and settings accept it: the domain-count limit is gone and the textarea accepts up to 1,000,000 characters. Support and verification emails reference license@darkphish.eu; the admin panel and documentation link www.darkphish.eu. After updating, delete the saved darkphish_email_policy option (or paste tools/disposable-domains-2026-11-16.txt) so the new defaults apply. No signing key, wp-config.php or database change is required.

= 0.2.7 =
Renamed to DarkPhish. Editable, locally checked temporary email domain protection in Settings; starter list of 21 domains. Public registration and recovery return a clear mailbox message. Existing licenses and administrator issuance are unchanged.
