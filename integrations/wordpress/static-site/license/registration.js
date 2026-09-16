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
            if (response.status === 429) throw new Error('Too many attempts. Please try again later.');
            if (operation === 'verify' && response.status === 403) throw new Error('This link has expired or has already been used. Request a new verification email.');
            throw new Error('Registration could not be completed. Try again later or reload the page.');
        }
        return response.json();
    }
    verify.addEventListener('click', async function () {
        verify.disabled = true;
        try {
            const result = await send('verify', {token});
            if (typeof result.license_key !== 'string' || !/^DP-COM-[A-Za-z0-9_-]{43}$/.test(result.license_key)) throw new Error('The server did not return a valid activation key.');
            token = '';
            const key = root.querySelector('.dp-key');
            key.textContent = result.license_key;
            key.hidden = false;
            verify.hidden = true;
            status.textContent = 'Save this key securely and enter it in DarkPhish → Settings → Licensing. It is displayed only once.';
        } catch (error) { status.textContent = error.message; verify.disabled = false; restart.hidden = false; }
    });
    if (token) {
        form.hidden = true;
        if (!/^[A-Za-z0-9_-]{43}$/.test(token)) {
            token = ''; status.textContent = 'This verification link is invalid.'; restart.hidden = false; return;
        }
        verify.hidden = false;
        status.textContent = 'Verify your email to display your activation key. The link expires after 30 minutes.';
        return;
    }
    form.addEventListener('submit', async function (event) {
        event.preventDefault();
        if (fields.disabled || !form.reportValidity() || !challengeToken) return;
        submit.disabled = true;
        try {
            await send('request', {email: new FormData(form).get('email'), terms_version: termsVersion, challenge_token: challengeToken});
            status.textContent = 'If your request can be processed, a verification email will arrive shortly. Please check your spam folder too.';
            form.reset();
        } catch (error) { status.textContent = error.message; }
        finally { challengeToken = ''; if (window.turnstile) window.turnstile.reset(widget); }
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
                sitekey: settings.site_key, action: 'darkphish-license', language: 'en',
                callback: function (value) { challengeToken = value; submit.disabled = false; },
                'expired-callback': function () { challengeToken = ''; submit.disabled = true; },
                'error-callback': function () { challengeToken = ''; submit.disabled = true; status.textContent = 'The anti-abuse check could not be loaded. Please reload the page.'; }
            });
        };
        script.onerror = function () { status.textContent = 'The anti-abuse check could not be loaded. Please reload the page.'; };
        document.head.appendChild(script);
    } catch (error) { status.textContent = 'Registration is currently unavailable. Please try again later.'; fields.disabled = true; submit.disabled = true; }
});
