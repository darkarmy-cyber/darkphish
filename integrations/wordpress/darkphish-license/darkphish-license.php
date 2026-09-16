<?php
/**
 * Plugin Name: Darkphish Community Licensing
 * Description: Verified Community registration and signed installation leases for Darkphish.
 * Version: 0.1.2
 * Requires at least: 6.8
 * Requires PHP: 8.2
 * License: MIT
 */
declare(strict_types=1);
namespace Darkphish\Licensing;
if (!defined('ABSPATH')) { exit; }
require_once __DIR__ . '/includes/Signer.php';
require_once __DIR__ . '/includes/KeySetup.php';
require_once __DIR__ . '/includes/Store.php';
require_once __DIR__ . '/includes/Service.php';
require_once __DIR__ . '/includes/Browser.php';

function store(): Store { global $wpdb; return new Store($wpdb); }
function signer(): Signer {
    if (!function_exists('sodium_crypto_sign_detached') || !defined('DARKPHISH_LICENSE_KEY_FILE')) {
        throw new \RuntimeException('Signing is not configured');
    }
    // For WordPress in a subdirectory, DOCUMENT_ROOT also excludes sibling public files.
    $root = $_SERVER['DOCUMENT_ROOT'] ?? '';
    return Signer::fromFile(DARKPHISH_LICENSE_KEY_FILE, $root !== '' ? $root : ABSPATH);
}
function service(): Service { return new Service(store(), signer()); }

function initializeSigningKey(): void {
    if (($_SERVER['REQUEST_METHOD'] ?? '') !== 'POST' || !is_ssl() || !current_user_can('manage_options')) {
        wp_die('Forbidden', '', ['response' => 403]);
    }
    check_admin_referer('darkphish-license-initialize-key');
    if (!defined('DARKPHISH_LICENSE_KEY_FILE')) { throw new \RuntimeException('Configure the private key path first'); }
    $roots = [ABSPATH];
    if (!empty($_SERVER['DOCUMENT_ROOT'])) { $roots[] = $_SERVER['DOCUMENT_ROOT']; }
    $id = 'DP-COM-' . gmdate('Ymd') . '-' . bin2hex(random_bytes(4));
    $db = store();
    $db->event('', 'signing.initialize.request', get_current_user_id());
    KeySetup::create(DARKPHISH_LICENSE_KEY_FILE, $id, $roots);
    $db->event('', 'signing.initialize.success', get_current_user_id());
}

add_action('admin_post_darkphish_license_initialize_key', function (): void {
    try { initializeSigningKey(); }
    catch (\Throwable $error) { wp_die('Kľúč sa nepodarilo vytvoriť. Skontrolujte nakonfigurovanú súkromnú cestu, práva a či súbor už existuje. Existujúci kľúč sa neprepisuje.'); }
    wp_safe_redirect(admin_url('options-general.php?page=darkphish-license')); exit;
});

register_activation_hook(__FILE__, function (bool $networkWide): void {
    if ($networkWide || is_multisite()) { wp_die('Activate only on a single-site WordPress installation.'); }
    if (!function_exists('sodium_crypto_sign_detached')) { wp_die('PHP sodium is required.'); }
    store()->install();
    if (!wp_next_scheduled('darkphish_license_cleanup')) { wp_schedule_event(time() + 3600, 'hourly', 'darkphish_license_cleanup'); }
});
register_deactivation_hook(__FILE__, function (): void { wp_clear_scheduled_hook('darkphish_license_cleanup'); });
add_action('darkphish_license_cleanup', function (): void { store()->cleanup(); });

function reply(array $body, int $status = 200): \WP_REST_Response {
    return new \WP_REST_Response($body, $status, ['Cache-Control' => 'no-store, private', 'Pragma' => 'no-cache', 'X-Content-Type-Options' => 'nosniff', 'Referrer-Policy' => 'no-referrer']);
}

function input(\WP_REST_Request $request, array $fields): array {
    if (strlen($request->get_body()) > 8192 || !str_contains(strtolower((string) $request->get_header('content-type')), 'application/json')) {
        throw new \InvalidArgumentException('Invalid request');
    }
    $data = json_decode($request->get_body(), true, 8, JSON_THROW_ON_ERROR);
    if (!is_array($data) || array_diff(array_keys($data), $fields)) { throw new \InvalidArgumentException('Invalid request'); }
    foreach ($fields as $field) {
        if (!isset($data[$field]) || !is_string($data[$field]) || strlen($data[$field]) > 4096) { throw new \InvalidArgumentException('Invalid request'); }
    }
    return $data;
}

