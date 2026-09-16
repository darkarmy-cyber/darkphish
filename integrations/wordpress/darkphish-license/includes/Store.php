<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

final class Store {
    public readonly string $prefix;
    public function __construct(private \wpdb $db) {
        // Licensing failures must not print SQL (including account email) into
        // API responses or debug output, even when WordPress debug is enabled.
        $db->suppress_errors(true);
        $this->prefix = $db->prefix . 'darkphish_';
    }

    public function install(): void {
        require_once ABSPATH . 'wp-admin/includes/upgrade.php';
        $charset = $this->db->get_charset_collate();
        $tables = [
            'licenses' => "id varchar(40) NOT NULL, email varchar(254) NOT NULL, email_hash char(64) NOT NULL, edition varchar(20) NOT NULL DEFAULT 'community', key_hash char(64) NOT NULL, status varchar(16) NOT NULL, managed_users int NOT NULL, active_campaigns int NOT NULL, expires_at bigint NOT NULL, installation varchar(80) NOT NULL DEFAULT '', refresh_hash char(64) NOT NULL DEFAULT '', product_version varchar(40) NOT NULL DEFAULT '', last_seen bigint NOT NULL DEFAULT 0, created_at bigint NOT NULL, terms_version varchar(80) NOT NULL, PRIMARY KEY  (id), UNIQUE KEY email_hash (email_hash), UNIQUE KEY key_hash (key_hash), KEY refresh_hash (refresh_hash)",
            'requests' => "token_hash char(64) NOT NULL, email varchar(254) NOT NULL, expires_at bigint NOT NULL, terms_version varchar(80) NOT NULL, purpose varchar(16) NOT NULL DEFAULT 'register', PRIMARY KEY  (token_hash)",
            'limits' => "bucket char(64) NOT NULL, hits int NOT NULL DEFAULT 0, expires_at bigint NOT NULL, PRIMARY KEY  (bucket)",
            'events' => "id bigint unsigned NOT NULL AUTO_INCREMENT, license_id varchar(40) NOT NULL, action varchar(40) NOT NULL, actor bigint unsigned NOT NULL DEFAULT 0, created_at bigint NOT NULL, PRIMARY KEY  (id), KEY license_id (license_id)",
        ];
        foreach ($tables as $name => $columns) {
            // dbDelta requires one definition per line and PRIMARY KEY's two spaces.
            $sql = 'CREATE TABLE ' . $this->prefix . $name . " (\n" . str_replace(', ', ",\n", $columns) . "\n) ENGINE=InnoDB $charset;";
            dbDelta($sql);
            if ($name === 'licenses' && !$this->db->get_row("SHOW COLUMNS FROM {$this->prefix}licenses LIKE 'edition'")) {
                throw new \RuntimeException('License edition upgrade unavailable');
            }
            if ($name === 'requests' && !$this->db->get_row("SHOW COLUMNS FROM {$this->prefix}requests LIKE 'purpose'")) {
                throw new \RuntimeException('Request purpose upgrade unavailable');
            }
            if ($name === 'licenses') {
                $index = $this->db->get_results("SHOW INDEX FROM {$this->prefix}licenses WHERE Key_name='email_hash'", ARRAY_A);
                if (count($index ?: []) !== 1 || (int) $index[0]['Non_unique'] !== 0 || $index[0]['Column_name'] !== 'email_hash') {
                    throw new \RuntimeException('Unique email index unavailable');
                }
            }
            $actual = $this->db->get_row($this->db->prepare('SHOW TABLE STATUS WHERE Name = %s', $this->prefix . $name), ARRAY_A);
            if (!$actual || strcasecmp($actual['Engine'], 'InnoDB') !== 0) {
                throw new \RuntimeException('Transactional licensing tables unavailable');
            }
        }
    }

    public function transaction(callable $operation): mixed {
        $this->execute('START TRANSACTION');
        try {
            $value = $operation();
            $this->execute('COMMIT');
            return $value;
        } catch (\Throwable $error) {
            $this->db->query('ROLLBACK');
            throw $error;
        }
    }

    public function execute(string $query): void {
        if ($this->db->query($query) === false) {
            throw new \RuntimeException('Licensing storage unavailable');
        }
    }

    public function find(string $table, string $field, string $value, bool $lock = false): ?array {
        if (!in_array($table, ['licenses', 'requests'], true) || !in_array($field, ['id', 'email_hash', 'key_hash', 'refresh_hash', 'token_hash'], true)) {
            throw new \LogicException('Invalid storage lookup');
        }
        $row = $this->db->get_row($this->db->prepare("SELECT * FROM {$this->prefix}$table WHERE $field = %s" . ($lock ? ' FOR UPDATE' : ''), $value), ARRAY_A);
        if ($this->db->last_error !== '') {
            throw new \RuntimeException('Licensing storage unavailable');
        }
        return $row;
    }

