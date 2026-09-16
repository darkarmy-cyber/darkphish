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
    // Exercise the documented CLI itself, not just the admin setup helper.
    $cliPath = $base . '/private/cli-key.json';
    $cli = dirname(__DIR__) . '/darkphish-license/tools/create-key.php';
    $runCli = function (string $fault) use ($cli, $cliPath): array {
        $code = '$GLOBALS["keySetupFault"]=' . var_export($fault, true) . '; require ' . var_export(__DIR__ . '/key-setup-faults.php', true) . '; $argv=["create-key.php",' . var_export($cliPath, true) . ',"cli-test"]; $argc=3; require ' . var_export($cli, true) . ';';
        $command = [PHP_BINARY];
        if (PHP_OS_FAMILY === 'Windows') { array_push($command, '-d', 'extension_dir=' . ini_get('extension_dir'), '-d', 'extension=php_sodium.dll'); }
        array_push($command, '-r', $code);
        $process = proc_open($command, [0 => ['pipe', 'r'], 1 => ['pipe', 'w'], 2 => ['pipe', 'w']], $pipes);
        check(is_resource($process), 'CLI test process unavailable'); fclose($pipes[0]);
        $out = stream_get_contents($pipes[1]); $err = stream_get_contents($pipes[2]); fclose($pipes[1]); fclose($pipes[2]);
        return [proc_close($process), $out, $err];
    };
    foreach (['write', 'flush', 'sync'] as $fault) {
        [$status, $out] = $runCli($fault);
        clearstatcache(true, $cliPath);
        check($status === 1 && $out === '' && !file_exists($cliPath), 'Failed CLI initialization retained a partial key');
    }
    [$status, $out] = $runCli('');
    check($status === 0 && isset(json_decode($out, true)['keys']['cli-test']), 'CLI retry did not produce a usable public keyring');
    $cliDigest = hash_file('sha256', $cliPath);
    [$status] = $runCli('');
    check($status === 1 && hash_file('sha256', $cliPath) === $cliDigest, 'CLI replaced an existing key');
    unlink($cliPath);
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
        symlink($path, $base . '/public/key-alias.json');
        symlink($base . '/private', $base . '/public/dir-alias');
        try {
            rejected(fn () => Signer::fromFile($base . '/public/key-alias.json', $base . '/public'));
            rejected(fn () => Signer::fromFile($base . '/public/dir-alias/key.json', $base . '/public'));
            rejected(fn () => KeySetup::create($base . '/public/dir-alias/new.json', 'test', [$base . '/public']));
            check(!file_exists($base . '/private/new.json'), 'Public alias setup created a signing key');
        } finally { unlink($base . '/public/key-alias.json'); unlink($base . '/public/dir-alias'); }
        symlink($base . '/public', $base . '/link');
        rejected(fn () => KeySetup::create($base . '/link/key.json', 'test', [$base . '/public']));
        unlink($base . '/link');
        chmod($base . '/private', 0777);
        rejected(fn () => KeySetup::create($base . '/private/other.json', 'test', [$base . '/public']));
        chmod($base . '/private', 0700);
    }
    echo "No-SSH key setup passed: public-path rejection, exclusive creation, key loading and private permissions.\n";
} finally {
    if (isset($cliPath) && is_file($cliPath)) { unlink($cliPath); }
    if (is_file($path)) { unlink($path); }
    rmdir($base . '/private'); rmdir($base . '/public'); rmdir($base);
}
