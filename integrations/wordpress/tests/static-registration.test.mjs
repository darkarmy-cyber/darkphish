import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
const source = readFileSync(new URL('../static-site/license/registration.js', import.meta.url), 'utf8');
test('shortcode strips the bearer fragment synchronously before DOM readiness', () => {
    const shortcode = readFileSync(new URL('../darkphish-license/assets/registration.js', import.meta.url), 'utf8');
    for (const hash of ['#dp-verify=mailbox-secret', '#dp-verify=']) {
        const steps = [];
        vm.runInNewContext(shortcode, {
            URLSearchParams, location: {hash, pathname: '/registration/', search: '?mode=recover'},
            history: {replaceState: (...args) => steps.push(['remove', ...args])},
            document: {addEventListener: name => steps.push(['wait', name])},
        });
        assert.deepEqual(steps, [['remove', null, '', '/registration/?mode=recover'], ['wait', 'DOMContentLoaded']]);
        assert.doesNotMatch(steps[0][3], /mailbox-secret|dp-verify/);
    }
});
function element(hidden = false) {
    return {hidden, disabled: false, textContent: '', dataset: {}, focused: false, focus() { this.focused = true; }, handlers: {}, addEventListener(name, fn) { this.handlers[name] = fn; }};
}
async function page(hash = '', reply = async () => ({ok: true, json: async () => ({site_key: 'public', terms_url: 'https://www.darkphish.test/terms/', terms_version: 'v1', registration_url: 'https://www.darkphish.test/license/'})}), search = '') {
    const nodes = Object.fromEntries(['.dp-status', '.dp-request', '.dp-verify', '.dp-restart', '.dp-key', '.dp-terms', '.dp-challenge', '.dp-confirmation', '.dp-confirmation-title', '.dp-confirmation-email', '.dp-mode-label', '.dp-form-title', '.dp-terms-row', '.dp-terms-checkbox', '.hint', '.dp-mode-link', '.dp-confirmation-next', '.dp-intro-title', '.lead', '.features', '.confirmation-again'].map(name => [name, element(name !== '.dp-status')]));
    const submit = element(); submit.disabled = true;
    const fields = element(); fields.disabled = true;
    nodes['.dp-request'].hidden = false;
    nodes['.dp-request'].emailValue = 'test@example.test';
    nodes['.dp-request'].querySelector = name => name === 'fieldset' ? fields : submit;
    nodes['.dp-request'].reportValidity = () => true;
    nodes['.dp-request'].reset = () => {};
    const requests = [], scripts = [], replaced = [];
    let ready, challenge;
    const context = {
        URL, URLSearchParams, Error, TypeError,
        location: {hash, origin: 'https://www.darkphish.test', pathname: '/license/', search},
        history: {replaceState: (...args) => replaced.push(args)},
        document: {getElementById: () => ({dataset: {api: 'https://fsociety.test/wp-json/darkphish-license/v1/'}, querySelector: name => nodes[name]}),
            addEventListener: (_, fn) => { ready = fn; }, createElement: () => ({}), head: {appendChild: script => scripts.push(script)}},
        fetch: async (url, options) => { requests.push({url, options}); return reply(url, options); },
        FormData: class { get() { return nodes['.dp-request'].emailValue; } },
        window: {turnstile: {render: (_, options) => { challenge = options; return 'widget'; }, reset: () => {}}}
    };
    vm.runInNewContext(source, context); await ready();
    return {nodes, requests, scripts, replaced, submit, fields, challenge: () => challenge};
}
test('verification removes the fragment before external work and requires an explicit click', async () => {
    const token = 'a'.repeat(43);
    const p = await page('#dp-verify=' + token, async () => ({ok: true, json: async () => ({license_key: 'DP-COM-' + 'b'.repeat(43)})}));
    assert.equal(p.replaced[0][2], '/license/');
    assert.equal(p.requests.length, 0); assert.equal(p.scripts.length, 0);
    assert.equal(p.nodes['.dp-verify'].hidden, false);
    assert.equal(p.nodes['.dp-request'].hidden, true);
    await p.nodes['.dp-verify'].handlers.click();
    assert.equal(p.requests.length, 1);
    assert.equal(JSON.parse(p.requests[0].options.body).token, token);
    assert.equal(p.requests[0].options.credentials, 'omit');
    assert.equal(p.requests[0].options.referrerPolicy, 'no-referrer');
    assert.equal(p.requests[0].options.redirect, 'error');
    assert.equal(p.nodes['.dp-key'].textContent, 'DP-COM-' + 'b'.repeat(43));
    assert.equal(p.nodes['.dp-verify'].hidden, true);
});
test('registration uses current server settings and submits only after a challenge', async () => {
    const p = await page();
    assert.equal(p.requests[0].options.method, 'GET');
    assert.equal(p.nodes['.dp-request'].hidden, false);
    assert.equal(p.nodes['.dp-terms'].href, 'https://www.darkphish.test/terms/');
    await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
    assert.equal(p.requests.length, 1);
    p.scripts[0].onload();
    assert.equal(p.challenge().size, 'compact');
    p.challenge().callback('valid-token');
    assert.equal(p.submit.disabled, false);
    await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
    assert.deepEqual(JSON.parse(p.requests[1].options.body), {email: 'test@example.test', terms_version: 'v1', challenge_token: 'valid-token'});
    assert.equal(p.submit.disabled, true);
    assert.equal(p.nodes['.dp-confirmation'].hidden, false);
    assert.equal(p.nodes['.dp-request'].hidden, true);
    assert.equal(p.nodes['.dp-confirmation-title'].focused, true);
    assert.equal(p.nodes['.dp-confirmation-email'].textContent, 'test@example.test');
    await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
    assert.equal(p.requests.length, 2);
});
test('configuration failures keep the form visible but prevent submission', async () => {
    const p = await page('', async () => ({ok: false, status: 503}));
    assert.equal(p.nodes['.dp-request'].hidden, false); assert.equal(p.scripts.length, 0);
    assert.equal(p.fields.disabled, true); assert.equal(p.submit.disabled, true);
    assert.match(p.nodes['.dp-status'].textContent, /not open yet/);
    await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
    assert.equal(p.requests.length, 1);
});
test('expired verification offers recovery without rendering untrusted content', async () => {
    const p = await page('#dp-verify=' + 'a'.repeat(43), async () => ({ok: false, status: 403}));
    await p.nodes['.dp-verify'].handlers.click();
    assert.equal(p.nodes['.dp-restart'].hidden, false); assert.equal(p.nodes['.dp-key'].hidden, true);
});
test('malformed verification tokens never reach the API', async () => {
    const p = await page('#dp-verify=bad');
    assert.equal(p.requests.length, 0); assert.equal(p.nodes['.dp-restart'].hidden, false);
});