    public function insert(string $table, array $record): void {
        if ($this->db->insert($this->prefix . $table, $record) === false) {
            throw new \RuntimeException('Licensing storage unavailable');
        }
    }

    /** Check the stored address too, so legacy noncanonical hashes cannot create new licenses. */
    public function communityForEmail(string $email): array {
        $rows = $this->db->get_results($this->db->prepare("SELECT * FROM {$this->prefix}licenses WHERE edition='community' AND (email_hash=%s OR LOWER(TRIM(email))=%s) ORDER BY id LIMIT 2 FOR UPDATE", hash('sha256', $email), $email), ARRAY_A);
        if ($this->db->last_error !== '') { throw new \RuntimeException('Licensing storage unavailable'); }
        return $rows ?: [];
    }

    public function duplicateCommunityEmails(): int {
        $count = $this->db->get_var("SELECT COUNT(*) FROM (SELECT LOWER(TRIM(email)) FROM {$this->prefix}licenses WHERE edition='community' GROUP BY LOWER(TRIM(email)) HAVING COUNT(*)>1) AS duplicates");
        if ($this->db->last_error !== '') { throw new \RuntimeException('Licensing storage unavailable'); }
        return (int) $count;
    }

    public function update(string $id, array $record): void {
        if ($this->db->update($this->prefix . 'licenses', $record, ['id' => $id]) === false) {
            throw new \RuntimeException('Licensing storage unavailable');
        }
    }

    public function deleteRequest(string $hash): void {
        if ($this->db->delete($this->prefix . 'requests', ['token_hash' => $hash]) !== 1) {
            throw new \DomainException('Verification unavailable');
        }
    }

    public function event(string $id, string $action, int $actor = 0): void {
        $this->insert('events', ['license_id' => $id, 'action' => $action, 'actor' => $actor, 'created_at' => time()]);
    }

    public function rate(string $scope, string $identity, int $limit, int $window, int $now): bool {
        $end = (intdiv($now, $window) + 1) * $window;
        $bucket = hash_hmac('sha256', "$scope:$identity:$end", wp_salt('auth'));
        return $this->transaction(function () use ($bucket, $end, $limit): bool {
            $table = $this->prefix . 'limits';
            $this->execute($this->db->prepare("INSERT INTO $table (bucket,hits,expires_at) VALUES (%s,1,%d) ON DUPLICATE KEY UPDATE hits=hits+1", $bucket, $end));
            $hits = $this->db->get_var($this->db->prepare("SELECT hits FROM $table WHERE bucket=%s FOR UPDATE", $bucket));
            if ($hits === null) {
                throw new \RuntimeException('Rate limiter unavailable');
            }
            return (int) $hits <= $limit;
        });
    }

    public function cleanup(): void {
        $now = time();
        foreach (['requests', 'limits'] as $table) {
            $this->execute($this->db->prepare("DELETE FROM {$this->prefix}$table WHERE expires_at < %d LIMIT 1000", $now));
        }
    }

    public function recentLicenses(string $edition = 'community', int $page = 1): array {
        if (!in_array($edition, ['community', 'professional', 'enterprise'], true)) { throw new \InvalidArgumentException('Invalid edition'); }
        $rows = $this->db->get_results($this->db->prepare("SELECT id,email,edition,status,managed_users,active_campaigns,installation,expires_at FROM {$this->prefix}licenses WHERE edition=%s ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET %d", $edition, (max(1, $page) - 1) * 50), ARRAY_A);
        if ($this->db->last_error !== '') { throw new \RuntimeException('Licensing storage unavailable'); }
        return $rows ?: [];
    }

    public function statistics(int $now): array {
        $rows = $this->db->get_results($this->db->prepare("SELECT edition,COUNT(*) AS total,SUM(status='active' AND expires_at>%d) AS active,SUM(status='active' AND expires_at>%d AND installation='') AS unbound,SUM(status='active' AND expires_at<=%d) AS expired,SUM(status='revoked') AS revoked FROM {$this->prefix}licenses GROUP BY edition", $now, $now, $now), ARRAY_A);
        if ($this->db->last_error !== '') { throw new \RuntimeException('Licensing storage unavailable'); }
        $result = [];
        foreach (['community', 'professional', 'enterprise'] as $edition) { $result[$edition] = array_fill_keys(['total','active','unbound','expired','revoked'], 0); }
        foreach ($rows ?: [] as $row) {
            if (!isset($result[$row['edition']])) { continue; }
            foreach ($result[$row['edition']] as $key => $_) { $result[$row['edition']][$key] = (int) $row[$key]; }
        }
        return $result;
    }
}
