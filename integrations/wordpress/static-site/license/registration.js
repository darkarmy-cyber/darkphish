/* Public settings only. Credentials stay in memory, never browser storage. */
document.addEventListener('DOMContentLoaded', async function () {
    'use strict';
    const root = document.getElementById('darkphish-license');
    const status = root.querySelector('.dp-status');
    const form = root.querySelector('.dp-request');
    const submit = form.querySelector('button');
    const fields = form.querySelector('fieldset');
    const verify = root.querySelector('.dp-verify');
    const restart = root.querySelector('.dp-restart');
    const confirmation = root.querySelector('.dp-confirmation');
    const confirmationTitle = root.querySelector('.dp-confirmation-title');
    const recovering = new URLSearchParams(location.search).get('mode') === 'recover';
    if (recovering) {
        root.querySelector('.dp-intro-title').textContent = 'Recover your';
        root.querySelector('.lead').textContent = 'Get a replacement key for your existing Community license. Its original limits and expiry stay unchanged.';
        root.querySelector('.features').hidden = true;
        root.querySelector('.dp-mode-label').textContent = 'LICENSE RECOVERY';
        root.querySelector('.dp-form-title').textContent = 'Recover your Community license';
        root.querySelector('.dp-terms-row').hidden = true;
        root.querySelector('.dp-terms-checkbox').disabled = true;
        root.querySelector('.hint').textContent = 'After email verification, we check for an existing license. Its original expiry date and limits stay unchanged.';
        root.querySelector('.dp-mode-link').href = './';
        root.querySelector('.dp-mode-link').textContent = 'Need your first license? Register →';
        root.querySelector('.dp-confirmation-next').textContent = 'Open the recovery email and verify your address. After you confirm, we will check for an existing active license and display and email its replacement key.';
    }
    let requesting = false, accepted = false;
    function showStatus(message, state = '', focus = false) {
        status.textContent = message;
        status.dataset.state = state;
        if (focus) status.focus();
    }
    let token = new URLSearchParams(location.hash.slice(1)).get('dp-verify');
    // Remove the fragment before any external script loads. GET never redeems it.
    if (location.hash) history.replaceState(null, '', location.pathname + location.search);
    const api = root.dataset.api;
    let termsVersion = '', challengeToken = '', widget;
    async function send(operation, data) {
        const response = await fetch(api + operation, {
            method: data ? 'POST' : 'GET', credentials: 'omit', redirect: 'error', cache: 'no-store', referrerPolicy: 'no-referrer',
            ...(data ? {headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data)} : {})
        });
        if (!response.ok) {
            if (operation === 'public-config' && response.status === 503) {
                const error = new Error('Online registration is not open yet. For help, email license@darkphish.sk.');
                error.registrationNotReady = true;
                throw error;
            }
            if (response.status === 429) throw new Error('Too many attempts. Please try again later.');
            if (['request', 'recover'].includes(operation) && response.status === 503) throw new Error('We could not send your verification email. Please try again later.');
            if (['request', 'recover'].includes(operation) && response.status === 400) throw new Error('Please reload the page, check your email and complete the required fields and security check again.');
            if (operation === 'verify' && response.status === 403) throw new Error('This link has expired or has already been used. Request a new verification email.');
            throw new Error('Registration could not be completed. Try again later or reload the page.');
        }
        return response.json();
    }
    verify.addEventListener('click', async function () {
        verify.disabled = true;
        try {
            const result = await send('verify', {token});
            const outcomes = {
                already_registered: 'This email already has a Community license. Use license recovery to replace a lost key. Registration does not renew your license.',
                not_found: 'No Community license was found for this verified email address. You can register for your first license.',
                expired: 'Your Community license has expired. Recovery cannot renew it. Contact license@darkphish.sk.',
                revoked: 'This license is not available for recovery. Contact license@darkphish.sk.',
                support_required: 'Your license records need review. Contact license@darkphish.sk. No license has been changed.'
            };
            if (outcomes[result.outcome]) {
                token = ''; verify.hidden = true;
                showStatus(outcomes[result.outcome], 'notice', true);
                restart.hidden = false;
                restart.href = result.outcome === 'already_registered' ? './?mode=recover' : './';
                restart.textContent = result.outcome === 'already_registered' ? 'Recover your existing license →' : 'Back to registration';
                return;
            }
            if (typeof result.license_key !== 'string' || !/^DP-COM-[A-Za-z0-9_-]{43}$/.test(result.license_key)) throw new Error('The server did not return a valid activation key.');
            token = '';
            const key = root.querySelector('.dp-key');
            key.textContent = result.license_key;
            key.hidden = false;
            verify.hidden = true;
            status.textContent = 'Save this key securely and enter it in DarkPhish → Settings → Licensing. It is displayed only once.';
            if (result.outcome === 'recovered') {
                const expiry = new Date(result.expires_at * 1000).toISOString().slice(0, 10);
                status.textContent = 'Your replacement key is ready. Original expiry: ' + expiry + ' (UTC). Limits and installation are unchanged. ' +
                    (result.email_accepted ? 'A copy has been submitted for email delivery.' : 'The email copy could not be sent. Save the key below securely.');
            }
        } catch (error) { status.textContent = error.message; verify.disabled = false; restart.hidden = false; }
    });
    if (token) {
        form.hidden = true;
        if (!/^[A-Za-z0-9_-]{43}$/.test(token)) {
            token = ''; status.textContent = 'This verification link is invalid.'; restart.hidden = false; return;
        }
        verify.hidden = false;
        status.textContent = recovering ? 'Confirm your email to check for an existing Community license and recover its key. Recovery does not extend its expiry.' : 'Verify your email to complete registration. The link expires after 30 minutes.';
        return;
    }
    form.addEventListener('submit', async function (event) {
        event.preventDefault();
        if (requesting || accepted || fields.disabled || !form.reportValidity() || !challengeToken) return;
        const email = new FormData(form).get('email');
        requesting = true;
        submit.disabled = true;
        submit.textContent = 'Sending request…';
        showStatus('Sending your request. Please wait…', 'pending');
        try {
            await send(recovering ? 'recover' : 'request', {email, ...(!recovering ? {terms_version: termsVersion} : {}), challenge_token: challengeToken});
            accepted = true;
            form.hidden = true;
            status.hidden = true;
            root.querySelector('.dp-confirmation-email').textContent = email;
            confirmation.hidden = false;
            confirmationTitle.focus();
            form.reset();
        } catch (error) {
            showStatus(error instanceof TypeError ? 'We could not confirm your request. Check your connection and inbox before trying again.' : error.message, 'error', true);
        } finally {
            requesting = false; challengeToken = ''; submit.disabled = true;
            submit.textContent = 'Send verification email →';
            if (!accepted && window.turnstile) window.turnstile.reset(widget);
        }
    });
    try {
        const settings = await send('public-config');
        const terms = new URL(settings.terms_url);
        const registration = new URL(settings.registration_url);
        if (terms.protocol !== 'https:' || terms.username || terms.password ||
            registration.origin !== location.origin ||
            typeof settings.site_key !== 'string' || !settings.site_key ||
            typeof settings.terms_version !== 'string' || !settings.terms_version) throw new Error('Registration has not been configured correctly yet.');
        termsVersion = settings.terms_version;
        root.querySelector('.dp-terms').href = terms.href;
        fields.disabled = false;
        status.textContent = 'We will send a verification link to your email address.';
        const script = document.createElement('script');
        script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';
        script.async = true;
        script.onload = function () {
            widget = window.turnstile.render(root.querySelector('.dp-challenge'), {
                sitekey: settings.site_key, action: 'darkphish-license', language: 'en', size: 'compact',
                callback: function (value) { challengeToken = value; submit.disabled = requesting || accepted; },
                'expired-callback': function () { challengeToken = ''; submit.disabled = true; },
                'error-callback': function () { challengeToken = ''; submit.disabled = true; status.textContent = 'The anti-abuse check could not be loaded. Please reload the page.'; }
            });
        };
        script.onerror = function () { status.textContent = 'The anti-abuse check could not be loaded. Please reload the page.'; };
        document.head.appendChild(script);
    } catch (error) { status.textContent = error.registrationNotReady ? error.message : 'We could not load registration. Please reload the page or contact license@darkphish.sk.'; fields.disabled = true; submit.disabled = true; }
});
