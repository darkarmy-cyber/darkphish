import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
const source = readFileSync(new URL('../static-site/license/registration.js', import.meta.url), 'utf8');
function element(hidden = false) {
    return {hidden, disabled: false, textContent: '', handlers: {}, addEventListener(name, fn) { this.handlers[name] = fn; }};
}
async function page(hash = '', reply = async () => ({ok: true, json: async () => ({site_key: 'public', terms_url: 'https://www.darkphish.test/terms/', terms_version: 'v1', registration_url: 'https://www.darkphish.test/license/'})})) {
    const nodes = Object.fromEntries(['.dp-status', '.dp-request', '.dp-verify', '.dp-restart', '.dp-key', '.dp-terms', '.dp-challenge'].map(name => [name, element(name !== '.dp-status')]));
    const submit = element(); submit.disabled = true;
    nodes['.dp-request'].querySelector = () => submit;
    nodes['.dp-request'].reportValidity = () => true;
    nodes['.dp-request'].reset = () => {};
    const requests = [], scripts = [], replaced = [];
    let ready, challenge;
    const context = {
        URL, URLSearchParams, Error,
        location: {hash, origin: 'https://www.darkphish.test', pathname: '/license/', search: ''},
        history: {replaceState: (...args) => replaced.push(args)},
        document: {getElementById: () => ({dataset: {api: 'https://fsociety.test/wp-json/darkphish-license/v1/'}, querySelector: name => nodes[name]}),
            addEventListener: (_, fn) => { ready = fn; }, createElement: () => ({}), head: {appendChild: script => scripts.push(script)}},
        fetch: async (url, options) => { requests.push({url, options}); return reply(url, options); },
        FormData: class { get() { return 'test@example.test'; } },
        window: {turnstile: {render: (_, options) => { challenge = options; return 'widget'; }, reset: () => {}}}
    };
    vm.runInNewContext(source, context); await ready();
    return {nodes, requests, scripts, replaced, submit, challenge: () => challenge};
}
test('verification removes the fragment before external work and requires an explicit click', async () => {
    const token = 'a'.repeat(43);
    const p = await page('#dp-verify=' + token, async () => ({ok: true, json: async () => ({license_key: 'DP-COM-' + 'b'.repeat(43)})}));
    assert.equal(p.replaced[0][2], '/license/');
    assert.equal(p.requests.length, 0); assert.equal(p.scripts.length, 0);
    assert.equal(p.nodes['.dp-verify'].hidden, false);
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
    p.scripts[0].onload(); p.challenge().callback('valid-token');
    assert.equal(p.submit.disabled, false);
    await p.nodes['.dp-request'].handlers.submit({preventDefault() {}});
    assert.deepEqual(JSON.parse(p.requests[1].options.body), {email: 'test@example.test', terms_version: 'v1', challenge_token: 'valid-token'});
    assert.equal(p.submit.disabled, true);
});
test('configuration failures keep the registration form unavailable', async () => {
    const p = await page('', async () => ({ok: false, status: 503}));
    assert.equal(p.nodes['.dp-request'].hidden, true); assert.equal(p.scripts.length, 0);
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
