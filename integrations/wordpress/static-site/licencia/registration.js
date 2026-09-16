/* Public settings only. Credentials stay in memory, never browser storage. */
document.addEventListener('DOMContentLoaded', async function () {
    'use strict';
    const root = document.getElementById('darkphish-license');
    const status = root.querySelector('.dp-status');
    const form = root.querySelector('.dp-request');
    const submit = form.querySelector('button');
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
            if (response.status === 429) throw new Error('Príliš veľa pokusov. Skúste to neskôr.');
            if (operation === 'verify' && response.status === 403) throw new Error('Odkaz vypršal alebo už bol použitý. Požiadajte o nový e-mail.');
            throw new Error('Registráciu sa nepodarilo dokončiť. Skúste to neskôr alebo obnovte stránku.');
        }
        return response.json();
    }
    verify.addEventListener('click', async function () {
        verify.disabled = true;
        try {
            const result = await send('verify', {token});
            if (typeof result.license_key !== 'string' || !/^DP-COM-[A-Za-z0-9_-]{43}$/.test(result.license_key)) throw new Error('Server nevrátil platný aktivačný kľúč.');
            token = '';
            const key = root.querySelector('.dp-key');
            key.textContent = result.license_key;
            key.hidden = false;
            verify.hidden = true;
            status.textContent = 'Kľúč si bezpečne uložte a vložte do DarkPhish → Settings → Licensing. Zobrazí sa iba raz.';
        } catch (error) { status.textContent = error.message; verify.disabled = false; restart.hidden = false; }
    });
    if (token) {
        if (!/^[A-Za-z0-9_-]{43}$/.test(token)) {
            token = ''; status.textContent = 'Overovací odkaz je neplatný.'; restart.hidden = false; return;
        }
        verify.hidden = false;
        status.textContent = 'Potvrďte e-mail a zobrazte svoj aktivačný kľúč. Odkaz platí 30 minút.';
        return;
    }
    form.addEventListener('submit', async function (event) {
        event.preventDefault();
        if (!form.reportValidity() || !challengeToken) return;
        submit.disabled = true;
        try {
            await send('request', {email: new FormData(form).get('email'), terms_version: termsVersion, challenge_token: challengeToken});
            status.textContent = 'Ak je možné žiadosť spracovať, pošleme vám overovací e-mail. Skontrolujte aj priečinok spam.';
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
            typeof settings.terms_version !== 'string' || !settings.terms_version) throw new Error('Registrácia ešte nie je správne nakonfigurovaná.');
        termsVersion = settings.terms_version;
        root.querySelector('.dp-terms').href = terms.href;
        form.hidden = false;
        status.textContent = 'Potvrdenie pošleme na vašu e-mailovú adresu.';
        const script = document.createElement('script');
        script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';
        script.async = true;
        script.onload = function () {
            widget = window.turnstile.render(root.querySelector('.dp-challenge'), {
                sitekey: settings.site_key, action: 'darkphish-license',
                callback: function (value) { challengeToken = value; submit.disabled = false; },
                'expired-callback': function () { challengeToken = ''; submit.disabled = true; },
                'error-callback': function () { challengeToken = ''; submit.disabled = true; status.textContent = 'Ochranu formulára sa nepodarilo načítať. Obnovte stránku.'; }
            });
        };
        script.onerror = function () { status.textContent = 'Ochranu formulára sa nepodarilo načítať. Obnovte stránku.'; };
        document.head.appendChild(script);
    } catch (error) { status.textContent = 'Registrácia momentálne nie je dostupná. Skúste to neskôr.'; form.hidden = true; }
});
