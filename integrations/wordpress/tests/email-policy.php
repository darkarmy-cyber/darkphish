<?php
declare(strict_types=1);
use Darkphish\Licensing\EmailPolicy;
use Darkphish\Licensing\EmailDomainBlocked;

check(EmailPolicy::settings() === EmailPolicy::defaults(), 'Missing option did not load starter domains');
foreach (['someone@temp-mail.org', ' SOMEONE@YOPMAIL.COM ', 'someone@inbox.guerrillamail.com'] as $email) {
    check(EmailPolicy::blocked($email), 'Disposable email allowed: ' . $email);
}
foreach (['someone@gmail.com', 'someone@outlook.com', 'someone@proton.me', 'someone@darkphish.sk', 'someone@notyopmail.com', 'someone@yopmail.com.example.test', 'yopmail.com@example.test'] as $email) {
    check(!EmailPolicy::blocked($email), 'Legitimate or lookalike domain blocked: ' . $email);
}
$custom = ['enabled' => '1', 'domains' => "  BLOCKED.TEST \r\nblocked.test\nsecond.test\n"];
update_option('darkphish_email_policy', $custom);
$saved = ['enabled' => true, 'domains' => "blocked.test\nsecond.test"];
check(EmailPolicy::settings() === $saved, 'Settings did not normalize/deduplicate');
check(EmailPolicy::sanitize($saved) === $saved, 'Sanitizer is not idempotent');
foreach ([null, 'bad', new stdClass(), ['domains' => []], ['domains' => 'bad.test', 'enabled' => []]] as $bad) {
    check(EmailPolicy::sanitize($bad) === $saved, 'Malformed settings replaced saved settings');
}
foreach (['https://bad.test', 'mail@bad.test', '*.bad.test', 'com', '-bad.test', 'bad..test', '127.0.0.1', '<script>alert(1)</script>', str_repeat('a', 64) . '.test', str_repeat('a', 65537)] as $bad) {
    update_option('darkphish_email_policy', ['enabled' => '1', 'domains' => $bad]);
    check(EmailPolicy::settings() === $saved, 'Invalid list overwrote prior policy');
}
update_option('darkphish_email_policy', ['enabled' => '1', 'domains' => '']);
check(!EmailPolicy::blocked('owner@yopmail.com') && EmailPolicy::settings()['domains'] === '', 'Empty list reloaded defaults');
update_option('darkphish_email_policy', ['domains' => 'blocked.test']);
check(!EmailPolicy::blocked('owner@blocked.test'), 'Unchecked checkbox did not disable protection');
update_option('darkphish_email_policy', $saved);
do_action('plugins_loaded');
check(EmailPolicy::settings() === $saved, 'Plugin load reset custom policy');

$mailCount = count($mails);
$requestCount = (int) $wpdb->get_var("SELECT COUNT(*) FROM {$db->prefix}requests");
$_SERVER['REMOTE_ADDR'] = '192.0.2.23';
$currentTerms = get_option('darkphish_license_settings')['terms_version'];
foreach (['request', 'recover'] as $operation) {
    $data = ['email' => 'Owner@Blocked.Test', 'challenge_token' => 'valid-test-token'];
    if ($operation === 'request') { $data['terms_version'] = $currentTerms; }
    $response = call_api($operation, $data, 'https://darkphish.test');
    check($response->get_status() === 400 && $response->get_data() === ['code' => 'email_domain_blocked', 'message' => EmailPolicy::MESSAGE], 'Blocked API response missing');
    try { $service->request($data['email'], $currentTerms, time(), $operation === 'recover' ? 'recover' : 'register'); throw new LogicException('Direct service bypassed email policy'); }
    catch (EmailDomainBlocked $expected) {}
}
check(count($mails) === $mailCount, 'Blocked request sent email');
check((int) $wpdb->get_var("SELECT COUNT(*) FROM {$db->prefix}requests") === $requestCount, 'Blocked request stored token');

// A domain added after the verification email was sent must not issue/rotate a key.
update_option('darkphish_email_policy', ['enabled' => false, 'domains' => 'blocked.test']);
$now = time();
$issued = $service->verify($service->request('existing@blocked.test', 'test', $now), $now);
$before = $db->find('licenses', 'id', $issued['license_id']);
$pendingRegister = $service->request('new@blocked.test', 'test', $now);
$pendingRecovery = $service->request('existing@blocked.test', '', $now, 'recover');
update_option('darkphish_email_policy', $saved);
foreach ([$pendingRegister, $pendingRecovery] as $token) {
    $response = call_api('verify', ['token' => $token]);
    check($response->get_status() === 200 && $response->get_data() === ['outcome' => 'email_domain_blocked'], 'Pending verification bypassed new policy');
    check(call_api('verify', ['token' => $token])->get_status() === 403, 'Blocked verification token replayed');
}
check(count($mails) === $mailCount, 'Blocked verification emailed a key');
check(!$db->communityForEmail('new@blocked.test'), 'Blocked verification issued a license');
check($db->find('licenses', 'id', $issued['license_id']) === $before, 'Blocking altered existing license');
check(isset($service->exchange($issued['license_key'], '12345678-1234-1234-1234-222222222222', 'test', false, $now)['lease']), 'Blocking broke existing license activation');

// Restore defaults for the remaining HTTP harness. Only this disposable test database is touched.
delete_option('darkphish_email_policy');
$_SERVER['REMOTE_ADDR'] = '127.0.0.1';
echo "Email policy passed: editable defaults, normalization, safe matching, no mail/token on rejection, pending links, unchanged licenses.\n";
