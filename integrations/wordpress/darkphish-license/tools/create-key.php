<?php
declare(strict_types=1);
// Run only over a server terminal; never as a web request. Prints public data only.
if (PHP_SAPI !== 'cli') { http_response_code(404); exit; }
require_once dirname(__DIR__) . '/includes/Signer.php';
if ($argc !== 3 || !preg_match('/^[A-Za-z0-9._-]{1,80}$/D', $argv[2])) {
    fwrite(STDERR, "Usage: php create-key.php /private/path/key.json KEY-ID\n"); exit(1);
}
umask(0077);
$handle = fopen($argv[1], 'x');
if (!$handle) { fwrite(STDERR, "Cannot create key file; existing files are never overwritten.\n"); exit(1); }
try {
    $pair = sodium_crypto_sign_keypair();
    $secret = sodium_crypto_sign_secretkey($pair);
    $json = json_encode(['key_id' => $argv[2], 'secret_key' => base64_encode($secret)], JSON_THROW_ON_ERROR);
    if (fwrite($handle, $json . "\n") !== strlen($json) + 1 || !fflush($handle)) { throw new RuntimeException('Unable to persist key'); }
    echo json_encode((new \Darkphish\Licensing\Signer($argv[2], $secret))->keyring(), JSON_PRETTY_PRINT | JSON_THROW_ON_ERROR) . "\n";
    sodium_memzero($secret); sodium_memzero($pair);
} finally { fclose($handle); }
