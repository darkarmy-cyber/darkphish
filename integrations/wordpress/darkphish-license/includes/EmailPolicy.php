<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

final class EmailDomainBlocked extends \DomainException {}

final class EmailPolicy {
    public const MESSAGE = 'Please use your personal or business email address. Temporary email addresses are not accepted.';
    // Small, editable starter list; attribution and scope are documented in INSTALL.md.
    public static function defaults(): array {
        return ['enabled' => true, 'domains' => implode("\n", [
            '10minutemail.com', '10minutemail.net', '10minemail.com',
            '1secmail.com', '1secmail.net', '1secmail.org',
            'guerrillamail.com', 'guerrillamail.net', 'guerrillamail.org',
            'guerrillamail.biz', 'guerrillamail.de', 'guerrillamail.info',
            'guerrillamailblock.com', 'mailinator.com',
            'temp-mail.org', 'temp-mail.com', 'temp-mail.lol', 'tempail.com',
            'yopmail.com', 'yopmail.fr', 'yopmail.net',
        ])];
    }

    public static function settings(): array {
        // An intentionally empty saved list must never be replaced by the defaults.
        return get_option('darkphish_email_policy', self::defaults());
    }

    public static function sanitize(mixed $input): array {
        if (!is_array($input)) { return self::invalid(); }
        $domains = $input['domains'] ?? null;
        $enabled = $input['enabled'] ?? false;
        if (!is_string($domains) || strlen($domains) > 65536 ||
            !in_array($enabled, [true, false, '1', '0'], true)) { return self::invalid(); }
        $result = [];
        foreach (preg_split('/\r\n|\r|\n/', $domains) as $line) {
            $domain = strtolower(trim($line));
            if ($domain === '') { continue; }
            // Only explicit ASCII/punycode domains, never URLs, mailboxes or wildcard patterns.
            if (strlen($domain) > 253 || filter_var($domain, FILTER_VALIDATE_IP) ||
                !preg_match('/^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$/D', $domain)) {
                return self::invalid();
            }
            $result[$domain] = true;
            if (count($result) > 1000) { return self::invalid(); }
        }
        return ['enabled' => $enabled === true || $enabled === '1', 'domains' => implode("\n", array_keys($result))];
    }

    private static function invalid(): array {
        add_settings_error('darkphish_email_policy', 'invalid_domains', 'Email policy was not saved. Enter one domain per line (for example temp-mail.org), without https://, @ or wildcards. Maximum: 1,000 domains / 65,536 characters. Use punycode for international domains.');
        return self::settings();
    }

    public static function blocked(string $email): bool {
        $settings = self::settings();
        if (empty($settings['enabled'])) { return false; }
        $email = strtolower(trim($email));
        $at = strrpos($email, '@');
        if ($at === false) { return false; }
        $domain = substr($email, $at + 1);
        foreach (explode("\n", $settings['domains']) as $blocked) {
            if ($blocked !== '' && ($domain === $blocked || str_ends_with($domain, '.' . $blocked))) { return true; }
        }
        return false;
    }
}

add_action('admin_init', function (): void {
    register_setting('darkphish_email_policy', 'darkphish_email_policy', ['sanitize_callback' => [EmailPolicy::class, 'sanitize']]);
});

function emailPolicySettings(): void {
    $settings = EmailPolicy::settings();
    echo '<h2>Temporary email protection</h2><p>Applies to public Community registration and recovery, including pending verification links. Existing licenses remain valid; manual administrator issuance is unchanged.</p>';
    settings_errors('darkphish_email_policy');
    echo '<form method="post" action="options.php">';
    settings_fields('darkphish_email_policy');
    echo '<p><label><input type="checkbox" name="darkphish_email_policy[enabled]" value="1"' . checked(!empty($settings['enabled']), true, false) . '> Block temporary email domains</label></p>';
    echo '<p><label for="dp-email-domains">Blocked email domains</label></p><textarea id="dp-email-domains" class="large-text code" rows="12" maxlength="65536" name="darkphish_email_policy[domains]" aria-describedby="dp-email-help">' . esc_textarea($settings['domains']) . '</textarea>';
    echo '<p id="dp-email-help" class="description">One domain per line, for example temp-mail.org. Enter the domain after @ in the email address, which may differ from the service website. Subdomains are also blocked. Do not enter public suffixes such as co.uk. Remove any entries you want to allow. The starter list is not exhaustive and is not automatically refreshed; saving an empty list allows all domains. Checks run locally without sending addresses to another service.</p>';
    submit_button('Save email protection');
    echo '</form>';
}
