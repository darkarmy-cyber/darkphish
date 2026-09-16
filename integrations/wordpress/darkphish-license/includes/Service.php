<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

final class Service {
    public function __construct(private Store $store, private ?Signer $signer = null) {}

    public function request(string $email, string $terms, int $now, string $purpose = 'register'): string {
        $email = strtolower(trim($email));
        if (!is_email($email) || strlen($email) > 254 || !in_array($purpose, ['register', 'recover'], true)) {
            throw new \InvalidArgumentException('Invalid request');
        }
        if (EmailPolicy::blocked($email)) { throw new EmailDomainBlocked(); }
        $token = Signer::encode(random_bytes(32));
        $this->store->insert('requests', ['token_hash' => hash('sha256', $token), 'email' => $email, 'terms_version' => $terms, 'purpose' => $purpose, 'expires_at' => $now + 1800]);
        return $token;
    }

    public function verify(string $token, int $now): array {
        return $this->store->transaction(function () use ($token, $now): array {
            $hash = hash('sha256', $token);
            $request = $this->store->find('requests', 'token_hash', $hash, true);
            if (!$request || (int) $request['expires_at'] <= $now) { throw new \DomainException('Verification unavailable'); }
            $purpose = $request['purpose'] ?? 'register';
            if (!in_array($purpose, ['register', 'recover'], true)) { throw new \DomainException('Verification unavailable'); }
            $email = strtolower(trim($request['email']));
            if (EmailPolicy::blocked($email)) {
                $this->store->deleteRequest($hash);
                return ['outcome' => 'email_domain_blocked'];
            }
            $matches = $this->store->communityForEmail($email);
            $license = $matches[0] ?? null;
            // These results are disclosed only after proof of mailbox ownership.
            $outcome = count($matches) > 1 ? 'support_required' : null;
            if (!$outcome && $purpose === 'register' && $license) { $outcome = 'already_registered'; }
            if (!$outcome && $purpose === 'recover') {
                if (!$license) { $outcome = 'not_found'; }
                elseif ($license['status'] !== 'active') { $outcome = 'revoked'; }
                elseif ((int) $license['expires_at'] <= $now) { $outcome = 'expired'; }
            }
            if ($outcome) {
                $this->store->deleteRequest($hash);
                return ['outcome' => $outcome];
            }
            $key = 'DP-COM-' . Signer::encode(random_bytes(32));
            if ($purpose === 'register') {
                $license = ['id' => 'DP-' . bin2hex(random_bytes(16)), 'email' => $email, 'email_hash' => hash('sha256', $email),
                    'edition' => 'community', 'key_hash' => hash('sha256', $key), 'status' => 'active', 'managed_users' => 100, 'active_campaigns' => 1,
                    'expires_at' => $now + 365 * 86400, 'created_at' => $now, 'terms_version' => $request['terms_version']];
                $this->store->insert('licenses', $license);
                $this->store->event($license['id'], 'license.issue');
            } else {
                // Recovery changes only the key. No renewal, unbinding or revocation bypass.
                $this->store->update($license['id'], ['key_hash' => hash('sha256', $key)]);
                $this->store->event($license['id'], 'license.key.replace');
            }
            $this->store->deleteRequest($hash);
            return ['outcome' => $purpose === 'recover' ? 'recovered' : 'issued', 'license_key' => $key,
                'license_id' => $license['id'], 'expires_at' => (int) $license['expires_at']];
        });
    }

