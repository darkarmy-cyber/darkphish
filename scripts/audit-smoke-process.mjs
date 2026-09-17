import {createServer} from 'node:net'
import {setTimeout as delay} from 'node:timers/promises'

// Keep every allocation bound until immediately before its child is spawned.
// listen(0)-close repeated N times may return the SAME port more than once.
export async function reserveLoopbackPorts(count) {
  const reservations = []
  try {
    for (let i = 0; i < count; i++) {
      const server = createServer()
      await new Promise((ok, fail) => {server.once('error', fail); server.listen(0, '127.0.0.1', ok)})
      let released = false
      reservations.push({port: server.address().port, release: async () => {
        if (released) return
        released = true
        await new Promise((ok, fail) => server.close(error => error ? fail(error) : ok()))
      }})
    }
    return reservations
  } catch (error) {
    await Promise.all(reservations.map(r => r.release()))
    throw error
  }
}

// Buffer first, redact second: secrets split across stream chunks must never
// reach CI output. Retain only a bounded tail, not arbitrary process output.
export function captureDiagnostic(child, secrets = []) {
  let tail = '', spawnError = '', truncated = false
  const append = data => {
    const value = tail + data.toString()
    truncated ||= value.length > 16384
    tail = value.slice(-16384)
  }
  child.stdout?.on('data', append)
  child.stderr?.on('data', append)
  child.on('error', error => {spawnError = error.code || 'spawn-error'})
  return () => {
    // Drop the incomplete first line after truncation so a partial secret at
    // the buffer boundary cannot evade exact-value redaction.
    const safeTail = truncated ? (tail.includes('\n') ? tail.slice(tail.indexOf('\n') + 1) : '[overlong diagnostic line omitted]') : tail
    let output = `exit=${child.exitCode} signal=${child.signalCode} spawn=${spawnError}\n${safeTail}`
    for (const secret of [...secrets].filter(Boolean).sort((a, b) => b.length - a.length)) output = output.split(secret).join('[REDACTED]')
    // Strip terminal/control escapes and guard common DSN credential syntax.
    return output.replace(/\x1b\[[0-9;]*[A-Za-z]/g, '').replace(/[\x00-\x08\x0b-\x1f\x7f]/g, '')
      .replace(/(postgres(?:ql)?:\/\/)[^\s/@]+:[^\s/@]+@/gi, '$1[REDACTED]@')
      .replace(/[^\s:]+:[^\s@]+@tcp\(/g, '[REDACTED]@tcp(')
  }
}

export async function waitForReady(child, base, diagnostic, timeout = 30000) {
  let failed = false
  child.once('error', () => {failed = true})
  const deadline = Date.now() + timeout
  const alive = () => !failed && child.exitCode === null && child.signalCode === null
  while (Date.now() < deadline) {
    if (!alive()) throw new Error(`audit smoke server exited (${base}): ${diagnostic()}`)
    try {
      const response = await fetch(`${base}/readyz`, {signal: AbortSignal.timeout(Math.min(1000, timeout))})
      await response.body?.cancel()
      if (response.status === 200 && alive()) return
    } catch {}
    await delay(100)
  }
  throw new Error(`audit smoke readiness timeout (${base}): ${diagnostic()}`)
}
