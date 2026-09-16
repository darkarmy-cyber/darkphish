<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

final class Signer {
    public function __construct(private string $id, #[\SensitiveParameter] private string $secret) {
        if (!preg_match('/^[A-Za-z0-9._-]{1,80}$/D', $id) || strlen($secret) !== SODIUM_CRYPTO_SIGN_SECRETKEYBYTES) {
            throw new \RuntimeException('Invalid signing configuration');
        }
    }

    public static function fromFile(string $path, string $webRoot): self {
        $file = realpath($path);
        $root = realpath($webRoot);
        if (!$file || !$root || !is_file($file) || !is_readable($file) || filesize($file) > 4096) {
            throw new \RuntimeException('Signing file unavailable');
        }
        $prefix = rtrim(str_replace('\\', '/', $root), '/') . '/';
        if (str_starts_with(strtolower(str_replace('\\', '/', $file)), strtolower($prefix))) {
            throw new \RuntimeException('Signing file must be outside the web root');
        }
        if (PHP_OS_FAMILY !== 'Windows' && (fileperms($file) & 0077) !== 0) {
            throw new \RuntimeException('Signing file must have private permissions');
        }
        $raw = @file_get_contents($file);
        if ($raw === false) { throw new \RuntimeException('Signing file unavailable'); }
        $data = json_decode($raw, true, 8, JSON_THROW_ON_ERROR);
        $secret = base64_decode($data['secret_key'] ?? '', true);
        if (!is_string($secret)) {
            throw new \RuntimeException('Invalid signing configuration');
        }
        return new self($data['key_id'] ?? '', $secret);
    }

    public function keyring(): array {
        return ['schema' => 'darkphish-license-keyring/v1', 'keys' => [$this->id => base64_encode(sodium_crypto_sign_publickey_from_secretkey($this->secret))]];
    }

    public function lease(array $license, string $installation, int $now): array {
        $edition = $license['edition'] ?? 'community';
        if (!in_array($edition, ['community', 'professional', 'enterprise'], true)) { throw new \DomainException('Invalid edition'); }
        $expiry = min($now + 30 * 86400, (int) $license['expires_at']);
        if ($expiry <= $now) {
            throw new \DomainException('License unavailable');
        }
        $payload = json_encode([
            'schema' => 'darkphish-license-lease/v1', 'product' => 'darkphish', 'edition' => $edition,
            'license_id' => $license['id'], 'installation_id' => $installation,
            'issued_at' => $now, 'not_before' => $now, 'expires_at' => $expiry,
            'grace_until' => min($expiry + 30 * 86400, (int) $license['expires_at']),
            'entitlements' => ['managed_users' => (int) $license['managed_users'], 'active_campaigns' => (int) $license['active_campaigns']],
        ], JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
        return ['key_id' => $this->id, 'algorithm' => 'Ed25519', 'payload' => self::encode($payload), 'signature' => self::encode(sodium_crypto_sign_detached($payload, $this->secret))];
    }

    public static function encode(string $bytes): string {
        return rtrim(strtr(base64_encode($bytes), '+/', '-_'), '=');
    }
}
