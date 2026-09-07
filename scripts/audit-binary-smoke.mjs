// Exercise only audit recording for denied, unauthenticated requests on loopback.
// The caller must provide a dedicated disposable database, never production.
import assert from 'node:assert/strict'
import {spawn, spawnSync} from 'node:child_process'
import {mkdtempSync, readFileSync, writeFileSync} from 'node:fs'
import {createServer} from 'node:net'
import {tmpdir} from 'node:os'
import {dirname, join, resolve} from 'node:path'
import {setTimeout as delay} from 'node:timers/promises'

const [binaryArg, expectedSHA, assetRootArg] = process.argv.slice(2)
assert.ok(binaryArg)
assert.match(expectedSHA || '', /^[a-f0-9]{40}$/)
const backend = process.env.DARKPHISH_AUDIT_TEST_BACKEND
assert.ok(['mysql', 'postgres'].includes(backend))
const dsn = process.env.DARKPHISH_AUDIT_TEST_DSN
assert.ok(dsn)
const binary = resolve(binaryArg), root = assetRootArg ? resolve(assetRootArg) : dirname(binary)
const directory = mkdtempSync(join(tmpdir(), 'darkphish-audit-binary-'))
const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !/^(?:DARKPHISH_|GOPHISH_)/.test(key)))
env.DARKPHISH_INITIAL_ADMIN_PASSWORD = 'synthetic-audit-smoke-bootstrap-only-7931!'
const children = []
const invoke = args => {
  const result = spawnSync(binary, args, {cwd: root, env, encoding: 'utf8', timeout: 45000, windowsHide: true})
  assert.equal(result.error, undefined, 'binary command failed to finish')
  assert.equal(result.status, 0, 'binary audit command failed')
  return result.stdout
}
assert.ok(invoke(['version']).includes(`commit ${expectedSHA}`))
async function port() {
  const server = createServer()
  await new Promise((ok, fail) => {server.once('error', fail); server.listen(0, '127.0.0.1', ok)})
  const value = server.address().port
  await new Promise(ok => server.close(ok))
  return value
}
const configs = []
for (let i = 0; i < 3; i++) {
  const listenPort = await port()
  const path = join(directory, `config-${i}.json`)
  writeFileSync(path, JSON.stringify({
    admin_server: {listen_url: `127.0.0.1:${listenPort}`, use_tls: false},
    phish_server: {listen_url: '127.0.0.1:0', use_tls: false},
    production_mode: false, db_name: backend, db_path: dsn,
    migrations_prefix: join(root, 'db', 'db_'), bootstrap_directory: directory,
    session: {auth_key: '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', encryption_key: 'abcdef0123456789abcdef0123456789'},
    secrets: {provider: 'local', active_key_id: 'fixture', keys: {fixture: '0123456789abcdef0123456789abcdef'}},
    audit: {multi_instance: true, checkpoint_interval: 7, retention_days: 365, active_signing_key_id: 'fixture', signing_keys: {fixture: 'abcdef0123456789abcdef0123456789'}},
  }), {mode: 0o600, flag: 'wx'})
  configs.push({path, base: `http://127.0.0.1:${listenPort}`})
}
const configured = args => invoke(['--config', configs[0].path, ...args])
// One migration/bootstrap runner before starting the other writers.
assert.match(configured(['audit', 'verify']), /Verified \d+ audit events/)
async function start(config) {
  const child = spawn(binary, ['--config', config.path, '--mode', 'admin', '--disable-mailer'], {cwd: root, env, windowsHide: true, stdio: 'ignore'})
  children.push(child)
  let failed = false
  child.once('error', () => {failed = true})
  const deadline = Date.now() + 30000
  while (Date.now() < deadline) {
    assert.ok(!failed && child.exitCode === null, 'audit smoke server exited')
    try {if ((await fetch(`${config.base}/readyz`, {signal: AbortSignal.timeout(1000)})).status === 200) return} catch {}
    await delay(100)
  }
  throw new Error('audit smoke readiness timeout')
}
async function stopAll() {
  await Promise.all(children.splice(0).map(async child => {
    if (child.exitCode !== null || child.signalCode !== null) return
    const stopped = new Promise(ok => child.once('exit', ok))
    child.kill('SIGTERM')
    const timer = setTimeout(() => child.kill('SIGKILL'), 5000)
    try {await stopped} finally {clearTimeout(timer)}
  }))
}
const expected = new Set()
// Cross the runtime's 256-row history page boundary in the actual packaged
// executable, not just in tests compiled directly from the source tree.
const requestsPerServer = 90
try {
  await Promise.all(configs.map(start))
  await Promise.all(configs.map(async config => {
    for (let i = 0; i < requestsPerServer; i++) {
      const response = await fetch(`${config.base}/api/campaigns/`, {signal: AbortSignal.timeout(30000)})
      assert.equal(response.status, 401, 'unauthenticated request must remain denied')
      const requestID = response.headers.get('x-request-id')
      assert.ok(requestID)
      assert.ok(!expected.has(requestID))
      expected.add(requestID)
      await response.text()
    }
  }))
  // Verify with a fourth binary process while the three servers remain alive.
  assert.match(configured(['audit', 'verify']), /Verified \d+ audit events/)
  await stopAll()
  await Promise.all(configs.map(start))
  assert.match(configured(['audit', 'verify']), /Verified \d+ audit events/)
  await stopAll()
  const output = join(directory, 'audit.json')
  configured(['audit', 'export', '--output', output])
  assert.match(configured(['audit', 'verify-export', output, join(directory, 'audit.manifest.json')]), /Verified audit export/)
  const events = JSON.parse(readFileSync(output, 'utf8'))
  const observed = events.filter(event => expected.has(event.request_id))
  assert.equal(observed.length, expected.size, 'every request must have exactly one durable audit event')
  assert.equal(new Set(observed.map(event => event.request_id)).size, expected.size)
  assert.equal(expected.size, configs.length * requestsPerServer)
  console.log(`PASS ${backend} native binary ${expectedSHA}: 3 independent servers, ${expected.size} concurrent denied requests across multiple audit pages, exact durable event coverage, restart, valid chain, signed export verification`)
} finally {
  await stopAll()
}
