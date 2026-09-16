<?php
declare(strict_types=1);
// Included by integration.php against its explicitly disposable WP/MySQL database.
// Reproduce an in-place upgrade with a real existing license and credentials.
$legacy = $db->find('licenses', 'id', $license['license_id']);
$wpdb->query("ALTER TABLE {$db->prefix}licenses DROP COLUMN edition");
delete_option('darkphish_license_schema');
do_action('plugins_loaded');
check(get_option('darkphish_license_schema') === '2', 'Upgrade version marker missing');
$upgraded = $db->find('licenses', 'id', $license['license_id']);
check($upgraded['edition'] === 'community' && $upgraded['key_hash'] === $legacy['key_hash'] && $upgraded['installation'] === $legacy['installation'], 'Upgrade lost existing license');
check(get_option('darkphish_license_settings') === $settings, 'Upgrade lost settings');
$db->install();
check($db->find('licenses', 'id', $license['license_id']) === $upgraded, 'Repeated upgrade changed license');

$paid = $service->issuePaid('professional', 'owner@example.test', 500, 5, 365, 'contract-test', 1, time());
$enterprise = $service->issuePaid('enterprise', 'owner@example.test', -1, -1, 365, 'contract-test', 1, time());
foreach (['professional' => $paid, 'enterprise' => $enterprise] as $edition => $issued) {
    $paidActivation = $service->exchange($issued['license_key'], $installation, '0.11.0', false, time());
    $payload = json_decode(base64_decode(strtr($paidActivation['lease']['payload'], '-_', '+/')), true);
    check($payload['edition'] === $edition, 'Signed edition incorrect');
    if ($edition === 'enterprise') { check($payload['entitlements']['managed_users'] === -1 && $payload['entitlements']['active_campaigns'] === -1, 'Unlimited entitlement lost'); }
    $storedPaid = $db->find('licenses', 'id', $issued['license_id']);
    check(!str_contains(json_encode($storedPaid), $issued['license_key']), 'Paid key stored in plaintext');
}
$paidBefore = $db->find('licenses', 'id', $paid['license_id']);
$token = $service->request('owner@example.test', 'test-v1', time());
$communityRecovery = $service->verify($token, time());
check($communityRecovery['license_id'] === $license['license_id'], 'Community recovery selected paid license');
check($db->find('licenses', 'id', $paid['license_id']) === $paidBefore, 'Community recovery changed paid license');
try { $service->issuePaid('professional', 'owner@example.test', 500, 5, 365, 'contract-test', 1, time()); throw new LogicException('Duplicate edition admitted'); }
catch (DomainException $expected) {}
foreach ([['community', 500, 5], ['invalid', 500, 5], ['professional', 0, 5], ['professional', 500, 0]] as [$edition, $users, $campaigns]) {
    try { $service->issuePaid($edition, 'bad@example.test', $users, $campaigns, 365, 'contract-test', 1, time()); throw new LogicException('Invalid paid license admitted'); }
    catch (InvalidArgumentException $expected) {}
}
$service->editPaid($paid['license_id'], 'professional', 750, 7, time() + 400 * 86400, 1, time());
$edited = $db->find('licenses', 'id', $paid['license_id']);
check((int) $edited['managed_users'] === 750 && (int) $edited['active_campaigns'] === 7 && $edited['installation'] === $paidBefore['installation'] && $edited['key_hash'] === $paidBefore['key_hash'], 'Edit corrupted binding or limits');
try { $service->editPaid($license['license_id'], 'professional', 500, 5, time() + 86400, 1, time()); throw new LogicException('Cross-edition edit admitted'); }
catch (DomainException $expected) {}
$wpdb->query("CREATE TRIGGER darkphish_test_reject_event BEFORE INSERT ON {$db->prefix}events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test event failure'");
try {
    try { $service->editPaid($paid['license_id'], 'professional', 900, 9, time() + 86400, 1, time()); throw new LogicException('Edit audit failure ignored'); } catch (RuntimeException $expected) {}
    try { $service->issuePaid('enterprise', 'rollback@example.test', 900, 9, 10, 'test', 1, time()); throw new LogicException('Issue audit failure ignored'); } catch (RuntimeException $expected) {}
} finally { $wpdb->query('DROP TRIGGER darkphish_test_reject_event'); }
check($db->find('licenses', 'id', $paid['license_id']) === $edited, 'Failed audit left partial edit');
check($db->find('licenses', 'email_hash', hash('sha256', 'enterprise:rollback@example.test')) === null, 'Failed audit left issued license');
$service->administer($enterprise['license_id'], 'revoke', 1, time());
try { $service->exchange($enterprise['license_key'], $installation, '0.11.0', false, time()); throw new LogicException('Revoked paid activation accepted'); } catch (DomainException $expected) {}
for ($i = 0; $i < 51; $i++) { $service->issuePaid('enterprise', "page$i@example.test", 2000, 20, 30, 'test', 1, time()); }
$stats = $db->statistics(time());
check($stats['professional']['total'] === 1 && $stats['enterprise']['total'] === 52 && $stats['enterprise']['revoked'] === 1 && $stats['enterprise']['unbound'] === 51, 'Dashboard counts incorrect');
check(count($db->recentLicenses('enterprise', 1)) === 50 && count($db->recentLicenses('enterprise', 2)) === 2, 'Edition pagination lost records');
check(count($db->recentLicenses('professional')) === 1, 'Edition filter leaked rows');
$menu = [[0 => 'Zeus', 2 => 'fsociety-zeus'], [0 => 'Darkphish Licenses', 2 => 'darkphish-licenses']];
check(Darkphish\Licensing\licenseMenuOrder(['index.php', 'darkphish-licenses', 'fsociety-zeus', 'edit.php']) === ['index.php', 'fsociety-zeus', 'darkphish-licenses', 'edit.php'], 'Menu not immediately below Zeus');
$menu = [];
check(Darkphish\Licensing\licenseMenuOrder(['index.php', 'darkphish-licenses']) === ['index.php', 'darkphish-licenses'], 'No-Zeus fallback changed menu');

