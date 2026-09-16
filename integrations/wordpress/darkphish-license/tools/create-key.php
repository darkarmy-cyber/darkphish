<?php
declare(strict_types=1);
namespace Darkphish\Licensing;
// Run only over a server terminal; never as a web request. Prints public data only.
if (PHP_SAPI !== 'cli') { http_response_code(404); exit; }
if (!function_exists('sodium_crypto_sign_keypair')) { fwrite(STDERR, "PHP sodium is required.\n"); exit(1); }
require_once dirname(__DIR__) . '/includes/Signer.php';
if ($argc !== 3 || !preg_match('/^[A-Za-z0-9._-]{1,80}$/D', $argv[2])) {
    fwrite(STDERR, "Usage: php create-key.php /private/path/key.json KEY-ID\n"); exit(1);
}
try { SigningPath::requireDirectAbsolute($argv[1]); }
catch (\Throwable $error) { \fwrite(STDERR, "A direct absolute private signing path without symlinks is required.\n"); exit(1); }
umask(0077);
$handle = @fopen($argv[1], 'x');
if (!$handle) { fwrite(STDERR, "Cannot create key file; existing files are never overwritten.\n"); exit(1); }
$complete = false;
$created = fstat($handle);
$pair = $secret = $json = '';
try {
    $pair = sodium_crypto_sign_keypair();
    $secret = sodium_crypto_sign_secretkey($pair);
    $json = json_encode(['key_id' => $argv[2], 'secret_key' => base64_encode($secret)], JSON_THROW_ON_ERROR);
    if (fwrite($handle, $json . "\n") !== strlen($json) + 1 || !fflush($handle) || !fsync($handle)) { throw new \RuntimeException('Unable to persist key'); }
    $complete = true;
    echo json_encode((new \Darkphish\Licensing\Signer($argv[2], $secret))->keyring(), JSON_PRETTY_PRINT | JSON_THROW_ON_ERROR) . "\n";
} catch (\Throwable $error) {
    // Do not print exception details that could expose a private path or data.
    \fwrite(STDERR, $complete ? "Key saved; public keyring output failed. Do not recreate the key.\n" : "Unable to persist key; initialization did not complete.\n");
    $failed = true;
} finally {
    fclose($handle);
    if (!$complete) {
        clearstatcache(true, $argv[1]);
        $current = @lstat($argv[1]);
        if ($created && $current && $current['dev'] === $created['dev'] && $current['ino'] === $created['ino']) { @unlink($argv[1]); }
    }
    if ($json !== '') { sodium_memzero($json); }
    if ($secret !== '') { sodium_memzero($secret); }
    if ($pair !== '') { sodium_memzero($pair); }
}
if ($failed ?? false) { exit(1); }
