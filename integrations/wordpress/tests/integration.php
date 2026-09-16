<?php
declare(strict_types=1);
// Disposable WordPress/database only. Mail and anti-abuse calls are intercepted.
$root = getenv('DARKPHISH_WP_TEST_ROOT');
if (!$root || !is_file($root . '/wp-load.php') || !getenv('DARKPHISH_WP_TEST_DB')) { throw new RuntimeException('Explicit test WordPress/database required'); }
define('DB_NAME', getenv('DARKPHISH_WP_TEST_DB'));
define('DB_USER', getenv('DARKPHISH_WP_TEST_USER') ?: 'root');
define('DB_PASSWORD', getenv('DARKPHISH_WP_TEST_PASSWORD') ?: 'root');
define('DB_HOST', '127.0.0.1');
define('DB_CHARSET', 'utf8mb4');
define('DB_COLLATE', '');
define('AUTH_KEY', 'isolated-test-only-auth-key');
define('AUTH_SALT', 'isolated-test-only-auth-salt');
define('WP_INSTALLING', true);
define('WP_DEBUG', true);
define('WP_DEBUG_DISPLAY', false);
define('WP_HOME', 'https://fsociety.test');
define('WP_SITEURL', 'https://fsociety.test');
define('ABSPATH', rtrim($root, '/\\') . '/');
define('DARKPHISH_TURNSTILE_SECRET', 'test-only-stub');
define('DARKPHISH_TURNSTILE_SITE_KEY', 'test-public-site-key');
define('DARKPHISH_LICENSE_REGISTRATION_ORIGIN', 'https://darkphish.test');
$table_prefix = 'wp_';
$_SERVER['HTTPS'] = 'on'; $_SERVER['HTTP_HOST'] = 'fsociety.test'; $_SERVER['REMOTE_ADDR'] = '127.0.0.1'; $_SERVER['DOCUMENT_ROOT'] = ABSPATH;
$keyPath = getenv('DARKPHISH_WP_TEST_KEY');
if (!$keyPath) { throw new RuntimeException('Test key path required'); }
define('DARKPHISH_LICENSE_KEY_FILE', $keyPath);
require_once ABSPATH . 'wp-settings.php';
require_once ABSPATH . 'wp-admin/includes/upgrade.php';
require_once ABSPATH . 'wp-admin/includes/plugin.php';
// wp_install sends an administrator notification; never deliver mail in tests.
add_filter('pre_wp_mail', static fn () => true, -100);
if (!is_blog_installed()) { wp_install('License tests', 'testadmin', 'admin@example.test', true, '', 'test-only-password-not-for-production'); }
$activated = activate_plugin('darkphish-license/darkphish-license.php');
if (is_wp_error($activated)) { throw new RuntimeException('Activation failed: ' . $activated->get_error_message()); }
// WP_INSTALLING intentionally omits active plugins on subsequent subprocess boots.
require_once ABSPATH . 'wp-content/plugins/darkphish-license/darkphish-license.php';

function check(bool $condition, string $message): void { if (!$condition) { throw new RuntimeException($message); } }
function call_api(string $operation, array|string $data, string $origin = '', string $method = 'POST'): WP_REST_Response {
    $request = new WP_REST_Request($method, '/darkphish-license/v1/' . $operation);
    $request->set_header('Content-Type', 'application/json');
    if ($origin !== '') { $request->set_header('Origin', $origin); }
    $request->set_body(is_string($data) ? $data : json_encode($data, JSON_THROW_ON_ERROR));
    return rest_do_request($request);
}

// Used only by the disposable local HTTP harness to verify real emitted CORS headers.
if (PHP_SAPI === 'cli-server') {
    rest_get_server()->serve_request(parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH));
    return;
}
// Subprocesses compete for the same license using independent DB connections.
if (($argv[1] ?? '') === 'compete') {
    $response = call_api('activate', ['license_key' => getenv('DARKPHISH_WP_TEST_LICENSE'), 'installation_id' => $argv[2], 'product_version' => '0.11.0']);
    echo 'STATUS:' . $response->get_status() . "\n"; exit;
}