function challenge(string $token, string $expectedHostname): bool {
    if (!defined('DARKPHISH_TURNSTILE_SECRET') || DARKPHISH_TURNSTILE_SECRET === '' || strlen($token) > 2048) { return false; }
    $response = wp_remote_post('https://challenges.cloudflare.com/turnstile/v0/siteverify', [
        'timeout' => 10, 'redirection' => 0,
        'body' => ['secret' => DARKPHISH_TURNSTILE_SECRET, 'response' => $token],
    ]);
    if (is_wp_error($response) || wp_remote_retrieve_response_code($response) !== 200) { return false; }
    $body = json_decode(wp_remote_retrieve_body($response), true);
    return is_array($body) && ($body['success'] ?? false) === true &&
        ($body['hostname'] ?? '') === $expectedHostname && ($body['action'] ?? '') === 'darkphish-license';
}

function endpoint(string $operation, \WP_REST_Request $request): \WP_REST_Response {
    try {
        if (!is_ssl() || wp_parse_url(home_url(), PHP_URL_SCHEME) !== 'https') { return reply(['message' => 'HTTPS is required.'], 503); }
        $db = store();
        $now = time();
        // Forwarded headers are intentionally not trusted. Configure the web server's
        // real client address handling for a known reverse proxy.
        $ip = (string) ($_SERVER['REMOTE_ADDR'] ?? 'unknown');
        if (!$db->rate('ip:' . $operation, $ip, $operation === 'request' ? 10 : 120, 3600, $now)) {
            $response = reply(['message' => 'Please try again later.'], 429);
            $response->header('Retry-After', '3600');
            return $response;
        }
        $service = new Service($db, signer());
        if ($operation === 'request') {
            $data = input($request, ['email', 'terms_version', 'challenge_token']);
            $email = strtolower(trim($data['email']));
            $settings = get_option('darkphish_license_settings', []);
            if (!is_email($email) || strlen($email) > 254 || empty($settings['terms_url']) || empty($settings['terms_version']) ||
                !registrationUrlAllowed($settings['registration_url'] ?? '') || !hash_equals($settings['terms_version'], $data['terms_version'])) {
                return reply(['message' => 'A valid email and current terms acceptance are required.'], 400);
            }
            $challengeOrigin = $request->get_header('origin') ?: httpsOrigin($settings['registration_url']);
            if (!challenge($data['challenge_token'], (string) wp_parse_url($challengeOrigin, PHP_URL_HOST))) { return reply(['message' => 'Please complete the anti-abuse check.'], 400); }
            $generic = ['message' => 'If the request can be processed, a verification link will arrive by email.'];
            if (!$db->rate('email:request', $email, 3, 3600, $now)) { return reply($generic, 202); }
            $token = $service->request($email, $settings['terms_version'], $now);
            $url = $settings['registration_url'] . '#dp-verify=' . rawurlencode($token);
            $sent = wp_mail($email, 'Verify your Darkphish Community license request', "Confirm your request within 30 minutes:\n\n" . $url . "\n\nIf you did not request a license, ignore this message.");
            if (!$sent) { $db->deleteRequest(hash('sha256', $token)); return reply(['message' => 'Email delivery is unavailable. Try again later.'], 503); }
            return reply($generic, 202);
        }
        if ($operation === 'verify') {
            $data = input($request, ['token']);
            if (!preg_match('/^[A-Za-z0-9_-]{43}$/D', $data['token'])) { return reply(['message' => 'Verification unavailable.'], 403); }
            return reply($service->verify($data['token'], $now));
        }
        $credential = $operation === 'refresh' ? 'refresh_token' : 'license_key';
        $data = input($request, [$credential, 'installation_id', 'product_version']);
        return reply($service->exchange($data[$credential], $data['installation_id'], $data['product_version'], $operation === 'refresh', $now));
    } catch (\DomainException $error) {
        return reply(['message' => 'License or verification unavailable.'], 403);
    } catch (\InvalidArgumentException | \JsonException $error) {
        return reply(['message' => 'Invalid request.'], 400);
    } catch (\Throwable $error) {
        // Never reflect database, key-file or remote-service errors into public responses.
        return reply(['message' => 'Licensing service is temporarily unavailable.'], 503);
    }
}

