<?php
declare(strict_types=1);
require_once __DIR__ . '/key-setup-faults.php';
require_once dirname(__DIR__) . '/darkphish-license/includes/Signer.php';
require_once dirname(__DIR__) . '/darkphish-license/includes/KeySetup.php';
use Darkphish\Licensing\KeySetup;
use Darkphish\Licensing\Signer;
function check(bool $ok, string $message): void { if (!$ok) { throw new LogicException($message); } }
function rejected(callable $operation): void {
    try { $operation(); } catch (RuntimeException $expected) { return; }
    throw new LogicException('Unsafe key creation accepted');
}
$base = sys_get_temp_dir() . '/darkphish-key-setup-' . bin2hex(random_bytes(8));
mkdir($base, 0700); mkdir($base . '/public', 0700); mkdir($base . '/private', 0700);
$path = $base . '/private/key.json';
try {
    rejected(fn () => KeySetup::create($base . '/public/key.json', 'test', [$base . '/public']));
    rejected(fn () => KeySetup::create($base . '/private/../public/key.json', 'test', [$base . '/public']));
    rejected(fn () => KeySetup::create('relative-key.json', 'test', [$base . '/public']));
    rejected(fn () => KeySetup::create($path, 'invalid key id', [$base . '/public']));
    rejected(fn () => KeySetup::create($path, 'test', []));
    rejected(fn () => KeySetup::create($path, 'test', [$base . '/nonexistent']));
    check(!file_exists($path) && !file_exists($base . '/public/key.json'), 'Rejected creation left a file');
    foreach (['write', 'flush', 'sync'] as $fault) {
        $GLOBALS['keySetupFault'] = $fault;
        rejected(fn () => KeySetup::create($path, 'test', [$base . '/public']));
        clearstatcache(true, $path);
        check(!file_exists($path), 'Failed key initialization left an unrecoverable partial file');
    }
    unset($GLOBALS['keySetupFault']);
    KeySetup::create($path, 'test', [$base . '/public']);
    $signer = Signer::fromFile($path, $base . '/public');
    check(isset($signer->keyring()['keys']['test']), 'Generated key cannot be read');
    rejected(fn () => Signer::fromFile($path, [$base . '/public', $base . '/private']));
    rejected(fn () => Signer::fromFile($path, [$base . '/private', $base . '/public']));
    rejected(fn () => Signer::fromFile($path, [$base . '/public', $base . '/missing']));
    rejected(fn () => Signer::fromFile($path, []));
    $before = hash_file('sha256', $path);
    rejected(fn () => KeySetup::create($path, 'replacement', [$base . '/public']));
    check(hash_file('sha256', $path) === $before, 'Existing key was overwritten');
    if (PHP_OS_FAMILY !== 'Windows') {
        check((fileperms($path) & 0777) === 0600, 'Private file permissions incorrect');
        symlink($base . '/public', $base . '/link');
        rejected(fn () => KeySetup::create($base . '/link/key.json', 'test', [$base . '/public']));
        unlink($base . '/link');
        chmod($base . '/private', 0777);
        rejected(fn () => KeySetup::create($base . '/private/other.json', 'test', [$base . '/public']));
        chmod($base . '/private', 0700);
    }
    echo "No-SSH key setup passed: public-path rejection, exclusive creation, key loading and private permissions.\n";
} finally {
    if (is_file($path)) { unlink($path); }
    rmdir($base . '/private'); rmdir($base . '/public'); rmdir($base);
}