test('request failures are visible, focused and never show a success confirmation', async () => {
    for (const response of [{ok: false, status: 503}, {ok: false, status: 429}, new TypeError('Network failure')]) {
        // Keep initial public configuration valid, fail only the subsequent POST.
        const failed = await page('', async (url) => {
            if (url.endsWith('public-config')) return {ok: true, json: async () => ({site_key: 'public', terms_url: 'https://www.darkphish.test/terms/', terms_version: 'v1', registration_url: 'https://www.darkphish.test/license/'})};
            if (response instanceof Error) throw response;
            return response;
        });
        failed.scripts[0].onload(); failed.challenge().callback('valid-token');
        await failed.nodes['.dp-request'].handlers.submit({preventDefault() {}});
        assert.equal(failed.nodes['.dp-confirmation'].hidden, true);
        assert.equal(failed.nodes['.dp-request'].hidden, false);
        assert.equal(failed.nodes['.dp-status'].dataset.state, 'error');
        assert.equal(failed.nodes['.dp-status'].focused, true);
        assert.match(failed.nodes['.dp-status'].textContent, /could not|Too many/);
        assert.equal(failed.submit.disabled, true);
    }
});

test('a pending request cannot be submitted twice even if the challenge refreshes', async () => {
    let complete;
    const p = await page('', async url => {
        if (url.endsWith('public-config')) return {ok: true, json: async () => ({site_key: 'public', terms_url: 'https://www.darkphish.test/terms/', terms_version: 'v1', registration_url: 'https://www.darkphish.test/license/'})};
        return new Promise(resolve => { complete = () => resolve({ok: true, json: async () => ({})}); });
    });
    p.scripts[0].onload(); p.challenge().callback('valid-token');
    const pending = p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
    assert.equal(p.nodes['.dp-status'].dataset.state, 'pending');
    p.challenge().callback('refreshed-token');
    assert.equal(p.submit.disabled, true);
    await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
    assert.equal(p.requests.length, 2);
    p.nodes['.dp-request'].emailValue = 'edited-while-sending@example.test';
    complete(); await pending;
    assert.equal(p.nodes['.dp-confirmation'].hidden, false);
    assert.equal(p.nodes['.dp-confirmation-email'].textContent, 'test@example.test');
});