$_SERVER['REQUEST_METHOD'] = 'POST'; $_REQUEST = []; $_POST = ['license_operation' => 'issue'];
wp_set_current_user(0);
try { Darkphish\Licensing\processLicenseForm('professional'); throw new LogicException('Anonymous issuance accepted'); } catch (AdminDenied $expected) {}
wp_set_current_user(1);
try { Darkphish\Licensing\processLicenseForm('professional'); throw new LogicException('No-nonce issuance accepted'); } catch (AdminDenied $expected) {}
$_REQUEST['_wpnonce'] = wp_create_nonce('darkphish-license-manage');
$_POST += ['email' => 'admin-issued@example.test', 'managed_users' => '300', 'active_campaigns' => '3', 'days' => '90', 'terms_version' => 'test'];
$fromForm = Darkphish\Licensing\processLicenseForm('professional');
check(str_starts_with($fromForm['license_key'], 'DP-PRO-'), 'Authorized manual form failed');
$_POST['managed_users'] = '300x';
try { Darkphish\Licensing\processLicenseForm('professional'); throw new LogicException('Malformed integer accepted'); } catch (InvalidArgumentException $expected) {}
$_SERVER['REQUEST_METHOD'] = 'GET'; $_GET = ['tab' => 'dashboard'];
check(Darkphish\Licensing\planDefaults('professional')['managed_users'] === 250 && Darkphish\Licensing\planDefaults('professional')['active_campaigns'] === -1, 'Published Professional defaults incorrect');
check(Darkphish\Licensing\planDefaults('enterprise')['managed_users'] === -1, 'Published Enterprise defaults incorrect');
$presets = ['professional' => ['managed_users' => '500', 'active_campaigns' => '5', 'days' => '365'], 'enterprise' => ['managed_users' => '', 'active_campaigns' => '', 'days' => '']];
$cleanPlans = Darkphish\Licensing\sanitizePlans($presets);
check(Darkphish\Licensing\sanitizePlans($cleanPlans) === $cleanPlans, 'Repeated WordPress sanitization changes defaults');
update_option('darkphish_license_plans', $cleanPlans);
check(Darkphish\Licensing\planDefaults('professional')['managed_users'] === 500, 'Plan defaults not stored');
$unlimitedPlans = $presets; $unlimitedPlans['professional']['active_campaigns_unlimited'] = '1';
check(Darkphish\Licensing\sanitizePlans($unlimitedPlans)['professional']['active_campaigns'] === -1, 'Unlimited preset not accepted');
$presets['professional']['managed_users'] = '-5';
check(Darkphish\Licensing\sanitizePlans($presets) === $cleanPlans, 'Invalid presets overwrote saved values');
ob_start(); Darkphish\Licensing\adminPage(); $dashboard = ob_get_clean();
foreach (['Dashboard', 'Community', 'Professional', 'Enterprise', 'Settings', 'License overview'] as $label) { check(str_contains($dashboard, $label), 'Missing dashboard tab'); }
check(!str_contains($dashboard, $fromForm['license_key']), 'Dashboard leaks issued key');
$_GET = []; $_POST = []; $_REQUEST = []; wp_set_current_user(0);
echo "Edition migration, admin capability/nonce, paid issuance/edit/audit, recovery isolation, menu ordering and pagination passed.\n";