add_action('rest_api_init', function (): void {
    register_rest_route('darkphish-license/v1', '/public-config', [
        'methods' => 'GET', 'permission_callback' => '__return_true',
        'callback' => __NAMESPACE__ . '\publicConfiguration',
    ]);
    foreach (['request', 'verify', 'activate', 'refresh'] as $operation) {
        register_rest_route('darkphish-license/v1', '/' . $operation, [
            'methods' => 'POST', 'permission_callback' => '__return_true',
            'callback' => fn (\WP_REST_Request $request) => endpoint($operation, $request),
        ]);
    }
});

add_action('admin_init', function (): void {
    register_setting('darkphish_license', 'darkphish_license_settings', ['sanitize_callback' => function ($value): array {
        $clean = [];
        foreach (['terms_url', 'registration_url'] as $key) {
            $url = esc_url_raw(is_string($value[$key] ?? null) ? $value[$key] : '', ['https']);
            // External registration is pinned by a server-owned wp-config.php constant.
            if ($key === 'registration_url' && !registrationUrlAllowed($url)) { $url = ''; }
            if (wp_parse_url($url, PHP_URL_FRAGMENT) || wp_parse_url($url, PHP_URL_USER) || wp_parse_url($url, PHP_URL_PASS)) { $url = ''; }
            $clean[$key] = $url;
        }
        $clean['terms_version'] = substr(sanitize_text_field($value['terms_version'] ?? ''), 0, 80);
        return $clean;
    }]);
});

add_action('admin_menu', function (): void {
    add_options_page('Darkphish licensing', 'Darkphish licensing', 'manage_options', 'darkphish-license', __NAMESPACE__ . '\\adminPage');
});

function adminPage(): void {
    if (!current_user_can('manage_options')) { wp_die('Forbidden', '', ['response' => 403]); }
    $settings = get_option('darkphish_license_settings', []);
    echo '<div class="wrap"><h1>Darkphish Community licensing</h1><p>Community: 100 managed users, 1 active campaign, 365-day license. Lease: 30 days + up to 30 days offline grace.</p>';
    try {
        $keyring = signer()->keyring();
        echo '<h2>Public verification keyring</h2><p>Copy this public JSON to Darkphish. No private key is displayed.</p><pre>' . esc_html(wp_json_encode($keyring, JSON_PRETTY_PRINT)) . '</pre>';
        echo '<p>API: <code>' . esc_html(rest_url('darkphish-license/v1')) . '</code></p>';
    } catch (\Throwable $error) {
        echo '<h2>Vytvorenie podpisového kľúča bez SSH</h2><p>Vo wp-config.php nastavte DARKPHISH_LICENSE_KEY_FILE na absolútnu cestu k novému JSON súboru v existujúcom súkromnom priečinku mimo verejného webu. Priečinok musí byť zapisovateľný používateľom PHP.</p>';
        echo '<p>WordPress: <code>' . esc_html(ABSPATH) . '</code><br>Verejný koreň webu: <code>' . esc_html($_SERVER['DOCUMENT_ROOT'] ?? ABSPATH) . '</code></p>';
        if (defined('DARKPHISH_LICENSE_KEY_FILE')) {
            echo '<p>Nakonfigurovaná cesta: <code>' . esc_html(DARKPHISH_LICENSE_KEY_FILE) . '</code></p>';
            if (!file_exists(DARKPHISH_LICENSE_KEY_FILE) && !is_link(DARKPHISH_LICENSE_KEY_FILE) && is_ssl()) {
                echo '<form method="post" action="' . esc_url(admin_url('admin-post.php')) . '">';
                wp_nonce_field('darkphish-license-initialize-key');
                echo '<input type="hidden" name="action" value="darkphish_license_initialize_key"><button class="button button-primary">Vytvoriť podpisový kľúč na serveri</button></form><p>Súkromný kľúč sa nevypíše ani neodošle do prehliadača. Existujúci súbor sa nikdy neprepíše.</p>';
            } else { echo '<p>Súbor už existuje alebo administrácia nepoužíva HTTPS. Overte konfiguráciu; nevytvárajte náhradný kľúč bez plánovanej rotácie.</p>'; }
        }
    }
    echo '<form action="options.php" method="post">';
    settings_fields('darkphish_license');
    foreach (['registration_url' => 'Registration page URL (static HTML or shortcode)', 'terms_url' => 'Community terms URL', 'terms_version' => 'Terms version (for example 2026-09-16)'] as $field => $label) {
        echo '<p><label>' . esc_html($label) . '<br><input class="regular-text" name="darkphish_license_settings[' . esc_attr($field) . ']" value="' . esc_attr($settings[$field] ?? '') . '" required></label></p>';
    }
    submit_button(); echo '</form><h2>Latest 100 licenses</h2><p>Revocation prevents further leases. Previously signed leases remain usable until their signed grace deadline. Reset does not invalidate an offline lease.</p><table class="widefat"><thead><tr><th>License / email</th><th>Status / expires</th><th>Installation</th><th>Action</th></tr></thead><tbody>';
    foreach (store()->recentLicenses() as $license) {
        echo '<tr><td>' . esc_html($license['id']) . '<br>' . esc_html($license['email']) . '</td><td>' . esc_html($license['status']) . '<br>' . esc_html(gmdate('Y-m-d', (int) $license['expires_at'])) . '</td><td>' . esc_html($license['installation'] ?: 'Not activated') . '</td><td><form method="post" action="' . esc_url(admin_url('admin-post.php')) . '">';
        wp_nonce_field('darkphish-license:' . $license['id']);
        echo '<input type="hidden" name="action" value="darkphish_license_admin"><input type="hidden" name="license_id" value="' . esc_attr($license['id']) . '"><select name="operation"><option value="revoke">Revoke</option><option value="restore">Restore</option><option value="reset">Reset installation</option><option value="renew">Renew for 365 days</option></select> <button class="button">Apply</button></form></td></tr>';
    }
    echo '</tbody></table></div>';
}

