<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

final class KeySetup {
    /** The path comes only from wp-config.php, never from an HTTP parameter. */
    public static function create(string $path, string $id, array $publicRoots): void {
        if (!function_exists('sodium_crypto_sign_keypair') || !preg_match('/^[A-Za-z0-9._-]{1,80}$/D', $id)) {
            throw new \RuntimeException('Invalid signing setup');
        }
        if (!str_starts_with($path, '/') && !preg_match('/^[A-Za-z]:[\\\\\/]/', $path)) {
            throw new \RuntimeException('An absolute signing path is required');
        }
        $parent = realpath(dirname($path));
        if (!$parent || !is_dir($parent) || !is_writable($parent) || !$publicRoots) {
            throw new \RuntimeException('Private signing directory unavailable');
        }
        $target = $parent . DIRECTORY_SEPARATOR . basename($path);
        $normalized = strtolower(str_replace('\\', '/', $target));
        foreach ($publicRoots as $root) {
            $resolved = realpath($root);
            if (!$resolved || !is_dir($resolved)) { throw new \RuntimeException('Public directory cannot be verified'); }
            $prefix = strtolower(rtrim(str_replace('\\', '/', $resolved), '/') . '/');
            if (str_starts_with($normalized, $prefix)) { throw new \RuntimeException('Signing directory must be outside public directories'); }
        }
        if (PHP_OS_FAMILY !== 'Windows' && (fileperms($parent) & 0022) !== 0) {
            throw new \RuntimeException('Private signing directory must not be writable by other users');
        }
        // Exclusive creation also rejects an existing symlink or a competing setup.
        $previousMask = umask(0077);
        try { $handle = @fopen($target, 'xb'); }
        finally { umask($previousMask); }
        if (!$handle) { throw new \RuntimeException('Signing file already exists or cannot be created'); }
        $pair = $secret = '';
        try {
            if (PHP_OS_FAMILY !== 'Windows' && ((fstat($handle)['mode'] ?? 0777) & 0077) !== 0) {
                throw new \RuntimeException('Private signing permissions could not be set');
            }
            $pair = sodium_crypto_sign_keypair();
            $secret = sodium_crypto_sign_secretkey($pair);
            $json = json_encode(['key_id' => $id, 'secret_key' => base64_encode($secret)], JSON_THROW_ON_ERROR) . "\n";
            if (fwrite($handle, $json) !== strlen($json) || !fflush($handle) || !fsync($handle)) {
                throw new \RuntimeException('Signing file could not be saved');
            }
            sodium_memzero($json);
        } finally {
            fclose($handle);
            if ($secret !== '') { sodium_memzero($secret); }
            if ($pair !== '') { sodium_memzero($pair); }
        }
    }
}
