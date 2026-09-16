/* No activation credentials or email addresses are stored in browser storage. */
document.addEventListener('DOMContentLoaded', function () {
    'use strict';
    const root = document.getElementById('darkphish-license');
    if (!root) return;
    const status = root.querySelector('.dp-status');
    const form = root.querySelector('.dp-request');
    const verify = root.querySelector('.dp-verify');
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
            const result = await send('request', {email: values.get('email'), terms_version: root.dataset.terms, challenge_token: values.get('cf-turnstile-response') || ''});
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
            const key = root.querySelector('.dp-key');
            key.textContent = result.license_key;
            key.hidden = false;
            verify.hidden = true;
            status.textContent = 'Save this key securely and enter it in Darkphish → Settings → Licensing. It is displayed only once. Request a new verification link if you lose it.';
        } catch (error) { status.textContent = error.message; verify.disabled = false; }
    });
});
