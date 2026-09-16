/* No activation credentials or email addresses are stored in browser storage. */
document.addEventListener('DOMContentLoaded', function () {
    'use strict';
    const root = document.getElementById('darkphish-license');
    if (!root) return;
    const status = root.querySelector('.dp-status');
    const form = root.querySelector('.dp-request');
    const verify = root.querySelector('.dp-verify');
    const recovering = new URLSearchParams(location.search).get('mode') === 'recover';
    const recoveryLink = root.querySelector('.dp-recovery-link');
    if (recovering) {
        form.querySelector('input[type=checkbox]').disabled = true;
        form.querySelector('.dp-terms-row').hidden = true;
        form.querySelector('button').textContent = 'Recover license';
        recoveryLink.href = location.pathname;
        recoveryLink.textContent = 'Register a new license';
        status.textContent = 'Verify your email to recover your existing Community license. Its expiry stays unchanged.';
    }
    const fragment = new URLSearchParams(location.hash.slice(1));
    let token = fragment.get('dp-verify');
    if (token) {
        history.replaceState(null, '', location.pathname + location.search);
        form.hidden = true;
        verify.hidden = false;
        status.textContent = 'Confirm your email to display your license key. The link expires after 30 minutes.';
    }
    async function send(operation, data) {
        const response = await fetch(root.dataset.api + operation, {
            method: 'POST', credentials: 'omit', redirect: 'error', cache: 'no-store',
            headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data)
        });
        const result = await response.json();
        if (!response.ok) throw new Error(result.message || 'Service unavailable. Please try again later.');
        return result;
    }
    form.addEventListener('submit', async function (event) {
        event.preventDefault();
        const button = form.querySelector('button');
        button.disabled = true;
        try {
            const values = new FormData(form);
            const result = await send(recovering ? 'recover' : 'request', {email: values.get('email'), ...(!recovering ? {terms_version: root.dataset.terms} : {}), challenge_token: values.get('cf-turnstile-response') || ''});
            status.textContent = result.message;
            form.reset();
        } catch (error) { status.textContent = error.message; }
        finally { button.disabled = false; if (window.turnstile) window.turnstile.reset(); }
    });
    verify.addEventListener('click', async function () {
        verify.disabled = true;
        try {
            const result = await send('verify', {token: token});
            token = '';
            if (!result.license_key) {
                const messages = {email_domain_blocked: 'Please use your personal or business email address. Temporary email addresses are not accepted.', already_registered: 'This email already has a Community license. Use Lost your license below to recover it.', not_found: 'No Community license exists for this verified email. Register first.', expired: 'Your license has expired. Contact license@darkphish.sk to request renewal.', revoked: 'This license is not available for recovery. Contact license@darkphish.sk.', support_required: 'Your license records need review. Contact license@darkphish.sk.'};
                status.textContent = messages[result.outcome] || 'Verification could not be completed.';
                verify.hidden = true; return;
            }
            const key = root.querySelector('.dp-key');
            key.textContent = result.license_key;
            key.hidden = false;
            verify.hidden = true;
            status.textContent = 'Save this key securely and enter it in Darkphish → Settings → Licensing. It is displayed only once. Request a new verification link if you lose it.';
            if (result.outcome === 'recovered') status.textContent = 'Recovered. Original expiry: ' + new Date(result.expires_at * 1000).toISOString().slice(0, 10) + ' UTC. Limits and installation are unchanged. ' + (result.email_accepted ? 'A copy has been submitted for email delivery.' : 'The email copy could not be sent. Save this key securely.');
        } catch (error) { status.textContent = error.message; verify.disabled = false; }
    });
});
