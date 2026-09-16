<?php
declare(strict_types=1);
// Real disposable MySQL: multiple batches, bounded continuation, active rows retained.
$insertCleanupRows = function (string $table, int $count, int $expires, string $label) use ($db, $wpdb): void {
    for ($start = 0; $start < $count; $start += 500) {
        $rows = [];
        for ($i = $start; $i < min($count, $start + 500); $i++) {
            $hash = hash('sha256', "cleanup:$table:$label:$i");
            $rows[] = $table === 'requests'
                ? $wpdb->prepare('(%s,%s,%d,%s)', $hash, 'cleanup@example.test', $expires, 'cleanup-test')
                : $wpdb->prepare('(%s,1,%d)', $hash, $expires);
        }
        $columns = $table === 'requests' ? '(token_hash,email,expires_at,terms_version)' : '(bucket,hits,expires_at)';
        $db->execute("INSERT INTO {$db->prefix}$table $columns VALUES " . implode(',', $rows));
    }
};
foreach (['requests', 'limits'] as $table) {
    $insertCleanupRows($table, 2501, 1, 'expired');
    $insertCleanupRows($table, 1, time() + 3600, 'active');
}
check($db->cleanup() === false, 'Small multi-batch backlog was not fully drained');
foreach (['requests', 'limits'] as $table) {
    check((int) $wpdb->get_var("SELECT COUNT(*) FROM {$db->prefix}$table WHERE expires_at=1") === 0, 'Expired batches were retained');
    $column = $table === 'requests' ? 'token_hash' : 'bucket';
    check((int) $wpdb->get_var($wpdb->prepare("SELECT COUNT(*) FROM {$db->prefix}$table WHERE $column=%s", hash('sha256', "cleanup:$table:active:0"))) === 1, 'Active cleanup row removed');
}
$insertCleanupRows('requests', 10001, 1, 'large');
wp_clear_scheduled_hook('darkphish_license_cleanup_backlog');
Darkphish\Licensing\cleanupStorage();
check((int) $wpdb->get_var("SELECT COUNT(*) FROM {$db->prefix}requests WHERE expires_at=1") === 1, 'Cleanup exceeded its bounded budget');
$continuation = wp_next_scheduled('darkphish_license_cleanup_backlog');
check(is_int($continuation) && $continuation >= time() && $continuation <= time() + 65, 'Bounded backlog did not schedule prompt continuation');
do_action('darkphish_license_cleanup_backlog');
check((int) $wpdb->get_var("SELECT COUNT(*) FROM {$db->prefix}requests WHERE expires_at=1") === 0, 'Continuation did not drain remaining backlog');
wp_clear_scheduled_hook('darkphish_license_cleanup_backlog');
echo "Cleanup regression passed: multiple batches, bounded work, continuation and unexpired records preserved.\n";