test('recovery uses a separate request and does not require new terms acceptance', async () => {
    const p = await page('', undefined, '?mode=recover');
    assert.equal(p.nodes['.features'].hidden, true);
    assert.equal(p.nodes['.confirmation-again'].href, './?mode=recover');
    assert.equal(p.nodes['.dp-terms-row'].hidden, true);
    assert.equal(p.nodes['.dp-terms-checkbox'].disabled, true);
    p.scripts[0].onload(); p.challenge().callback('valid-token');
    await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
    assert.ok(p.requests[1].url.endsWith('/recover'));
    assert.deepEqual(JSON.parse(p.requests[1].options.body), {email: 'test@example.test', challenge_token: 'valid-token'});
});

test('verified existing registration offers recovery without displaying a key', async () => {
    const p = await page('#dp-verify=' + 'a'.repeat(43), async () => ({ok: true, json: async () => ({outcome: 'already_registered'})}));
    await p.nodes['.dp-verify'].handlers.click();
    assert.equal(p.nodes['.dp-key'].hidden, true);
    assert.equal(p.nodes['.dp-verify'].hidden, true);
    assert.equal(p.nodes['.dp-restart'].href, './?mode=recover');
    assert.match(p.nodes['.dp-status'].textContent, /already has/);
});

test('recovered key shows original expiry even when its email copy fails', async () => {
    const p = await page('#dp-verify=' + 'a'.repeat(43), async () => ({ok: true, json: async () => ({outcome: 'recovered', license_key: 'DP-COM-' + 'b'.repeat(43), expires_at: 1800000000, email_accepted: false})}), '?mode=recover');
    await p.nodes['.dp-verify'].handlers.click();
    assert.equal(p.nodes['.dp-key'].hidden, false);
    assert.match(p.nodes['.dp-status'].textContent, /Original expiry: 2027-01-15/);
    assert.match(p.nodes['.dp-status'].textContent, /email copy could not be sent/);
});

test('blocked domains display the personal/business mailbox message in both forms', async () => {
    for (const search of ['', '?mode=recover']) {
        const p = await page('', async url => url.endsWith('public-config')
            ? {ok: true, json: async () => ({site_key: 'public', terms_url: 'https://www.darkphish.test/terms/', terms_version: 'v1', registration_url: 'https://www.darkphish.test/license/'})}
            : {ok: false, status: 400, json: async () => ({code: 'email_domain_blocked', message: '<untrusted>'})}, search);
        p.scripts[0].onload(); p.challenge().callback('valid-token');
        await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
        assert.equal(p.nodes['.dp-status'].textContent, 'Please use your personal or business email address. Temporary email addresses are not accepted.');
        assert.equal(p.nodes['.dp-status'].focused, true);
        assert.equal(p.nodes['.dp-status'].dataset.state, 'error');
        assert.equal(p.nodes['.dp-confirmation'].hidden, true);
        assert.equal(p.nodes['.dp-request'].hidden, false);
    }
});
test('invalid error bodies use a safe generic message', async () => {
    for (const json of [async () => { throw new SyntaxError(); }, async () => ({code: 'unknown', message: '<untrusted>'}), async () => null]) {
        const p = await page('', async url => url.endsWith('public-config')
            ? {ok: true, json: async () => ({site_key: 'public', terms_url: 'https://www.darkphish.test/terms/', terms_version: 'v1', registration_url: 'https://www.darkphish.test/license/'})}
            : {ok: false, status: 400, json});
        p.scripts[0].onload(); p.challenge().callback('valid-token');
        await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
        assert.match(p.nodes['.dp-status'].textContent, /Please reload the page/);
    }
});
test('a verification blocked by an updated policy shows no key and offers registration', async () => {
    const p = await page('#dp-verify=' + 'a'.repeat(43), async () => ({ok: true, json: async () => ({outcome: 'email_domain_blocked'})}));
    await p.nodes['.dp-verify'].handlers.click();
    assert.match(p.nodes['.dp-status'].textContent, /personal or business/);
    assert.equal(p.nodes['.dp-key'].hidden, true);
    assert.equal(p.nodes['.dp-verify'].hidden, true);
    assert.equal(p.nodes['.dp-restart'].href, './');
});
