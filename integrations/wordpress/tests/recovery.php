<?php
declare(strict_types=1);
// Disposable database, mock mail only. Exercise recovery without extending a license.
$now = time(); $issuedAt = $now - 60 * 86400;
$email = 'recovery-test@example.test';
$issuedToken = $service->request(' Recovery-Test@Example.Test ', 'original-terms', $issuedAt);
$original = $service->verify($issuedToken, $issuedAt);
$service->exchange($original['license_key'], '12345678-1234-1234-1234-111111111111', 'test', false, $issuedAt);
$beforeRecovery = $db->find('licenses', 'id', $original['license_id']);
$registerAgain = $service->request('RECOVERY-TEST@example.test', 'new-terms', $now);
check($service->verify($registerAgain, $now)['outcome'] === 'already_registered', 'Repeat registration did not require recovery');
check($db->find('licenses', 'id', $original['license_id']) === $beforeRecovery, 'Repeat registration changed existing license');
try { $service->verify($registerAgain, $now); throw new LogicException('Duplicate registration token replayed'); } catch (DomainException $expected) {}

$recoveryInput = ['email' => $email, 'challenge_token' => 'valid-test-token'];
$siblingRecovery = $service->request($email, '', $now, 'recover');
$unrelatedRecovery = $service->request('unrelated@example.test', '', $now, 'recover');
$knownResponse = call_api('recover', $recoveryInput, 'https://darkphish.test');
check($knownResponse->get_status() === 202, 'Recovery request failed');
$recoveryMail = $mails[array_key_last($mails)];
check(str_contains($recoveryMail['message'], 'Recovery does not extend'), 'Recovery mail does not explain unchanged expiry');
preg_match('/#dp-verify=([A-Za-z0-9_-]{43})/', $recoveryMail['message'], $recoveryMatch);
check(isset($recoveryMatch[1]), 'Recovery verification token missing');
check($db->find('licenses', 'id', $original['license_id']) === $beforeRecovery, 'Request changed license before mailbox verification');
$unknownResponse = call_api('recover', array_replace($recoveryInput, ['email' => 'unknown-recovery@example.test']), 'https://darkphish.test');
check($unknownResponse->get_status() === $knownResponse->get_status() && $unknownResponse->get_data() === $knownResponse->get_data(), 'Recovery request disclosed license existence');
$recoveredResponse = call_api('verify', ['token' => $recoveryMatch[1]]);
$recovered = $recoveredResponse->get_data();
check($recoveredResponse->get_status() === 200 && $recovered['outcome'] === 'recovered', 'Verified recovery failed');
check($recovered['license_id'] === $original['license_id'] && $recovered['expires_at'] === $original['expires_at'], 'Recovery renewed or replaced the license');
check($recovered['license_key'] !== $original['license_key'] && $recovered['email_accepted'] === true, 'Recovery key not replaced/emailed');
$afterRecovery = $db->find('licenses', 'id', $original['license_id']);
check($afterRecovery === array_replace($beforeRecovery, ['key_hash' => hash('sha256', $recovered['license_key'])]), 'Recovery altered limits, binding, refresh credential or terms');
check(str_contains($mails[array_key_last($mails)]['message'], $recovered['license_key']), 'Recovered key missing from email');
check(call_api('verify', ['token' => $recoveryMatch[1]])->get_status() === 403, 'Recovery verification replayed');
check(call_api('verify', ['token' => $siblingRecovery])->get_status() === 403, 'Sibling recovery rotated the displayed key');
check($db->find('licenses', 'id', $original['license_id'])['key_hash'] === hash('sha256', $recovered['license_key']), 'Sibling token invalidated the successful key');
check($db->find('requests', 'token_hash', hash('sha256', $unrelatedRecovery)) !== null, 'Recovery invalidated another mailbox');
check(count($db->communityForEmail($email)) === 1, 'Duplicate Community license created');

