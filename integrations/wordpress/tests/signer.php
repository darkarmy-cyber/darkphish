<?php
declare(strict_types=1);
require_once dirname(__DIR__) . '/darkphish-license/includes/Signer.php';
use Darkphish\Licensing\Signer;

function check(bool $condition, string $message): void {
    if (!$condition) { throw new RuntimeException($message); }
}
$pair = sodium_crypto_sign_keypair();
$signer = new Signer('test-ephemeral', sodium_crypto_sign_secretkey($pair));
$now = 1789552800;
$license = ['id' => 'DP-test', 'expires_at' => $now + 365 * 86400, 'managed_users' => 100, 'active_campaigns' => 1];
$envelope = $signer->lease($license, '12345678-1234-1234-1234-123456789012', $now);
$payload = base64_decode(strtr($envelope['payload'], '-_', '+/'), true);
$signature = base64_decode(strtr($envelope['signature'], '-_', '+/'), true);
check(sodium_crypto_sign_verify_detached($signature, $payload, sodium_crypto_sign_publickey($pair)), 'Signature invalid');
check(!sodium_crypto_sign_verify_detached($signature, $payload . 'x', sodium_crypto_sign_publickey($pair)), 'Tampered lease accepted');
$decoded = json_decode($payload, true, 16, JSON_THROW_ON_ERROR);
check($decoded['expires_at'] === $now + 30 * 86400 && $decoded['grace_until'] === $now + 60 * 86400, 'Incorrect lease bounds');
$short = $signer->lease(array_replace($license, ['expires_at' => $now + 600]), $decoded['installation_id'], $now);
$shortPayload = json_decode(base64_decode(strtr($short['payload'], '-_', '+/')), true);
check($shortPayload['grace_until'] === $now + 600, 'Lease outlives license');
try { $signer->lease(array_replace($license, ['expires_at' => $now]), 'installation', $now); throw new RuntimeException('Expired license accepted'); }
catch (DomainException $expected) {}
foreach (['professional', 'enterprise'] as $edition) {
    $paid = $signer->lease(array_replace($license, ['edition' => $edition]), $decoded['installation_id'], $now);
    $body = base64_decode(strtr($paid['payload'], '-_', '+/'));
    check(json_decode($body, true)['edition'] === $edition, 'Paid edition changed');
    check(sodium_crypto_sign_verify_detached(base64_decode(strtr($paid['signature'], '-_', '+/')), $body, sodium_crypto_sign_publickey($pair)), 'Paid signature invalid');
}
try { $signer->lease(array_replace($license, ['edition' => 'unknown']), 'installation', $now); throw new RuntimeException('Unknown edition signed'); }
catch (DomainException $expected) {}
$fixture = ['envelope' => $envelope, 'keyring' => $signer->keyring(), 'now' => $now, 'installation_id' => $decoded['installation_id']];
if (isset($argv[1])) { file_put_contents($argv[1], json_encode($fixture, JSON_THROW_ON_ERROR)); }
echo "Signer policy, tamper and expiry tests passed.\n";
