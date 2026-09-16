<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

/** Self-contained email: no remote images, scripts, fonts or tracking pixels. */
function verificationEmail(string $url, bool $recover = false): array {
    $link = htmlspecialchars($url, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
    $subject = 'Verify your email — DarkPhish Community';
    $text = "DARKPHISH COMMUNITY\n\nVerify your email address\n\nYou're one step away from your Community activation key.\nOpen the link below within 30 minutes, then confirm on the page to display your key:\n\n"
        . $url . "\n\nYour key is displayed on the website after confirmation; it is not included in this email.\n\nIf you did not request a license, ignore this message. Do not forward this verification link.\n\nNeed help? license@darkphish.sk\nDarkPhish — Built for defenders.\n";
    $html = <<<HTML
<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Verify your email — DarkPhish</title></head>
<body style="margin:0;padding:0;background-color:#080d10;font-family:Arial,Helvetica,sans-serif;-webkit-text-size-adjust:100%;">
<div style="display:none;font-size:1px;color:#080d10;line-height:1px;max-height:0;max-width:0;opacity:0;overflow:hidden;mso-hide:all;">Your Community activation starts here. Verify your email within 30 minutes.</div>
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="width:100%;background-color:#080d10;"><tr><td align="center" style="padding:32px 12px;">
<!--[if mso]><table role="presentation" width="600" cellspacing="0" cellpadding="0" border="0"><tr><td><![endif]-->
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="width:100%;max-width:600px;">
<tr><td style="padding:8px 16px 28px;color:#f3f7f8;"><span style="font-size:26px;font-weight:800;letter-spacing:-1px;">Dark<span style="color:#1fd1dc;">Phish</span></span><br><span style="font-size:10px;letter-spacing:3px;color:#60efda;line-height:24px;">BUILT FOR DEFENDERS</span></td></tr>
<tr><td style="height:5px;background-color:#1fd1dc;background-image:linear-gradient(90deg,#1fd1dc,#60efda);font-size:1px;line-height:5px;">&nbsp;</td></tr>
<tr><td style="padding:32px 24px;background-color:#ffffff;border-radius:0 0 16px 16px;color:#17252b;">
<p style="margin:0 0 20px;font-size:11px;font-weight:bold;letter-spacing:2px;color:#087d85;">COMMUNITY LICENSE</p>
<h1 style="margin:0 0 20px;font-size:32px;line-height:1.15;letter-spacing:-1px;color:#101c22;">Your next step:<br>verify your email</h1>
<p style="margin:0 0 24px;font-size:16px;line-height:1.65;color:#4b5b64;">You're one step away from your DarkPhish Community activation key. Confirm that this email address belongs to you to continue.</p>
<table role="presentation" cellspacing="0" cellpadding="0" border="0" style="margin:0 0 24px;"><tr><td bgcolor="#1fd1dc" style="background-color:#1fd1dc;border-radius:8px;text-align:center;mso-padding-alt:16px 24px;"><a href="{$link}" style="display:inline-block;padding:16px 24px;border:1px solid #1fd1dc;border-radius:8px;font-size:16px;font-weight:bold;line-height:20px;color:#04282c;text-decoration:none;">Verify email address &#8594;</a></td></tr></table>
<p style="margin:0 0 28px;font-size:13px;line-height:1.6;color:#52636b;"><strong style="color:#17252b;">Valid for 30 minutes.</strong> For your security, do not forward this link.</p>
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="width:100%;background-color:#eef8f7;border-radius:10px;"><tr><td style="padding:20px;">
<p style="margin:0 0 12px;font-size:13px;font-weight:bold;color:#153c40;">WHAT HAPPENS NEXT</p>
<p style="margin:0 0 8px;font-size:14px;line-height:1.6;color:#3b555c;">1. Open the verification page.</p>
<p style="margin:0 0 8px;font-size:14px;line-height:1.6;color:#3b555c;">2. Confirm to display your activation key.</p>
<p style="margin:0;font-size:14px;line-height:1.6;color:#3b555c;">3. Save the key and enter it in DarkPhish under Settings &rarr; Licensing.</p>
</td></tr></table>
<p style="margin:24px 0 8px;font-size:12px;line-height:1.6;color:#60717a;">Button not working? Copy this entire link into your browser:</p>
<p style="margin:0;font-size:12px;line-height:1.7;word-break:break-all;overflow-wrap:anywhere;"><a href="{$link}" style="color:#087d85;text-decoration:underline;word-break:break-all;">{$link}</a></p>
</td></tr>
<tr><td style="padding:24px 16px 8px;text-align:center;">
<p style="margin:0 0 10px;font-size:12px;line-height:1.7;color:#a5b6bf;">Didn't request a license? You can safely ignore this email.<br>Your activation key is displayed only after confirmation on the website.</p>
<p style="margin:0;font-size:12px;line-height:1.7;color:#a5b6bf;">Need help? <a href="mailto:license@darkphish.sk" style="color:#60efda;text-decoration:underline;">license@darkphish.sk</a></p>
</td></tr></table>
<!--[if mso]></td></tr></table><![endif]-->
</td></tr></table></body></html>
HTML;
    if ($recover) {
        $subject = 'Confirm your license recovery — DarkPhish';
        $html = str_replace(["Your Community activation starts here.", "You're one step away from your DarkPhish Community activation key. Confirm that this email address belongs to you to continue.", 'Your next step:<br>verify your email', '2. Confirm to display your activation key.', 'Your activation key is displayed only after confirmation on the website.'],
            ['Recover your existing Community license.', 'You requested a replacement activation key. Verify your email first. We will then check for an existing Community license. Recovery does not extend its expiry date.', 'Lost your key?<br>Recover your license', '2. Confirm recovery. Your original expiry date stays unchanged.', 'If an active license exists, its replacement key will be displayed and emailed after confirmation.'], $html);
        $text = "DARKPHISH COMMUNITY — LICENSE RECOVERY\n\nConfirm your request within 30 minutes:\n\n" . $url . "\n\nAfter you confirm, we check for an existing active Community license. Its replacement key is displayed and emailed; the original expiry date and installation remain unchanged.\n\nIf you did not request recovery, ignore this message. Do not forward this link.\nSupport: license@darkphish.sk\n";
    }
    return ['subject' => $subject, 'html' => $html, 'text' => $text];
}

function sendVerificationEmail(string $email, string $url, bool $recover = false): bool {
    return sendLicenseMessage($email, verificationEmail($url, $recover));
}

function sendLicenseMessage(string $email, array $message): bool {
    // Scope the alternative to this exact message, including nested wp_mail calls.
    $configure = static function ($mailer) use ($email, $message): void {
        $recipients = $mailer->getToAddresses();
        if ($mailer->Subject === $message['subject'] && $mailer->Body === $message['html'] &&
            count($recipients) === 1 && strcasecmp($recipients[0][0], $email) === 0) {
            $mailer->AltBody = $message['text'];
        }
    };
    add_action('phpmailer_init', $configure, PHP_INT_MAX);
    try {
        return wp_mail($email, $message['subject'], $message['html'], ['Content-Type: text/html; charset=UTF-8']);
    } finally {
        remove_action('phpmailer_init', $configure, PHP_INT_MAX);
    }
}

function recoveryKeyEmail(array $license): array {
    $key = htmlspecialchars($license['license_key'], ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
    $expiry = gmdate('Y-m-d H:i', (int) $license['expires_at']) . ' UTC';
    $subject = 'Your replacement Community key — DarkPhish';
    $text = "Your DarkPhish Community license has been recovered.\n\nReplacement activation key:\n" . $license['license_key'] . "\n\nOriginal expiry: " . $expiry . "\n\nThe previous activation key has been replaced. Your license limits, installation binding and expiry have not changed. Enter this key in DarkPhish > Settings > Licensing. Keep this email private.\n\nIf you did not confirm this recovery, contact license@darkphish.sk.\n";
    $html = <<<HTML
<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Your replacement DarkPhish key</title></head>
<body style="margin:0;background:#080d10;font-family:Arial,Helvetica,sans-serif;color:#17252b;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;">
<tr><td style="padding:12px 16px 28px;font-size:26px;font-weight:bold;color:#f3f7f8;">Dark<span style="color:#1fd1dc;">Phish</span></td></tr>
<tr><td style="padding:28px 24px;border-top:5px solid #1fd1dc;background:#ffffff;">
<p style="font-size:11px;letter-spacing:2px;font-weight:bold;color:#087d85;">COMMUNITY LICENSE RECOVERED</p>
<h1 style="font-size:28px;line-height:1.2;">Your replacement key</h1>
<p style="font-size:16px;line-height:1.7;">Your existing license is ready to use again. Its original expiry date has not changed.</p>
<p style="padding:18px;background:#eef8f7;font-family:monospace;font-size:15px;line-height:1.8;word-break:break-all;overflow-wrap:anywhere;">{$key}</p>
<p style="font-size:14px;line-height:1.7;"><strong>Valid until: {$expiry}</strong><br>Your license limits and installation binding remain unchanged. The previous activation key has been replaced.</p>
<p style="font-size:14px;line-height:1.7;">Enter this key in DarkPhish under <strong>Settings &rarr; Licensing</strong>. Keep this email private.</p>
</td></tr><tr><td style="padding:24px 16px;font-size:12px;line-height:1.7;color:#a5b6bf;">Didn't confirm this recovery? Contact <a href="mailto:license@darkphish.sk" style="color:#60efda;">license@darkphish.sk</a>.</td></tr>
</table></td></tr></table></body></html>
HTML;
    return ['subject' => $subject, 'html' => $html, 'text' => $text];
}

function sendRecoveryKeyEmail(string $email, array $license): bool {
    return sendLicenseMessage($email, recoveryKeyEmail($license));
}
