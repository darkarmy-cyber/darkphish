<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

function adminTab(mixed $value): string {
    return is_string($value) && in_array($value, ['dashboard', 'community', 'professional', 'enterprise', 'settings'], true) ? $value : 'dashboard';
}
function adminUrl(string $tab): string { return admin_url('admin.php?page=darkphish-licenses&tab=' . adminTab($tab)); }

add_action('admin_menu', function (): void {
    add_menu_page('Darkphish Licenses', 'Darkphish Licenses', 'manage_options', 'darkphish-licenses', __NAMESPACE__ . '\\adminPage', 'dashicons-admin-network', 3.5);
    // Keep existing bookmarks working after the plugin moves out of Settings.
    add_options_page('Darkphish Licenses', 'Darkphish Licenses', 'manage_options', 'darkphish-license', __NAMESPACE__ . '\\adminSettings');
    remove_submenu_page('options-general.php', 'darkphish-license');
});
add_filter('custom_menu_order', '__return_true');
add_filter('menu_order', __NAMESPACE__ . '\\licenseMenuOrder', 100);
function licenseMenuOrder(array $order): array {
    global $menu;
    $zeus = null;
    if (!is_array($menu)) { return $order; }
    foreach ($menu as $item) {
        if (strcasecmp(trim(wp_strip_all_tags((string) $item[0])), 'Zeus') === 0) { $zeus = $item[2]; break; }
    }
    if ($zeus === null || !in_array('darkphish-licenses', $order, true) || !in_array($zeus, $order, true)) { return $order; }
    $order = array_values(array_filter($order, static fn ($slug) => $slug !== 'darkphish-licenses'));
    array_splice($order, array_search($zeus, $order, true) + 1, 0, ['darkphish-licenses']);
    return $order;
}
add_action('admin_enqueue_scripts', function (string $hook): void {
    if ($hook === 'toplevel_page_darkphish-licenses') {
        wp_enqueue_script('darkphish-licenses-admin', plugins_url('assets/admin.js', dirname(__DIR__) . '/darkphish-license.php'), [], '0.2.0', true);
        wp_enqueue_style('darkphish-licenses-admin', plugins_url('assets/admin.css', dirname(__DIR__) . '/darkphish-license.php'), [], '0.2.0');
    }
});

function planDefaults(string $edition): array {
    $plans = get_option('darkphish_license_plans', []);
    $baseline = ['professional' => ['managed_users' => 250, 'active_campaigns' => -1, 'days' => ''], 'enterprise' => ['managed_users' => -1, 'active_campaigns' => -1, 'days' => '']];
    return is_array($plans[$edition] ?? null) ? $plans[$edition] : ($baseline[$edition] ?? []);
}
add_action('admin_init', function (): void {
    register_setting('darkphish_license_plans', 'darkphish_license_plans', ['sanitize_callback' => __NAMESPACE__ . '\\sanitizePlans']);
});
function sanitizePlans(mixed $input): array {
    $output = [];
    foreach (['professional', 'enterprise'] as $edition) {
        foreach (['managed_users' => 100000000, 'active_campaigns' => 1000000, 'days' => 3650] as $field => $maximum) {
            $raw = $input[$edition][$field] ?? '';
            if ($field !== 'days' && (($input[$edition][$field . '_unlimited'] ?? '') === '1' || $raw === -1)) { $output[$edition][$field] = -1; continue; }
            if ($raw === '') { $output[$edition][$field] = ''; continue; }
            $value = (is_string($raw) || is_int($raw)) && preg_match('/^[0-9]+$/D', (string) $raw) ? filter_var($raw, FILTER_VALIDATE_INT, ['options' => ['min_range' => 1, 'max_range' => $maximum]]) : false;
            if ($value === false) {
                add_settings_error('darkphish_license_plans', 'invalid_plan', 'Plan defaults must be positive whole numbers within the displayed limits.');
                return get_option('darkphish_license_plans', []);
            }
            $output[$edition][$field] = $value;
        }
    }
    return $output;
}
function planSettings(): void {
    echo '<h2>Default license limits</h2><p>Use the published plan limits here. Blank values require manual entry. Changes affect new manual issuance only; existing licenses keep their agreed limits.</p><form method="post" action="options.php">';
    settings_fields('darkphish_license_plans');
    foreach (['professional', 'enterprise'] as $edition) {
        $defaults = planDefaults($edition);
        echo '<h3>' . esc_html(ucfirst($edition)) . '</h3><div class="dp-admin-fields">';
        foreach (['managed_users' => ['Managed users', 100000000], 'active_campaigns' => ['Active campaigns', 1000000]] as $field => [$label, $max]) {
            limitField('darkphish_license_plans[' . $edition . '][' . $field . ']', 'darkphish_license_plans[' . $edition . '][' . $field . '_unlimited]', $label, $defaults[$field] ?? '', $max, false);
        }
        echo '<label>Validity in days<input type="number" min="1" max="3650" name="darkphish_license_plans[' . esc_attr($edition) . '][days]" value="' . esc_attr((string) ($defaults['days'] ?? '')) . '"></label>';
        echo '</div>';
    }
    submit_button('Save plan defaults'); echo '</form>';
}