add_action('admin_post_darkphish_license_admin', function (): void {
    if (($_SERVER['REQUEST_METHOD'] ?? '') !== 'POST' || !current_user_can('manage_options')) { wp_die('Forbidden', '', ['response' => 403]); }
    $id = sanitize_text_field(wp_unslash($_POST['license_id'] ?? ''));
    check_admin_referer('darkphish-license:' . $id);
    // Emergency revocation must remain available if the signing file is missing.
    try { (new Service(store()))->administer($id, sanitize_key($_POST['operation'] ?? ''), get_current_user_id(), time()); }
    catch (\Throwable $error) { wp_die('The licensing action could not be completed.'); }
    wp_safe_redirect(admin_url('options-general.php?page=darkphish-license')); exit;
});

add_shortcode('darkphish_license', function (): string {
    $settings = get_option('darkphish_license_settings', []);
    if (!is_ssl() || !defined('DARKPHISH_TURNSTILE_SITE_KEY') || !defined('DARKPHISH_TURNSTILE_SECRET') || empty($settings['terms_url']) || empty($settings['terms_version']) || empty($settings['registration_url'])) {
        return '<p>Community registration is not available yet.</p>';
    }
    // Token is carried in the fragment, never in a query string or referrer.
    wp_enqueue_script('darkphish-license', plugins_url('assets/registration.js', __FILE__), [], '0.1.2', true);
    wp_enqueue_script('darkphish-turnstile', 'https://challenges.cloudflare.com/turnstile/v0/api.js', [], null, true);
    return '<section id="darkphish-license" data-api="' . esc_url(rest_url('darkphish-license/v1/')) . '" data-terms="' . esc_attr($settings['terms_version']) . '"><h2>Darkphish Community</h2><p>Free registration: 100 managed users and 1 active campaign.</p><p role="status" aria-live="polite" class="dp-status"></p><form class="dp-request"><label>Email <input type="email" name="email" maxlength="254" autocomplete="email" required></label><p><label><input type="checkbox" required> I accept the <a href="' . esc_url($settings['terms_url']) . '" target="_blank" rel="noopener noreferrer">Community terms</a>.</label></p><div class="cf-turnstile" data-action="darkphish-license" data-sitekey="' . esc_attr(DARKPHISH_TURNSTILE_SITE_KEY) . '"></div><button type="submit">Request license</button></form><button type="button" class="dp-verify" hidden>Confirm email and display my license key</button><pre class="dp-key" hidden></pre></section>';
});