$db = new Darkphish\Licensing\Store($wpdb);
$db->install(); // upgrade/install idempotency
class AdminDenied extends RuntimeException {}
add_filter('wp_die_handler', static fn () => static function () { throw new AdminDenied('denied'); });
$_SERVER['REQUEST_METHOD'] = 'POST';
$_REQUEST = [];
wp_set_current_user(0);
try { Darkphish\Licensing\initializeSigningKey(); throw new LogicException('Anonymous key setup accepted'); }
catch (AdminDenied $expected) {}
wp_set_current_user(1);
try { Darkphish\Licensing\initializeSigningKey(); throw new LogicException('Key setup without nonce accepted'); }
catch (AdminDenied $expected) {}
check(!file_exists($keyPath), 'Unauthorized setup created a private key');
$_REQUEST['_wpnonce'] = wp_create_nonce('darkphish-license-initialize-key');
Darkphish\Licensing\initializeSigningKey();
$keyDigest = hash_file('sha256', $keyPath);
try { Darkphish\Licensing\initializeSigningKey(); throw new LogicException('Existing key replaced'); }
catch (RuntimeException $expected) {}
check(hash_file('sha256', $keyPath) === $keyDigest, 'Setup changed an existing private key');
wp_set_current_user(0); $_REQUEST = [];
$service = new Darkphish\Licensing\Service($db, Darkphish\Licensing\signer());
$settings = ['registration_url' => 'https://darkphish.test/licencia/', 'terms_url' => 'https://fsociety.test/terms/', 'terms_version' => 'test-v1'];
update_option('darkphish_license_settings', $settings);
$mails = [];
add_filter('pre_wp_mail', function ($return, array $mail) use (&$mails) { $mails[] = $mail; return true; }, 10, 2);
$challengeHostname = 'wrong.test';
add_filter('pre_http_request', function ($pre, array $args, string $url) use (&$challengeHostname) {
    if ($url !== 'https://challenges.cloudflare.com/turnstile/v0/siteverify') { throw new RuntimeException('Unexpected external network call'); }
    return ['response' => ['code' => 200], 'body' => json_encode(['success' => ($args['body']['response'] ?? '') === 'valid-test-token', 'hostname' => $challengeHostname, 'action' => 'darkphish-license'])];
}, 10, 3);

define('WP_ADMIN', true);
do_action('admin_init');
check(sanitize_option('darkphish_license_settings', $settings)['registration_url'] === $settings['registration_url'], 'Approved static registration URL rejected');
foreach (['https://evil.test/licencia/', 'https://darkphish.test.evil.test/licencia/', 'http://darkphish.test/licencia/', 'https://darkphish.test/licencia/#token', 'https://user@darkphish.test/licencia/'] as $bad) {
    check(sanitize_option('darkphish_license_settings', array_replace($settings, ['registration_url' => $bad]))['registration_url'] === '', 'Untrusted email destination accepted');
}
$publicConfig = call_api('public-config', [], 'https://darkphish.test', 'GET');
check($publicConfig->get_status() === 200 && $publicConfig->get_data()['site_key'] === 'test-public-site-key', 'Public config unavailable');
check(!str_contains(json_encode($publicConfig->get_data()), DARKPHISH_TURNSTILE_SECRET), 'Public config leaked secret');
foreach (['https://evil.test', 'null', 'http://darkphish.test', 'https://darkphish.test.evil.test', 'https://darkphish.test:444'] as $origin) {
    check(call_api('public-config', [], $origin, 'GET')->get_status() === 403, 'Untrusted browser origin accepted');
}
check(call_api('request', ['email' => 'blocked@example.test'], 'https://evil.test')->get_status() === 403, 'Untrusted request dispatched');
$request = ['email' => 'owner@example.test', 'terms_version' => 'test-v1', 'challenge_token' => 'invalid'];
$invalidChallenge = call_api('request', $request);
check($invalidChallenge->get_status() === 400 && count($mails) === 0, 'Invalid challenge status: ' . $invalidChallenge->get_status());
$request['challenge_token'] = 'valid-test-token';
check(call_api('request', $request, 'https://darkphish.test')->get_status() === 400 && count($mails) === 0, 'Wrong Turnstile hostname accepted');
$challengeHostname = 'darkphish.test';
check(call_api('request', $request, 'https://darkphish.test')->get_status() === 202 && count($mails) === 1, 'Registration email missing');
preg_match('/#dp-verify=([A-Za-z0-9_-]{43})/', $mails[0]['message'], $match);
check(isset($match[1]) && str_contains($mails[0]['message'], 'https://darkphish.test/licencia/#dp-verify='), 'Static-site verification link missing');
$verified = call_api('verify', ['token' => $match[1]]);
check($verified->get_status() === 200, 'Email verification failed');
$license = $verified->get_data();
check(call_api('verify', ['token' => $match[1]])->get_status() === 403, 'Verification token replay accepted');
$installation = '12345678-1234-1234-1234-123456789012';
$activation = ['license_key' => $license['license_key'], 'installation_id' => $installation, 'product_version' => '0.11.0'];
$activated = call_api('activate', $activation);
check($activated->get_status() === 200 && !empty($activated->get_data()['refresh_token']), 'Activation failed');
check(str_contains($activated->get_headers()['Cache-Control'] ?? '', 'no-store'), 'Credential response cacheable');
$envelope = $activated->get_data()['lease'];
$public = base64_decode(array_values(Darkphish\Licensing\signer()->keyring()['keys'])[0]);
check(sodium_crypto_sign_verify_detached(base64_decode(strtr($envelope['signature'], '-_', '+/')), base64_decode(strtr($envelope['payload'], '-_', '+/')), $public), 'Activation signature invalid');
$second = array_replace($activation, ['installation_id' => '87654321-1234-1234-1234-123456789012']);
check(call_api('activate', $second)->get_status() === 403, 'Second installation admitted');
$refresh = ['refresh_token' => $activated->get_data()['refresh_token'], 'installation_id' => $installation, 'product_version' => '0.11.0'];
$refreshed = call_api('refresh', $refresh);
check($refreshed->get_status() === 200 && !isset($refreshed->get_data()['refresh_token']), 'Stable refresh credential contract broken');
check(call_api('refresh', array_replace($refresh, ['installation_id' => $second['installation_id']]))->get_status() === 403, 'Refresh binding bypass');
check(call_api('activate', json_encode($activation) . '{}')->get_status() === 400, 'Trailing JSON accepted');
check(call_api('activate', array_replace($activation, ['admin' => 'yes']))->get_status() === 400, 'Unknown request field accepted');
$stored = $db->find('licenses', 'id', $license['license_id']);
check(!str_contains(json_encode($stored), $license['license_key']) && !str_contains(json_encode($stored), $refresh['refresh_token']), 'Raw credentials persisted');