function adminLimit(string $field, int $maximum): int {
    return ($_POST[$field . '_unlimited'] ?? '') === '1' ? -1 : adminNumber($field, 1, $maximum);
}
function limitLabel(mixed $value): string { return (int) $value === -1 ? 'Unlimited' : (string) $value; }
function limitField(string $name, string $unlimitedName, string $label, mixed $value, int $maximum, bool $required): void {
    $unlimited = (string) $value === '-1';
    echo '<div class="dp-limit"><label>' . esc_html($label) . '<input type="number" name="' . esc_attr($name) . '" min="1" max="' . esc_attr((string) $maximum) . '" value="' . esc_attr($unlimited ? '' : (string) $value) . '"' . ($required ? ' required' : '') . ($unlimited ? ' disabled' : '') . '></label><label class="dp-unlimited"><input type="checkbox" name="' . esc_attr($unlimitedName) . '" value="1"' . ($unlimited ? ' checked' : '') . '> Unlimited</label></div>';
}
function adminNumber(string $field, int $min, int $max): int {
    $value = $_POST[$field] ?? null;
    if (!is_string($value) || !preg_match('/^[0-9]+$/D', $value)) { throw new \InvalidArgumentException('Invalid number'); }
    $number = filter_var($value, FILTER_VALIDATE_INT, ['options' => ['min_range' => $min, 'max_range' => $max]]);
    if ($number === false) { throw new \InvalidArgumentException('Invalid number'); }
    return $number;
}

// Called only from the protected admin screen; public registration cannot choose an edition.
function processLicenseForm(string $tab): ?array {
    if (($_SERVER['REQUEST_METHOD'] ?? '') !== 'POST') { return null; }
    if (!current_user_can('manage_options') || !is_ssl()) { wp_die('Forbidden', '', ['response' => 403]); }
    check_admin_referer('darkphish-license-manage');
    $operation = is_string($_POST['license_operation'] ?? null) ? $_POST['license_operation'] : '';
    $service = new Service(store());
    if ($operation === 'issue' && in_array($tab, ['professional', 'enterprise'], true)) {
        return $service->issuePaid($tab, sanitize_email(wp_unslash($_POST['email'] ?? '')),
            adminLimit('managed_users', 100000000), adminLimit('active_campaigns', 1000000),
            adminNumber('days', 1, 3650), sanitize_text_field(wp_unslash($_POST['terms_version'] ?? '')), get_current_user_id(), time());
    }
    if ($operation === 'edit' && in_array($tab, ['professional', 'enterprise'], true)) {
        $id = sanitize_text_field(wp_unslash($_POST['license_id'] ?? ''));
        $date = is_string($_POST['expires_at'] ?? null) ? $_POST['expires_at'] : '';
        $parsed = \DateTimeImmutable::createFromFormat('!Y-m-d', $date, new \DateTimeZone('UTC'));
        if (!$parsed || $parsed->format('Y-m-d') !== $date) { throw new \InvalidArgumentException('Invalid expiry'); }
        $service->editPaid($id, $tab, adminLimit('managed_users', 100000000), adminLimit('active_campaigns', 1000000),
            $parsed->getTimestamp() + 86399, get_current_user_id(), time());
        return ['message' => 'License updated. Changes apply to the next signed lease; existing offline leases keep their original limits and expiry.'];
    }
    throw new \InvalidArgumentException('Invalid operation');
}