    // Paid issuance is administrative only. Community self-service never looks up these hashes.
    public function issuePaid(string $edition, string $email, int $users, int $campaigns, int $days, string $terms, int $actor, int $now): array {
        $email = strtolower(trim($email));
        if (!in_array($edition, ['professional', 'enterprise'], true) || !is_email($email) || strlen($email) > 254 ||
            ($users !== -1 && ($users < 1 || $users > 100000000)) || ($campaigns !== -1 && ($campaigns < 1 || $campaigns > 1000000)) || $days < 1 || $days > 3650 ||
            trim($terms) === '' || strlen($terms) > 80 || $actor < 1) { throw new \InvalidArgumentException('Invalid license details'); }
        return $this->store->transaction(function () use ($edition, $email, $users, $campaigns, $days, $terms, $actor, $now): array {
            $emailHash = hash('sha256', $edition . ':' . $email);
            if ($this->store->find('licenses', 'email_hash', $emailHash, true)) { throw new \DomainException('This email already has a license in this edition'); }
            $key = ($edition === 'professional' ? 'DP-PRO-' : 'DP-ENT-') . Signer::encode(random_bytes(32));
            $record = ['id' => 'DP-' . bin2hex(random_bytes(16)), 'edition' => $edition, 'email' => $email, 'email_hash' => $emailHash,
                'key_hash' => hash('sha256', $key), 'status' => 'active', 'managed_users' => $users, 'active_campaigns' => $campaigns,
                'expires_at' => $now + $days * 86400, 'created_at' => $now, 'terms_version' => $terms];
            $this->store->insert('licenses', $record);
            $this->store->event($record['id'], 'admin.issue.' . $edition, $actor);
            return ['license_key' => $key, 'license_id' => $record['id']];
        });
    }

    public function editPaid(string $id, string $edition, int $users, int $campaigns, int $expiry, int $actor, int $now): void {
        if (!in_array($edition, ['professional', 'enterprise'], true) || ($users !== -1 && ($users < 1 || $users > 100000000)) ||
            ($campaigns !== -1 && ($campaigns < 1 || $campaigns > 1000000)) || $expiry <= $now || $expiry > $now + 3651 * 86400 || $actor < 1) {
            throw new \InvalidArgumentException('Invalid license details');
        }
        $this->store->transaction(function () use ($id, $edition, $users, $campaigns, $expiry, $actor): void {
            $license = $this->store->find('licenses', 'id', $id, true);
            if (!$license || $license['edition'] !== $edition) { throw new \DomainException('License unavailable'); }
            $this->store->update($id, ['managed_users' => $users, 'active_campaigns' => $campaigns, 'expires_at' => $expiry]);
            $this->store->event($id, 'admin.edit', $actor);
        });
    }

    public function exchange(string $credential, string $installation, string $version, bool $refresh, int $now): array {
        if (!$this->signer) { throw new \RuntimeException('Signing is not configured'); }
        if (!preg_match('/^[A-Za-z0-9-]{16,80}$/D', $installation) || !preg_match('/^[A-Za-z0-9.+_-]{1,40}$/D', $version) || strlen($credential) < 32 || strlen($credential) > 128) {
            throw new \DomainException('License unavailable');
        }
        return $this->store->transaction(function () use ($credential, $installation, $version, $refresh, $now): array {
            $license = $this->store->find('licenses', $refresh ? 'refresh_hash' : 'key_hash', hash('sha256', $credential), true);
            if (!$license || $license['status'] !== 'active' || (int) $license['expires_at'] <= $now ||
                ($refresh && $license['installation'] !== $installation) ||
                (!$refresh && $license['installation'] !== '' && $license['installation'] !== $installation)) {
                throw new \DomainException('License unavailable');
            }
            $result = ['lease' => $this->signer->lease($license, $installation, $now)];
            $updates = ['installation' => $installation, 'product_version' => $version, 'last_seen' => $now];
            if (!$refresh) {
                $result['refresh_token'] = Signer::encode(random_bytes(32));
                $updates['refresh_hash'] = hash('sha256', $result['refresh_token']);
            }
            $this->store->update($license['id'], $updates);
            $this->store->event($license['id'], $refresh ? 'lease.refresh' : 'installation.activate');
            return $result;
        });
    }

    public function administer(string $id, string $action, int $actor, int $now): void {
        $this->store->transaction(function () use ($id, $action, $actor, $now): void {
            $license = $this->store->find('licenses', 'id', $id, true);
            if (!$license) {
                throw new \DomainException('License unavailable');
            }
            $updates = match ($action) {
                'revoke' => ['status' => 'revoked', 'refresh_hash' => ''],
                'restore' => ['status' => 'active'],
                'reset' => ['installation' => '', 'refresh_hash' => ''],
                'renew' => ['expires_at' => max($now, (int) $license['expires_at']) + 365 * 86400],
                default => throw new \DomainException('Invalid action'),
            };
            $this->store->update($id, $updates);
            $this->store->event($id, 'admin.' . $action, $actor);
        });
    }
}
