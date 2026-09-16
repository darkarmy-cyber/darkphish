<?php
declare(strict_types=1);
// Run inside the disposable WordPress integration. Never contact an SMTP server.
require_once ABSPATH . WPINC . '/PHPMailer/Exception.php';
require_once ABSPATH . WPINC . '/PHPMailer/PHPMailer.php';
require_once ABSPATH . WPINC . '/PHPMailer/SMTP.php';
require_once ABSPATH . WPINC . '/class-wp-phpmailer.php';
$previousMailer = $GLOBALS['phpmailer'] ?? null;
$capture = new class(true) extends WP_PHPMailer {
    public array $captured = [];
    public bool $fail = false;
    public function send() {
        if ($this->fail) { throw new \PHPMailer\PHPMailer\Exception('Simulated mail failure'); }
        if (!$this->preSend()) { return false; }
        $this->captured[] = ['html' => $this->Body, 'text' => $this->AltBody, 'mime' => $this->getSentMIMEMessage()];
        return true;
    }
};
$GLOBALS['phpmailer'] = $capture;
$allowCapture = static fn () => null;
add_filter('pre_wp_mail', $allowCapture, PHP_INT_MAX);
$url = 'https://darkphish.test/licencia/#dp-verify=' . str_repeat('m', 43);
$hookCount = static fn () => count($GLOBALS['wp_filter']['phpmailer_init']->callbacks[PHP_INT_MAX] ?? []);
$beforeHooks = $hookCount();
try {
    check(Darkphish\Licensing\sendVerificationEmail('mail-test@example.test', $url), 'HTML mail failed');
    $mail = $capture->captured[0];
    check(str_contains($mail['mime'], 'multipart/alternative'), 'Plain-text alternative missing');
    check(str_contains($mail['mime'], 'text/html') && str_contains($mail['mime'], 'text/plain'), 'MIME parts missing');
    check(str_contains($mail['html'], 'href="' . $url . '"') && str_contains($mail['text'], $url), 'Verification link changed');
    check(str_contains($mail['html'], 'Verify email address') && str_contains($mail['text'], '30 minutes'), 'Mail instructions missing');
    check(!preg_match('/<(img|script|iframe)\b/i', $mail['html']), 'Unexpected remote email resource');
    check($hookCount() === $beforeHooks, 'Mailer hook leaked after success');
    check(wp_mail('other@example.test', 'Unrelated WordPress message', 'Unrelated body'), 'Other mail failed');
    check($capture->captured[1]['text'] === '' && $capture->captured[1]['html'] === 'Unrelated body', 'Verification content leaked to other mail');
    $recovery = ['license_key' => 'DP-COM-' . str_repeat('r', 43), 'expires_at' => 1800000000];
    check(Darkphish\Licensing\sendRecoveryKeyEmail('mail-test@example.test', $recovery), 'Recovery key email failed');
    check(str_contains($capture->captured[2]['text'], $recovery['license_key']) && str_contains($capture->captured[2]['html'], '2027-01-15'), 'Recovery email key/expiry missing');
    check(str_contains($capture->captured[2]['mime'], 'multipart/alternative'), 'Recovery text alternative missing');
    $capture->fail = true;
    check(!Darkphish\Licensing\sendVerificationEmail('mail-test@example.test', $url), 'Mail failure ignored');
    check($hookCount() === $beforeHooks, 'Mailer hook leaked after failure');
    $unsafe = Darkphish\Licensing\verificationEmail('https://example.test/"<script>alert(1)</script>');
    check(!str_contains($unsafe['html'], '<script>') && str_contains($unsafe['html'], '&quot;&lt;script&gt;'), 'URL was not escaped');
} finally {
    remove_filter('pre_wp_mail', $allowCapture, PHP_INT_MAX);
    $GLOBALS['phpmailer'] = $previousMailer;
}
echo "HTML email passed: multipart content, exact fragment link, escaping, hook cleanup and unrelated mail isolation.\n";