function adminPage(): void {
    if (!current_user_can('manage_options')) { wp_die('Forbidden', '', ['response' => 403]); }
    nocache_headers();
    $tab = adminTab($_GET['tab'] ?? 'dashboard');
    $result = null; $failed = false;
    try { $result = processLicenseForm($tab); }
    catch (\Throwable $error) { $failed = true; }
    echo '<div class="wrap dp-admin"><header class="dp-admin-hero"><span class="dp-admin-mark" aria-hidden="true">D</span><div><p>DARKPHISH · LICENSE MANAGEMENT</p><h1>Darkphish Licenses</h1><span>One place for Community, Professional and Enterprise.</span></div></header>';
    echo '<nav class="dp-admin-tabs" aria-label="License management">';
    foreach (['dashboard', 'community', 'professional', 'enterprise', 'settings'] as $name) {
        echo '<a href="' . esc_url(adminUrl($name)) . '"' . ($tab === $name ? ' class="is-active" aria-current="page"' : '') . '>' . esc_html(ucfirst($name)) . '</a>';
    }
    echo '</nav>';
    if ($failed) { echo '<div class="notice notice-error"><p>The action could not be completed. Check the details, expiry and whether this email already has a license in this edition.</p></div>'; }
    if (isset($result['license_key'])) {
        echo '<section class="dp-admin-panel"><h2>License created</h2><p>Copy this key now and deliver it securely to the customer. It is shown only in this response and is not emailed automatically.</p><pre class="dp-admin-key">' . esc_html($result['license_key']) . '</pre><p>License ID: ' . esc_html($result['license_id']) . '</p></section>';
    } elseif (isset($result['message'])) { echo '<div class="notice notice-success"><p>' . esc_html($result['message']) . '</p></div>'; }
    try {
        if ($tab === 'settings') { echo '<section class="dp-admin-panel">'; adminSettings(); planSettings(); echo '</section>'; }
        elseif ($tab === 'dashboard') { adminDashboard(); }
        else { adminEdition($tab); }
    } catch (\Throwable $error) { echo '<div class="notice notice-error"><p>License data is unavailable. Check the database upgrade and try again.</p></div>'; }
    echo '</div>';
}

