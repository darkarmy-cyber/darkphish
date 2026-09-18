# License required for sending

An official installation without a verified license cannot create or launch a
campaign or send a test email, including tests from Sending Profiles. The API
returns HTTP 403 with activation guidance before processing the send request.
The worker rejects direct calls too; a missing license manager fails closed.

The dashboard, Campaigns and Sending Profiles show an activation warning.
Launch and both test-email controls are disabled in the server-rendered HTML.
Administrators receive a Settings > Licensing link; other users are directed
to their administrator. Reload the sending page after activation or renewal.
Reading existing data and configuring the installation remain available.

The production SMTP worker rechecks the signed lease before each connection,
reconnection and message. Missing, invalid or expired leases block delivery.
The existing signed offline grace period remains valid; this is not a free
unactivated trial. Unsent campaign recipients are unlocked and retained for a
later licensed retry, without consuming send attempts. Test-email requests get
an error and are not replayed automatically. Already accepted SMTP deliveries
cannot be recalled; their results are still recorded correctly.

Tests use ephemeral local signing keys and fake mailers only. No real license
activation, production migration, SMTP delivery or server deployment is needed.
