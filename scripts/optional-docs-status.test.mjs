import assert from 'node:assert/strict'
import test from 'node:test'
import {mergeStateReady} from './optional-docs-status.mjs'

function fixture() {
  const f = {
    pr: {mergeable: true, mergeable_state: 'unstable', head: {sha: 'a'.repeat(40)}},
    protection: {contexts: ['tests'], checks: []}, rules: [],
    checks: [{status: 'completed', conclusion: 'success'}],
    statuses: [{id: 2, context: 'GitBook (./changes)', state: 'pending', creator: {login: 'gitbook-com[bot]', id: 92167642, type: 'Bot'}}],
  }
  const request = async path => {
    assert.ok(path.endsWith('/branches/main'), 'no administrative permission needed')
    return {protected: true, protection: {required_status_checks: f.protection}}
  }
  const pages = async path => path.includes('/check-runs?') ? f.checks : path.includes('/rules/') ? f.rules : f.statuses
  f.ready = () => mergeStateReady('owner/repo', f.pr, {request, pages})
  return f
}
test('authentic optional GitBook preview cannot block otherwise green checks', async () => {
  for (const state of ['pending', 'failure', 'error']) {
    const f = fixture(); f.statuses[0].state = state
    assert.equal(await f.ready(), true)
  }
})
test('latest status wins even when an older preview succeeded', async () => {
  const f = fixture(); f.statuses.push({...f.statuses[0], id: 1, state: 'success'})
  assert.equal(await f.ready(), true)
  f.statuses[0].context = 'security'; assert.equal(await f.ready(), false)
})
test('unknown status, forged bot, required docs, failed CI and unsafe merge states fail closed', async () => {
  for (const mutate of [
    f => {f.statuses[0].creator.id = 1}, f => {f.statuses[0].creator.login = 'other'},
    f => {f.statuses[0].creator.type = 'User'}, f => {f.statuses[0].context = 'GitBook (./unknown)'},
    f => {f.protection.contexts.push('GitBook (./changes)')},
    f => {f.rules.push({type: 'required_status_checks', parameters: {required_status_checks: [{context: 'GitBook (./changes)'}]}})},
    f => {f.checks.push({status: 'completed', conclusion: 'failure'})},
    f => {f.checks[0].status = 'in_progress'}, f => {f.checks = []},
    f => {f.statuses.push({id: 3, context: 'security', state: 'pending'})},
    f => {f.statuses[0].state = 'unknown'}, f => {f.statuses[0].id = null},
    f => {f.pr.mergeable = false}, f => {f.pr.mergeable_state = 'blocked'},
    f => {f.pr.mergeable_state = 'behind'}, f => {f.pr.mergeable_state = 'dirty'},
    f => {f.pr.mergeable_state = 'unknown'}, f => {f.statuses = []},
  ]) {const f = fixture(); mutate(f); assert.equal(await f.ready(), false)}
})
test('API failure cannot permit a merge', async () => {
  const f = fixture()
  await assert.rejects(mergeStateReady('owner/repo', f.pr, {pages: async () => f.checks, request: async () => {throw Error('unavailable')}}), /unavailable/)
})
test('a clean GitHub aggregate cannot hide an additional failed dispatched CI', async () => {
  const f = fixture(); f.pr.mergeable_state = 'clean'
  assert.equal(await f.ready(), true)
  f.checks.push({status: 'completed', conclusion: 'failure'})
  assert.equal(await f.ready(), false)
})
