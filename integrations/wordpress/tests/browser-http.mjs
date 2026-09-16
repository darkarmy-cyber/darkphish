import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createServer} from 'node:net';
import {fileURLToPath} from 'node:url';

// This harness uses only the isolated CI WordPress/database and mock mail.
const reserve = createServer();
await new Promise(resolve => reserve.listen(0, '127.0.0.1', resolve));
const port = reserve.address().port;
await new Promise(resolve => reserve.close(resolve));
const php = spawn('php', ['-S', `127.0.0.1:${port}`, fileURLToPath(new URL('./integration.php', import.meta.url))], {stdio: ['ignore', 'ignore', 'pipe']});
let errors = ''; php.stderr.on('data', data => { errors += data; });
const base = `http://127.0.0.1:${port}`;
async function call(path, origin, options = {}) {
    return fetch(base + path, {...options, headers: {Origin: origin, ...options.headers}, signal: AbortSignal.timeout(5000)});
}
try {
    let ready = false;
    for (let attempt = 0; attempt < 30; attempt++) {
        try { await fetch(base + '/darkphish-license/v1/public-config'); ready = true; break; }
        catch { await new Promise(resolve => setTimeout(resolve, 100)); }
    }
    assert.ok(ready, `PHP server failed: ${errors}`);
    const origin = 'https://darkphish.test';
    const config = await call('/darkphish-license/v1/public-config', origin);
    assert.equal(config.status, 200);
    assert.equal(config.headers.get('access-control-allow-origin'), origin);
    assert.equal(config.headers.get('access-control-allow-credentials'), null);
    assert.match(config.headers.get('cache-control'), /no-store/);
    assert.equal((await config.json()).site_key, 'test-public-site-key');
    for (const bad of ['https://evil.test', 'null', 'https://darkphish.test.evil.test', 'http://darkphish.test']) {
        const denied = await call('/darkphish-license/v1/public-config', bad);
        assert.equal(denied.status, 403);
        assert.equal(denied.headers.get('access-control-allow-origin'), null);
        assert.equal(denied.headers.get('access-control-allow-credentials'), null);
    }
    const preflight = await call('/darkphish-license/v1/request', origin, {method: 'OPTIONS', headers: {
        'Access-Control-Request-Method': 'POST', 'Access-Control-Request-Headers': 'content-type'
    }});
    assert.equal(preflight.status, 204);
    assert.equal(preflight.headers.get('access-control-allow-headers'), 'Content-Type');
    const simplePreflight = await call('/darkphish-license/v1/public-config', origin, {method: 'OPTIONS', headers: {'Access-Control-Request-Method': 'GET'}});
    assert.equal(simplePreflight.status, 204);
    const forbidden = await call('/darkphish-license/v1/request', origin, {method: 'OPTIONS', headers: {
        'Access-Control-Request-Method': 'DELETE', 'Access-Control-Request-Headers': 'authorization'
    }});
    assert.equal(forbidden.status, 403);
    const failure = await call('/darkphish-license/v1/verify', origin, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{'});
    assert.equal(failure.status, 400);
    assert.equal(failure.headers.get('access-control-allow-origin'), origin);
    assert.equal(failure.headers.get('access-control-allow-credentials'), null);
    const unrelated = await call('/wp/v2/types', 'https://other.test');
    assert.equal(unrelated.headers.get('access-control-allow-origin'), 'https://other.test');
    assert.equal(unrelated.headers.get('access-control-allow-credentials'), 'true');
    console.log('Real HTTP CORS passed: exact origin, preflight, errors, no credentials, unrelated WordPress routes preserved.');
} finally {
    const exited = new Promise(resolve => php.once('exit', resolve));
    php.kill(); await exited;
}
