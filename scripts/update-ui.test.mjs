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
  const elements = new Map(), requests = [], timers = [], prompts = [], dialogs = []

  function element() {
    return {
      value: '', textContent: '', style: {}, children: [], hidden: false, disabled: false,
      text(value) { this.value = value; this.textContent = value; return this },
      prop(key, value) { this[key] = value; return this },
      on(event, fn) { this[event] = fn; return this },
      val(value) { if (arguments.length) { this.value = value; return this } return this.value }
    }
  }

  const $ = selector => {
    if (typeof selector === 'function') return selector()
    if (!elements.has(selector)) elements.set(selector, element())
    return elements.get(selector)
  }
  const document = {
    getElementById(id) { return $('#' + id) },
    createElement(tag) { const el = element(); el.tagName = tag.toUpperCase(); return el }
  }

  $.ajax = options => {
    assert.equal(options.timeout, ['/api/updates','/api/updates/check'].includes(options.url) ? 60000 : 15000)
    assert.ok(options.url.startsWith('/api/'))
    const request = Object.assign(deferred(), {url: options.url.slice(4), method: options.method,
      body: options.data && JSON.parse(options.data), session: true})
    requests.push(request); return request
  }

  const context = vm.createContext({$, document,
    setTimeout: fn => timers.push(fn),
    Swal: {fire: options => { const prompt = {options}; prompts.push(prompt); return {then: fn => { prompt.answer = fn }} },
      update: options => dialogs.push(options), enableButtons: () => {}}
  })
  vm.runInContext(readFileSync(new URL(path, import.meta.url), 'utf8'), context)

  function normalizedStatus(extra = {}) {
    const current = extra.current_version ?? '0.14.0'
    const latest = extra.latest_version !== undefined ? extra.latest_version : '0.15.0'
    let releases = extra.releases
    if (releases === undefined) {
      releases = []
      if (latest) releases.push({
        version: latest, published_at: '2026-09-28T10:00:00Z', release_notes: 'Release ' + latest,
        current: latest === current, latest: true, compatible: true, action: latest === current ? 'reinstall' : 'upgrade'
      })
    }
    return {current_version: current, latest_version: latest, releases, applying: false, ...extra}
  }

  return {$, document, requests, timers, prompts, dialogs,
    status: extra => requests.at(-1).resolve(normalizedStatus(extra)),
    click: id => $(id).click(),
    confirm: () => { $('#updateApply').click(); prompts.at(-1).options.preConfirm('test-password') }
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
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.$('#updateApplyHint').value, reason)
    assert.equal(p.$('#updateCheck').disabled, false)
    p.click('#updateApply')
    assert.equal(p.prompts.length, 0)
    p.click('#updateCheck')
    p.status({unsupported_reason: 'MySQL is unsupported'})
    assert.equal(p.$('#updateBlockedReason').value, 'MySQL is unsupported')
    assert.equal(p.$('#updateApply').disabled, true)
  })

  test(`${path}: no release, check failures and applying state cannot enable apply`, () => {
    const p = page(path)
    assert.equal(p.$('#updateApply').disabled, true)
    p.status({latest_version: '', releases: []})
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.$('#updateStatus').value, 'You have the latest version')
    p.click('#updateCheck'); p.status({error: 'Release check failed'})
    assert.equal(p.$('#updateApply').disabled, true)
    p.click('#updateCheck'); p.requests.at(-1).reject({})
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.$('#updateStatus').value, 'Unable to complete the update')
    assert.equal(p.$('#updateCheck').disabled, false)
    p.click('#updateCheck'); p.status({applying: true})
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.$('#updateCheck').disabled, true)
  })

  test(`${path}: the single action always targets the latest release`, () => {
    const p = page(path)
    p.status({latest_version: '0.16.0'})
    assert.equal(p.$('#updateCurrent').value, '0.14.0')
    assert.equal(p.$('#updateLatest').value, '0.16.0')
    assert.equal(p.$('#updateAvailableBadge').hidden, false)
    assert.equal(p.$('#updateAvailableText').value, 'Version 0.16.0 is available!')
    assert.equal(p.$('#updateApplyLabel').value, 'Update to 0.16.0')
    assert.equal(p.$('#updateApplyHint').value, 'Ready to install 0.16.0.')
    assert.equal(p.$('#updateApply').disabled, false)
    p.click('#updateCheck')
    p.status({current_version: '0.16.0', latest_version: '0.16.0'})
    assert.equal(p.$('#updateAvailableBadge').hidden, true)
    assert.equal(p.$('#updateApplyLabel').value, 'Update now')
    assert.equal(p.$('#updateApplyHint').value, 'You are running the latest version.')
    assert.equal(p.$('#updateApply').disabled, true)
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
    assert.deepEqual(p.requests.at(-1).body, {method: 'password', password: 'test-password'})
    p.requests.at(-1).reject({responseJSON: {message: 'Incorrect password'}})
    assert.equal(p.requests.at(-1).url, '/updates')
    p.status({})
    assert.equal(p.$('#updateStatus').value, 'Incorrect password')
    assert.equal(p.$('#updateApply').disabled, false)
    assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 0)
  })

  test(`${path}: a persisted previous result cannot hide a new authentication failure`, () => {
    for (const previous of ['Update completed successfully.', 'Update failed and was rolled back.']) {
      const p = page(path)
      p.status({result: previous}); p.confirm()
      p.requests.at(-1).reject({responseJSON: {message: 'Incorrect password'}})
      assert.equal(p.requests.at(-1).url, '/updates')
      p.status({result: previous})
      assert.equal(p.$('#updateStatus').value, 'Incorrect password')
      assert.equal(p.$('#updateApply').disabled, false)
      assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 0)
      assert.equal(p.timers.length, 0)
    }
  })

  test(`${path}: rejected apply reports the transport rejection without pretending an outcome`, () => {
    for (const status of [401, 403, 409, 428]) {
      const previous = 'Update completed successfully.'
      const p = page(path)
      p.status({result: previous}); p.confirm(); p.requests.at(-1).resolve({})
      p.requests.at(-1).reject({status, responseJSON: {message: 'Current request was not confirmed'}})
      p.status({result: previous})
      assert.equal(p.$('#updateStatus').value, 'Current request was not confirmed')
      assert.equal(p.dialogs.at(-1).title, 'Update failed')
      assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 1)
      assert.equal(p.timers.length, 0)
    }
  })

  test(`${path}: ambiguous apply failure keeps watching and closes only on a receipt`, () => {
    for (const previous of ['Update completed successfully.', 'Update failed and was rolled back.']) {
      const p = page(path)
      p.status({result: previous}); p.confirm(); p.requests.at(-1).resolve({})
      p.requests.at(-1).reject({status: 0})
      assert.equal(p.requests.at(-1).url, '/updates')
      p.status({applying: true})
      assert.equal(p.$('#updateApply').disabled, true)
      assert.equal(p.timers.length, 1)
      p.timers.shift()()
      if (previous.includes('rolled back')) {
        p.status({applying: false, result: previous, result_code: 'rollback'})
        assert.equal(p.dialogs.at(-1).title, 'Update failed')
        assert.equal(p.$('#updateStatus').value, previous)
      } else {
        p.status({applying: false, current_version: '0.15.0', result_code: 'applied', result: 'Update completed successfully'})
        assert.equal(p.dialogs.at(-1).title, 'Update successful')
        assert.equal(p.$('#updateStatus').value, 'DarkPhish 0.15.0 is running. Update completed successfully')
      }
      assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 1)
    }
  })

  test(`${path}: a watched disconnect cannot claim success without the target version`, () => {
    const p = page(path), result = 'Update completed successfully.'
    p.status({result}); p.confirm(); p.requests.at(-1).resolve({})
    p.requests.at(-1).reject({status: 0})
    p.status({applying: true})
    p.timers.shift()()
    p.requests.at(-1).reject({status: 0})
    assert.equal(p.$('#updateStatus').value, 'Waiting for DarkPhish to restart…')
    assert.equal(p.$('#updateApply').disabled, true)
    p.timers.shift()()
    p.status({current_version: '0.14.0', result_code: 'applied', result})
    assert.equal(p.dialogs.at(-1).title, 'Updating DarkPhish')
    assert.equal(p.timers.length, 1)
    p.timers.shift()()
    p.status({current_version: '0.15.0', result_code: 'applied', result})
    assert.equal(p.dialogs.at(-1).title, 'Update successful')
    assert.equal(p.$('#updateStatus').value, 'DarkPhish 0.15.0 is running. ' + result)
    assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 1)
  })

  test(`${path}: the accepted transaction target overrides a stale tab and later feed changes`, () => {
    for (const transport of ['accepted','concurrent','interrupted']) {
      const p = page(path)
      p.status({latest_version:'0.15.0'}); p.confirm(); p.requests.at(-1).resolve({})
      if (transport==='accepted') p.requests.at(-1).resolve({target_version:'0.16.0'})
      else {
        p.requests.at(-1).reject({status:transport==='concurrent'?409:0})
        p.status({applying:true,target_version:'0.16.0',latest_version:'0.17.0'})
      }
      assert.equal(p.timers.length, 1)
      p.timers.shift()()
      p.status({applying:false,current_version:'0.16.0',latest_version:'0.17.0',result_code:'applied',result:'Update completed successfully'})
      assert.equal(p.dialogs.at(-1).title,'Update successful')
      assert.equal(p.requests.filter(r=>r.url==='/updates/apply').length,1)
    }
  })

  test(`${path}: a rejected concurrent apply still monitors the authoritative operation`, () => {
    for (const result_code of ['applied','rollback']) {
      const p = page(path)
      p.status({}); p.confirm(); p.requests.at(-1).resolve({})
      p.requests.at(-1).reject({status:409,responseJSON:{message:'An update is already in progress'}})
      p.status({applying:true})
      assert.equal(p.$('#updateApply').disabled,true)
      assert.equal(p.timers.length,1)
      p.timers.shift()()
      p.status({applying:false,current_version:result_code==='applied'?'0.15.0':'0.14.0',result_code,result:result_code==='applied'?'Update completed successfully':'Update failed; previous application restored'})
      assert.equal(p.dialogs.at(-1).title,result_code==='applied'?'Update successful':'Update failed')
      assert.equal(p.requests.filter(r=>r.url==='/updates/apply').length,1)
    }
  })

  test(`${path}: rejected apply can be retried only after fresh status; failed status stays closed`, () => {
    const p = page(path)
    p.status({}); p.confirm(); p.requests.at(-1).resolve({})
    p.requests.at(-1).reject({status: 409, responseJSON: {message: 'Check again before applying'}})
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
    p.status({releases: [
      {version: '0.15.0', published_at: '2026-09-28T10:00:00Z', release_notes: text, latest: true, compatible: true, action: 'upgrade'}
    ]})
    assert.equal(p.$('#updateNotes').value, text)
    assert.equal(p.$('#updateNotesSection').hidden, false)
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
    p.status({current_version: '0.15.0', applying: false, result_code: 'applied', result: 'Update completed successfully'})
    assert.equal(p.$('#updateCheck').disabled, false)
    assert.equal(p.requests.filter(r => r.method === 'POST').length, 0)
  })

}