(new Darkphish\Licensing\Service($db))->administer($license['license_id'], 'revoke', 1, time());
check(call_api('activate', $activation)->get_status() === 403 && call_api('refresh', $refresh)->get_status() === 403, 'Revocation did not stop issuance');
$revokedToken = $service->request('owner@example.test', 'test-v1', time());
check(call_api('verify', ['token' => $revokedToken])->get_status() === 403, 'Email reissue bypassed revocation');
$service->administer($license['license_id'], 'restore', 1, time());
$service->administer($license['license_id'], 'reset', 1, time());
check(call_api('refresh', $refresh)->get_status() === 403, 'Reset retained old refresh token');

putenv('DARKPHISH_WP_TEST_LICENSE=' . $license['license_key']);
$workers = [];
foreach ([$installation, $second['installation_id']] as $id) {
    $pipes = [];
    $process = proc_open([PHP_BINARY, __FILE__, 'compete', $id], [0 => ['pipe', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']], $pipes);
    check(is_resource($process), 'Could not start competing process'); fclose($pipes[0]);
    $workers[] = [$process, $pipes];
}
$statuses = [];
foreach ($workers as [$process, $pipes]) {
    $out = stream_get_contents($pipes[1]); $err = stream_get_contents($pipes[2]); fclose($pipes[1]); fclose($pipes[2]);
    check(proc_close($process) === 0, 'Competing process failed: ' . $err);
    preg_match('/STATUS:(\d+)/', $out, $found); $statuses[] = (int) ($found[1] ?? 0);
}
sort($statuses); check($statuses === [200, 403], 'Unexpected competing activation statuses: ' . json_encode($statuses));

// Audit insertion failure must roll back every license mutation.
$before = $db->find('licenses', 'id', $license['license_id']);
$wpdb->query("CREATE TRIGGER darkphish_test_reject_event BEFORE INSERT ON {$db->prefix}events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test event failure'");
$wpdb->suppress_errors(true);
try { $service->administer($license['license_id'], 'revoke', 1, time()); throw new LogicException('Audit failure did not abort'); }
catch (RuntimeException $expected) {}
finally { $wpdb->query('DROP TRIGGER darkphish_test_reject_event'); }
check($db->find('licenses', 'id', $license['license_id']) === $before, 'Audit failure left a partial update');
check($db->rate('test', 'same-client', 1, 3600, time()) && !$db->rate('test', 'same-client', 1, 3600, time()), 'Rate limiting did not enforce limit');

// Exercise the registered admin handler's permission and CSRF gates without
// following its successful redirect/exit path.
$_SERVER['REQUEST_METHOD'] = 'POST';
$_POST = ['license_id' => $license['license_id'], 'operation' => 'revoke'];
wp_set_current_user(0);
try { do_action('admin_post_darkphish_license_admin'); throw new LogicException('Anonymous admin action accepted'); }
catch (AdminDenied $expected) {}
wp_set_current_user(1);
$_REQUEST = [];
try { do_action('admin_post_darkphish_license_admin'); throw new LogicException('Admin action without nonce accepted'); }
catch (AdminDenied $expected) {}
wp_set_current_user(0);
check($db->find('licenses', 'id', $license['license_id']) === $before, 'Rejected admin request mutated license');

require __DIR__ . '/admin-editions.php';
require __DIR__ . '/mail.php';

$_SERVER['HTTPS'] = 'off';
check(call_api('activate', $activation)->get_status() === 503, 'Plaintext issuance accepted');
echo "WordPress integration passed: registration, verification, replay rejection, signing, activation, refresh, binding, revocation, reset, competing processes, audit rollback and rate limits.\n";