function adminDashboard(): void {
    $stats = store()->statistics(time());
    echo '<h2>License overview</h2><p class="description">Current database totals. Valid includes licenses that have not been activated yet.</p><div class="dp-admin-grid">';
    foreach ($stats as $edition => $counts) {
        echo '<section class="dp-admin-panel"><p class="dp-admin-eyebrow">' . esc_html(strtoupper($edition)) . '</p><strong class="dp-admin-total">' . esc_html((string) $counts['total']) . '</strong><span> total licenses</span><dl class="dp-admin-stats">';
        foreach (['active' => 'Valid', 'unbound' => 'Valid, not activated', 'expired' => 'Expired', 'revoked' => 'Revoked'] as $key => $label) {
            echo '<div><dt>' . esc_html($label) . '</dt><dd>' . esc_html((string) $counts[$key]) . '</dd></div>';
        }
        echo '</dl><a class="button" href="' . esc_url(adminUrl($edition)) . '">Manage ' . esc_html(ucfirst($edition)) . '</a></section>';
    }
    echo '</div><section class="dp-admin-panel"><h2>Service connections</h2><p>Registration: <a href="https://www.darkphish.sk/license/">www.darkphish.sk/license/</a></p><p>License support: <a href="mailto:license@darkphish.sk">license@darkphish.sk</a> · Sales: <a href="mailto:sales@darkphish.sk">sales@darkphish.sk</a></p><p>These statistics describe issued licenses, not payment or subscription revenue.</p></section>';
}
function licenseFields(?array $license = null): void {
    echo '<div class="dp-admin-fields">';
    foreach (['managed_users' => ['Managed users', 100000000], 'active_campaigns' => ['Active campaigns', 1000000]] as $field => [$label, $max]) {
        limitField($field, $field . '_unlimited', $label, $license[$field] ?? '', $max, true);
    }
    echo '</div>';
}
function adminEdition(string $edition): void {
    $page = max(1, min(1000000, (int) ($_GET['paged'] ?? 1)));
    $db = store(); $licenses = $db->recentLicenses($edition, $page); $count = $db->statistics(time())[$edition]['total'];
    echo '<h2>' . esc_html(ucfirst($edition)) . ' licenses</h2>';
    if ($edition === 'community') { echo '<p>Self-service email verification · 100 managed users · 1 active campaign · 365 days.</p>'; }
    else {
        echo '<details class="dp-admin-panel"><summary>Issue a ' . esc_html(ucfirst($edition)) . ' license manually</summary><p>Enter the limits and validity agreed with the customer. No payment is processed. One license per email and edition; the same email may use other editions separately.</p><form method="post" action="' . esc_url(adminUrl($edition)) . '">';
        wp_nonce_field('darkphish-license-manage');
        echo '<input type="hidden" name="license_operation" value="issue"><label>Customer email<input type="email" name="email" maxlength="254" required></label>';
        $defaults = planDefaults($edition);
        licenseFields($defaults);
        echo '<div class="dp-admin-fields"><label>Validity in days<input type="number" name="days" min="1" max="3650" value="' . esc_attr((string) ($defaults['days'] ?? '')) . '" required></label><label>Accepted terms / agreement reference<input name="terms_version" maxlength="80" required></label></div><p>Commercial activation requires a client that supports the signed edition. This does not enable unimplemented Professional or Enterprise product features.</p><button class="button button-primary">Create license and show key</button></form></details>';
    }
    echo '<p>Revocation, resets and limit changes affect future leases. Existing signed offline leases remain usable until their original grace deadline.</p><div class="dp-admin-table"><table class="widefat striped"><thead><tr><th scope="col">License / email</th><th scope="col">Status / expiry (UTC)</th><th scope="col">Limits</th><th scope="col">Installation</th><th scope="col">Manage</th></tr></thead><tbody>';
    if (!$licenses) { echo '<tr><td colspan="5">No licenses in this edition yet.</td></tr>'; }
    foreach ($licenses as $license) {
        $status = $license['status'] === 'active' && (int) $license['expires_at'] <= time() ? 'expired' : $license['status'];
        echo '<tr><td><strong>' . esc_html($license['email']) . '</strong><br><code>' . esc_html($license['id']) . '</code></td><td>' . esc_html(ucfirst($status)) . '<br>' . esc_html(gmdate('Y-m-d H:i', (int) $license['expires_at'])) . '</td><td>' . esc_html(limitLabel($license['managed_users'])) . ' users<br>' . esc_html(limitLabel($license['active_campaigns'])) . ' campaigns</td><td>' . esc_html($license['installation'] ?: 'Not activated') . '</td><td><form method="post" action="' . esc_url(admin_url('admin-post.php')) . '">';
        wp_nonce_field('darkphish-license:' . $license['id']);
        echo '<input type="hidden" name="action" value="darkphish_license_admin"><input type="hidden" name="license_id" value="' . esc_attr($license['id']) . '"><input type="hidden" name="edition" value="' . esc_attr($edition) . '"><label class="screen-reader-text" for="op-' . esc_attr($license['id']) . '">License action</label><select id="op-' . esc_attr($license['id']) . '" name="operation"><option value="revoke">Revoke</option><option value="restore">Restore</option><option value="reset">Reset installation</option><option value="renew">Renew for 365 days</option></select> <button class="button">Apply</button></form>';
        if ($edition !== 'community') {
            echo '<details><summary>Edit limits and expiry</summary><form method="post" action="' . esc_url(adminUrl($edition)) . '">';
            wp_nonce_field('darkphish-license-manage');
            echo '<input type="hidden" name="license_operation" value="edit"><input type="hidden" name="license_id" value="' . esc_attr($license['id']) . '">';
            licenseFields($license);
            echo '<label>Expires at end of day (UTC)<input type="date" name="expires_at" value="' . esc_attr(gmdate('Y-m-d', (int) $license['expires_at'])) . '" required></label><button class="button">Save changes</button></form></details>';
        }
        echo '</td></tr>';
    }
    echo '</tbody></table></div><p>' . esc_html((string) $count) . ' licenses · Page ' . esc_html((string) $page) . '</p><nav aria-label="License pages">';
    if ($page > 1) { echo '<a class="button" href="' . esc_url(adminUrl($edition) . '&paged=' . ($page - 1)) . '">Previous</a> '; }
    if ($page * 50 < $count) { echo '<a class="button" href="' . esc_url(adminUrl($edition) . '&paged=' . ($page + 1)) . '">Next</a>'; }
    echo '</nav>';
}