for (const path of ['../static/js/src/app/update.js', '../static/js/dist/app/update.min.js']) {
  test(`${path}: the same dialog stays open through progress and completion`, () => {
    const p = page(path); p.status({}); p.confirm()
    assert.equal(p.prompts.length, 1)
    p.requests.at(-1).resolve({}); p.requests.at(-1).resolve({})
    assert.equal(p.dialogs.at(-1).showConfirmButton, false)
    assert.match(p.dialogs.at(-1).html, /role="status"/)
    assert.match(p.dialogs.at(-1).html, /update-progress-track/)
    assert.doesNotMatch(p.dialogs.at(-1).html, /test-password/)
    p.timers.shift()(); p.status({current_version: '0.15.0', result_code: 'applied', result: 'Update completed successfully'})
    assert.equal(p.dialogs.at(-1).title, 'Update successful')
    assert.equal(p.dialogs.at(-1).confirmButtonText, 'Close')
    assert.equal(p.prompts.length, 1)
    assert.equal(p.prompts[0].options.preConfirm(''), true)
    assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 1)
  })
  test(`${path}: every safe failure code shows diagnostics even when the feed is offline`, () => {
    const diagnostics = {
      verification_failed: /authenticity or archive verification failed/,
      backup_failed: /pre-update backup failed/,
      apply_failed: /Installation could not start/,
      rollback: /previous application and database were restored/
    }
    for (const code of Object.keys(diagnostics)) {
      const p = page(path), text = '<img src=x onerror=alert(1)> Failure'
      p.status({result: text, result_code: code}); p.confirm(); p.requests.at(-1).resolve({}); p.requests.at(-1).resolve({})
      p.timers.shift()(); p.status({result: text, result_code: code, error: 'Release feed unavailable'})
      assert.equal(p.dialogs.at(-1).title, 'Update failed')
      assert.equal(p.$('#updateDialogMessage').value, text)
      assert.match(p.$('#updateDialogDiagnostics').value, diagnostics[code])
      assert.doesNotMatch(p.dialogs.at(-1).html, /onerror/)
      assert.equal(p.$('#updateApply').disabled, true)
    }
  })
  test(`${path}: stale success and a version change without a completion receipt cannot claim success`, () => {
    for (const status of [
      {result_code: 'applied', result: 'Update completed successfully', current_version: '0.14.0'},
      {current_version: '0.15.0'},
      {result_code: 'applied', result: 'Update completed successfully', current_version: '0.16.0'}
    ]) {
      const p = page(path); p.status({}); p.confirm(); p.requests.at(-1).resolve({}); p.requests.at(-1).resolve({})
      p.timers.shift()(); p.status(status)
      assert.equal(p.dialogs.at(-1).title, 'Updating DarkPhish')
      p.timers.shift()(); p.requests.at(-1).reject({status: 401})
      assert.equal(p.dialogs.at(-1).title, 'Update not confirmed')
      assert.equal(p.$('#updateApply').disabled, true)
    }
  })
  test(`${path}: monitoring is bounded and cannot retry the apply POST`, () => {
    const p = page(path); p.status({}); p.confirm(); p.requests.at(-1).resolve({}); p.requests.at(-1).resolve({})
    for (let i = 0; i < 240; i++) { assert.equal(p.timers.length, 1); p.timers.shift()(); p.requests.at(-1).reject({status: 0}) }
    assert.equal(p.timers.length, 0)
    assert.equal(p.dialogs.at(-1).title, 'Update not confirmed')
    assert.equal(p.$('#updateApply').disabled, true)
    assert.equal(p.requests.filter(r => r.url === '/updates/apply').length, 1)
  })
}

test('the update page offers one action and keeps its guidance accessible without a mouse hover', () => {
  const html = readFileSync(new URL('../templates/update.html', import.meta.url), 'utf8')
  assert.match(html, /id="updateBlocked"[^>]*role="status"/)
  assert.match(html, /aria-describedby="updateApplyHint"/)
  assert.match(html, /id="updateApplyLabel"/)
  assert.match(html, /A verified update stops the application, backs it up, and restarts it with the latest stable version\./)
  assert.doesNotMatch(html, /updateTarget|<select|version selector/i)
})
