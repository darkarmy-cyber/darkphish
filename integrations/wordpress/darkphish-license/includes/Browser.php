<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

function httpsOrigin(string $url): string {
    $parts = wp_parse_url($url);
    if (!is_array($parts) || ($parts['scheme'] ?? '') !== 'https' || empty($parts['host']) ||
        isset($parts['user']) || isset($parts['pass']) || (isset($parts['port']) && $parts['port'] !== 443) ||
        !preg_match('/^[a-z0-9.-]+$/D', $parts['host'])) { return ''; }
    return 'https://' . $parts['host'];
}

function browserOrigins(): array {
    $origins = [httpsOrigin(home_url())];
    if (defined('DARKPHISH_LICENSE_REGISTRATION_ORIGIN') && is_string(DARKPHISH_LICENSE_REGISTRATION_ORIGIN)) {
        $origin = httpsOrigin(DARKPHISH_LICENSE_REGISTRATION_ORIGIN);
        if ($origin !== '' && $origin === DARKPHISH_LICENSE_REGISTRATION_ORIGIN) { $origins[] = $origin; }
    }
    return array_values(array_unique(array_filter($origins)));
}

function registrationUrlAllowed(string $url): bool {
    $origin = httpsOrigin($url);
    return $origin !== '' && in_array($origin, browserOrigins(), true) &&
        wp_parse_url($url, PHP_URL_FRAGMENT) === null && wp_parse_url($url, PHP_URL_QUERY) === null;
}

function licensingRoute(string $route): bool {
    return $route === '/darkphish-license/v1' || str_starts_with($route, '/darkphish-license/v1/');
}

// Limit only this plugin's namespace; other WordPress REST routes keep their policy.
add_filter('rest_pre_dispatch', function ($result, $server, \WP_REST_Request $request) {
    if (!licensingRoute($request->get_route())) { return $result; }
    $origin = $request->get_header('origin');
    if ($origin !== '' && !in_array($origin, browserOrigins(), true)) {
        return reply(['message' => 'Origin is not allowed.'], 403);
    }
    if ($request->get_method() === 'OPTIONS' && $origin !== '') {
        $method = $request->get_header('access-control-request-method');
        $expected = $request->get_route() === '/darkphish-license/v1/public-config' ? 'GET' : 'POST';
        $headers = array_filter(array_map('trim', explode(',', strtolower($request->get_header('access-control-request-headers')))));
        if ($method !== $expected || array_diff($headers, ['content-type'])) {
            return reply(['message' => 'Preflight is not allowed.'], 403);
        }
        return reply([], 204);
    }
    return $result;
}, 10, 3);

// WordPress normally reflects any Origin and allows credentials. Replace those
// headers after core's handler, only for licensing, including error responses.
add_filter('rest_pre_serve_request', function ($served, $result, \WP_REST_Request $request) {
    if (!licensingRoute($request->get_route())) { return $served; }
    foreach (['Origin', 'Credentials', 'Methods', 'Headers'] as $name) { header_remove('Access-Control-Allow-' . $name); }
    header_remove('Access-Control-Expose-Headers');
    header('Vary: Origin', false);
    $origin = $request->get_header('origin');
    if (in_array($origin, browserOrigins(), true)) {
        header('Access-Control-Allow-Origin: ' . $origin);
        header('Access-Control-Allow-Methods: GET, POST, OPTIONS');
        header('Access-Control-Allow-Headers: Content-Type');
    }
    return $served;
}, 100, 3);

function publicConfiguration(): \WP_REST_Response {
    $settings = get_option('darkphish_license_settings', []);
    if (!is_ssl() || !defined('DARKPHISH_TURNSTILE_SITE_KEY') || DARKPHISH_TURNSTILE_SITE_KEY === '' ||
        !defined('DARKPHISH_TURNSTILE_SECRET') || DARKPHISH_TURNSTILE_SECRET === '' ||
        !registrationUrlAllowed($settings['registration_url'] ?? '') ||
        httpsOrigin($settings['terms_url'] ?? '') === '' || empty($settings['terms_version'])) {
        return reply(['message' => 'Community registration is not available yet.'], 503);
    }
    return reply(['site_key' => DARKPHISH_TURNSTILE_SITE_KEY, 'terms_url' => $settings['terms_url'],
        'terms_version' => $settings['terms_version'], 'registration_url' => $settings['registration_url']]);
}
