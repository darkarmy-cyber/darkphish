<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

/** Self-contained email: no remote images, scripts, fonts or tracking pixels. */
function verificationEmail(string $url): array {
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
    return ['subject' => $subject, 'html' => $html, 'text' => $text];
}

function sendVerificationEmail(string $email, string $url): bool {
    $message = verificationEmail($url);
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