$workers = [];
foreach ([$service->request($email, '', $now, 'recover'), $service->request($email, '', $now, 'recover')] as $token) {
    $pipes = [];
    $process = proc_open([PHP_BINARY, __DIR__ . '/integration.php', 'register-compete', $token], [0 => ['pipe', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']], $pipes);
    check(is_resource($process), 'Could not start recovery worker'); fclose($pipes[0]);
    $workers[] = [$process, $pipes, $token];
}
$recoveryWinners = [];
foreach ($workers as [$process, $pipes, $token]) {
    $out = stream_get_contents($pipes[1]); $err = stream_get_contents($pipes[2]); fclose($pipes[1]); fclose($pipes[2]);
    check(proc_close($process) === 0, 'Recovery worker failed: ' . $err);
    preg_match('/RESULT:(.*)/', $out, $found); $result = json_decode($found[1] ?? '', true);
    if (($result['status'] ?? 0) === 503) {
        $retry = call_api('verify', ['token' => $token]); $data = $retry->get_data();
        $result = ['status' => $retry->get_status(), 'outcome' => $data['outcome'] ?? '', 'key_hash' => isset($data['license_key']) ? hash('sha256', $data['license_key']) : ''];
    }
    if ($result['status'] === 200 && $result['outcome'] === 'recovered') { $recoveryWinners[] = $result['key_hash']; }
    else { check($result['status'] === 403, 'Sibling recovery was not invalidated'); }
}
check(count($recoveryWinners) === 1 && $db->find('licenses', 'id', $original['license_id'])['key_hash'] === $recoveryWinners[0], 'Concurrent recovery invalidated a successfully returned key');

$failureToken = $service->request($email, '', $now, 'recover');
$failMail = static fn () => false;
add_filter('pre_wp_mail', $failMail, PHP_INT_MAX);
try { $mailFailure = call_api('verify', ['token' => $failureToken])->get_data(); }
finally { remove_filter('pre_wp_mail', $failMail, PHP_INT_MAX); }
check($mailFailure['outcome'] === 'recovered' && $mailFailure['email_accepted'] === false && $mailFailure['expires_at'] === $original['expires_at'], 'Mail failure lost recovery result or changed expiry');
check($db->find('licenses', 'id', $original['license_id'])['key_hash'] === hash('sha256', $mailFailure['license_key']), 'Displayed recovery key is not usable after mail failure');

foreach (['missing' => 'not_found', 'expired' => 'expired', 'revoked' => 'revoked'] as $state => $outcome) {
    $target = $state === 'missing' ? 'missing@example.test' : $email;
    if ($state === 'expired') { $db->update($original['license_id'], ['expires_at' => $now - 1]); }
    if ($state === 'revoked') { $db->update($original['license_id'], ['expires_at' => $now + 1000, 'status' => 'revoked']); }
    $before = $db->find('licenses', 'id', $original['license_id']);
    $token = $service->request($target, '', $now, 'recover');
    $result = $service->verify($token, $now);
    check($result === ['outcome' => $outcome], 'Invalid recovery outcome');
    check($db->find('licenses', 'id', $original['license_id']) === $before, 'Unavailable license changed');
    if ($state === 'missing') { check($db->communityForEmail($target) === [], 'Recovery created a new license'); }
}

// A legacy row with a noncanonical hash must not be mistaken for a new address.
$db->update($original['license_id'], ['status' => 'active', 'expires_at' => $original['expires_at'], 'email_hash' => hash('sha256', 'legacy-hash')]);
$legacyToken = $service->request($email, 'new-terms', $now);
check($service->verify($legacyToken, $now)['outcome'] === 'already_registered', 'Legacy address lookup bypassed');
$duplicate = $db->find('licenses', 'id', $original['license_id']);
$duplicate['id'] = 'DP-test-duplicate'; $duplicate['email'] = strtoupper($email);
$duplicate['email_hash'] = hash('sha256', 'second-legacy-hash'); $duplicate['key_hash'] = hash('sha256', 'duplicate-test-key');
$db->insert('licenses', $duplicate);
check($db->duplicateCommunityEmails() === 1, 'Legacy duplicate not diagnosed');
foreach (['register', 'recover'] as $purpose) {
    $token = $service->request($email, 'test', $now, $purpose);
    check($service->verify($token, $now) === ['outcome' => 'support_required'], 'Ambiguous records changed');
}
check(count($db->communityForEmail($email)) === 2, 'Duplicate records were destructively merged');
$plan = $wpdb->get_row($wpdb->prepare("EXPLAIN SELECT * FROM {$db->prefix}licenses WHERE edition='community' AND canonical_email_hash=%s ORDER BY id LIMIT 2 FOR UPDATE", hash('sha256', $email)), ARRAY_A);
check(($plan['key'] ?? '') === 'community_email' && ($plan['type'] ?? '') !== 'ALL', 'Verification is not an indexed mailbox lookup');

$tokens = [$service->request('concurrent@example.test', 'test', $now), $service->request('CONCURRENT@example.test', 'test', $now)];
$workers = [];
foreach ($tokens as $token) {
    $pipes = [];
    $process = proc_open([PHP_BINARY, __DIR__ . '/integration.php', 'register-compete', $token], [0 => ['pipe', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']], $pipes);
    check(is_resource($process), 'Could not start registration worker'); fclose($pipes[0]);
    $workers[] = [$process, $pipes, $token];
}
$issuedCount = 0;
foreach ($workers as [$process, $pipes, $token]) {
    $out = stream_get_contents($pipes[1]); $err = stream_get_contents($pipes[2]); fclose($pipes[1]); fclose($pipes[2]);
    check(proc_close($process) === 0, 'Registration worker failed: ' . $err);
    preg_match('/RESULT:(.*)/', $out, $found); $result = json_decode($found[1] ?? '', true);
    if (($result['status'] ?? 0) === 503) { // A transaction deadlock may require a retry.
        $retry = call_api('verify', ['token' => $token]);
        $result = ['status' => $retry->get_status(), 'outcome' => $retry->get_data()['outcome'] ?? ''];
    }
    check($result['status'] === 200 && in_array($result['outcome'], ['issued', 'already_registered'], true), 'Unexpected registration result');
    if ($result['outcome'] === 'issued') { $issuedCount++; }
}
check($issuedCount === 1 && count($db->communityForEmail('concurrent@example.test')) === 1, 'Concurrent requests created duplicate licenses');
echo "Recovery passed: unique registration, normalized email, verified-only lookup, unchanged expiry/binding/limits, replacement email, unavailable licenses and legacy duplicates.\n";
