import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'

function deferred() {
  let outcome, value
  const callbacks = {done: [], fail: [], always: []}
  const api = {}
  for (const kind of Object.keys(callbacks)) api[kind] = fn => {
    callbacks[kind].push(fn)
    if (outcome && (kind === outcome || kind === 'always')) fn(value)
    return api
  }
  function settle(kind, result) {
    assert.equal(outcome, undefined)
    outcome = kind; value = result
    for (const fn of callbacks[kind]) fn(value)
    for (const fn of callbacks.always) fn(value)
  }
  return Object.assign(api, {resolve: result => settle('done', result), reject: error => settle('fail', error)})
}

function page(path) {
  const elements = new Map(), requests = [], timers = [], prompts = [], notifications = []
  const $ = selector => {
    if (typeof selector === 'function') return selector()
    if (!elements.has(selector)) elements.set(selector, {
      text(value) { this.value = value; return this },
      prop(key, value) { this[key] = value; return this },
      on(event, fn) { this[event] = fn; return this }
    })
    return elements.get(selector)
  }
  const context = vm.createContext({$, renderAdminNotification: status => notifications.push(status),
    setTimeout: fn => timers.push(fn),
    Swal: {fire: options => ({then: fn => prompts.push({options, answer: fn})})},
    query: (url, method, body, session) => {
      const request = Object.assign(deferred(), {url, method, body: body && {...body}, session})
      requests.push(request); return request
    }
  })
  vm.runInContext(readFileSync(new URL(path, import.meta.url), 'utf8'), context)
  return {$, requests, timers, prompts, notifications,
    status: extra => requests.at(-1).resolve({current_version: '0.14.0', latest_version: '0.15.0', available: true, ...extra}),
    click: id => $(id).click(),
    confirm: () => { $('#updateApply').click(); prompts.at(-1).answer({value: 'test-password'}) }
  }
}

for (const path of ['../static/js/src/app/update.js', '../static/js/dist/app/update.min.js']) {
  test(`${path}: blocked readiness is separate from release availability and previous results`, () => {
    const p = page(path)
    const reason = 'one-click update requires administrator-installed /usr/bin/gh >= 2.100.0'
    p.status({unsupported_reason: reason, result: 'The previous update was rolled back'})
    assert.equal(p.$('#updateStatus').value, 'The previous update was rolled back')
    assert.equal(p.$('#updateBlocked').hidden, false)
    assert.equal(p.$('#updateBlockedReason').value, reason)
    assert.equal(p.$('#updateVerifierHelp').hidden, false)
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.$('#updateApplyHint').value, reason)
    assert.equal(p.$('#updateCheck').disabled, false)
    assert.equal(p.notifications.at(-1).available, true)
    p.click('#updateApply')
    assert.equal(p.prompts.length, 0)
    p.click('#updateCheck')
    p.status({unsupported_reason: 'MySQL is unsupported'})
    assert.equal(p.$('#updateVerifierHelp').hidden, true)
    assert.equal(p.$('#updateApply').disabled, true)
  })

  test(`${path}: no release, check failures and applying state cannot enable apply`, () => {
    const p = page(path)
    assert.equal(p.$('#updateApply').disabled, true)
    p.status({available: false})
    assert.equal(p.$('#updateApply').disabled, true)
    p.click('#updateCheck'); p.status({error: 'Release check failed'})
    assert.equal(p.$('#updateApply').disabled, true)
    p.click('#updateCheck'); p.requests.at(-1).reject({})
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.$('#updateCheck').disabled, false)
    p.click('#updateCheck'); p.status({applying: true})
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.$('#updateCheck').disabled, true)
  })

  test(`${path}: cancellation and failed password verification permit a checked retry`, () => {
    const p = page(path)
    p.status({})
    assert.equal(p.$('#updateApply').disabled, false)
    p.click('#updateApply'); p.prompts.at(-1).answer({})
    assert.equal(p.$('#updateApply').disabled, false)
    p.confirm()
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.requests.at(-1).url, '/reauthenticate')
    p.requests.at(-1).reject({responseJSON: {message: 'Incorrect password'}})
    assert.equal(p.requests.at(-1).url, '/updates')
    p.status({})
    assert.equal(p.$('#updateStatus').value, 'Incorrect password')
    assert.equal(p.$('#updateApply').disabled, false)
    assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 0)
  })

  test(`${path}: ambiguous apply failure rechecks and watches without duplicate POST`, () => {
    const p = page(path)
    p.status({}); p.confirm(); p.requests.at(-1).resolve({})
    assert.equal(p.requests.at(-1).url, '/updates/apply')
    p.requests.at(-1).reject({})
    assert.equal(p.requests.at(-1).url, '/updates')
    p.status({applying: true})
    assert.equal(p.$('#updateApply').disabled, true)
    p.click('#updateApply')
    assert.equal(p.timers.length, 1)
    p.timers.shift()()
    p.requests.at(-1).resolve({current_version: '0.15.0', available: false, applying: false, result: 'Update completed'})
    assert.equal(p.$('#updateStatus').value, 'Update completed')
    assert.equal(p.$('#updateCheck').disabled, false)
    assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 1)
  })

  test(`${path}: rejected apply can be retried only after fresh status; failed status stays closed`, () => {
    const p = page(path)
    p.status({}); p.confirm(); p.requests.at(-1).resolve({})
    p.requests.at(-1).reject({responseJSON: {message: 'Check again before applying'}})
    p.status({})
    assert.equal(p.$('#updateApply').disabled, false)
    assert.equal(p.$('#updateStatus').value, 'Check again before applying')
    p.confirm(); p.requests.at(-1).reject({})
    p.requests.at(-1).reject({})
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.$('#updateCheck').disabled, false)
  })

  test(`${path}: accepted apply waits through a disconnect and renders untrusted strings as text`, () => {
    const p = page(path), text = '<img src=x onerror=alert(1)>'
    p.status({release_notes: text, latest_version: text})
    assert.equal(p.$('#updateNotes').value, text)
    assert.equal(p.$('#updateLatest').value, text)
    p.confirm(); p.requests.at(-1).resolve({}); p.requests.at(-1).resolve({message: 'Restarting'})
    assert.equal(p.$('#updateCheck').disabled, true)
    p.timers.shift()(); p.requests.at(-1).reject({})
    assert.equal(p.$('#updateStatus').value, 'Waiting for DarkPhish to restart…')
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.timers.length, 1)
    assert.ok(p.requests.every(r => r.session === true))
  })

  test(`${path}: reopening the page during apply resumes watching`, () => {
    const p = page(path)
    p.status({applying: true})
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.timers.length, 1)
    p.timers.shift()()
    p.status({applying: false, available: false, result: 'Update completed'})
    assert.equal(p.$('#updateCheck').disabled, false)
    assert.equal(p.requests.filter(r => r.method === 'POST').length, 0)
  })
}

test('update prerequisite guidance remains visible and accessible without a mouse hover', () => {
  const html = readFileSync(new URL('../templates/update.html', import.meta.url), 'utf8')
  assert.match(html, /id="updateBlocked"[^>]*role="status"/)
  assert.match(html, /aria-describedby="updateApplyHint"/)
  assert.match(html, /GitHub CLI 2\.100\.0 or newer/)
  assert.match(html, /restart the DarkPhish service/)
  assert.match(html, /href="https:\/\/docs\.darkphish\.sk\/"/)
})
