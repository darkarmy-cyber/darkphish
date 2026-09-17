import assert from 'node:assert/strict'
import test from 'node:test'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createServer} from 'node:net'
import {PassThrough} from 'node:stream'
import {EventEmitter} from 'node:events'
import {captureDiagnostic, reserveLoopbackPorts, waitForReady} from './audit-smoke-process.mjs'

test('ports remain unique and bound until explicit idempotent release', async () => {
  const reservations = await reserveLoopbackPorts(12)
  try {
    assert.equal(new Set(reservations.map(r => r.port)).size, 12)
    for (const {port} of reservations) {
      const server = createServer()
      const failed = once(server, 'error')
      server.listen(port, '127.0.0.1')
      assert.equal((await failed)[0].code, 'EADDRINUSE')
    }
  } finally {await Promise.all(reservations.map(r => r.release()))}
  await Promise.all(reservations.map(r => r.release()))
})
test('diagnostic tails are bounded and redact chunk-split credentials', () => {
  const child = Object.assign(new EventEmitter(), {stdout: new PassThrough(), stderr: new PassThrough(), exitCode: 7, signalCode: null})
  const diagnostic = captureDiagnostic(child, ['split-secret'])
  child.stdout.write('x'.repeat(100000) + '\n')
  child.stderr.write(' split-'); child.stderr.write('secret fatal database problem')
  assert.ok(diagnostic().length < 17000)
  assert.match(diagnostic(), /exit=7/)
  assert.match(diagnostic(), /fatal database problem/)
  assert.doesNotMatch(diagnostic(), /split-secret/)
})
test('truncation discards partial first-line secrets', () => {
  const child = Object.assign(new EventEmitter(), {stdout: new PassThrough(), stderr: new PassThrough()})
  const diagnostic = captureDiagnostic(child, ['sensitive-boundary-value'])
  child.stdout.write('sensitive-boundary-value' + 'x'.repeat(16370) + '\nsafe line')
  assert.doesNotMatch(diagnostic(), /boundary|value/)
  assert.match(diagnostic(), /safe line/)
})
test('real child failure keeps exit code and reason instead of generic assertion', async () => {
  const child = spawn(process.execPath, ['-e', 'console.error("fixture startup failure"); process.exit(7)'], {stdio: ['ignore', 'pipe', 'pipe']})
  const diagnostic = captureDiagnostic(child)
  await once(child, 'close')
  await assert.rejects(waitForReady(child, 'http://127.0.0.1:1', diagnostic, 200), /exit=7.*\nfixture startup failure/)
})
test('terminated child cannot pass readiness', async () => {
  const child = Object.assign(new EventEmitter(), {exitCode: null, signalCode: 'SIGTERM'})
  await assert.rejects(waitForReady(child, 'http://127.0.0.1:1', () => 'signal=SIGTERM', 200), /signal=SIGTERM/)
})
