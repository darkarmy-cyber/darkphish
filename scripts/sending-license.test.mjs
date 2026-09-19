import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'

for (const variant of ['src', 'dist']) {
  test(`licensing/${variant}: activation guidance reflects every license state without sending a request`, () => {
    const code = readFileSync(new URL(`../static/js/${variant}/app/licensing${variant === 'dist' ? '.min' : ''}.js`, import.meta.url), 'utf8')
    for (const state of ['missing', 'expired', 'invalid', 'active', 'grace']) {
      const document = {}, values = new Map()
      const $ = selector => selector === document ? {ready: fn => fn()} : {
        text(value) { values.set(selector, value); return this }, prop() { return this }, on() { return this }
      }
      const query = (path, method) => {
        assert.equal(path, '/license'); assert.equal(method, 'GET')
        return {done(fn) { fn({configured: true, state}); return this }, fail() { return this }}
      }
      vm.runInNewContext(code, {$, document, query})
      const message = values.get('#licenseMessage')
      if (state === 'active') assert.match(message, /license is active/)
      else if (state === 'grace') assert.match(message, /offline grace/)
      else assert.match(message, /test emails are blocked/)
    }
  })
}

for (const name of ['campaigns', 'sending_profiles']) {
  for (const variant of ['src', 'dist']) {
    test(`${name}/${variant}: disabled sending actions do not reach API or confirmation`, () => {
      const code = readFileSync(new URL(`../static/js/${variant}/app/${name}${variant === 'dist' ? '.min' : ''}.js`, import.meta.url), 'utf8')
      const document = {}
      const $ = selector => {
        if (selector === document) return {ready() {}}
        if (selector === '#launchButton' || selector === '#sendTestModalSubmit') return {prop: key => { assert.equal(key, 'disabled'); return true }}
        throw new Error(`Sending logic reached ${selector}`)
      }
      const context = vm.createContext({$, document})
      vm.runInContext(code, context)
      if (name === 'campaigns') context.launch()
      context.sendTestEmail()
    })
  }
}
