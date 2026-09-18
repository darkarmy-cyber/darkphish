import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import {posix} from 'node:path'
import vm from 'node:vm'

function gate(results) {
  const workflow = readFileSync(new URL('../.github/workflows/codeql.yml', import.meta.url), 'utf8').replaceAll('\r\n', '\n')
  const source = workflow.match(/node --input-type=module <<'EOF'\n([\s\S]*?)\n          EOF/)[1]
    .replace(/^          /gm, '').replace(/^import .*$/gm, '')
  const logs = []
  let failure
  try {
    vm.runInNewContext(source, {
      process: {env: {CODEQL_RESULTS: '/analysis'}},
      readdirSync: () => ['go.sarif'], statSync: () => ({isDirectory: () => false}), join: posix.join,
      readFileSync: () => JSON.stringify({version: '2.1.0', runs: [{results}]}),
      console: {log: value => logs.push(value)},
    })
  } catch (error) { failure = error.message }
  return {logs, failure}
}

test('CodeQL diagnostic reports only rule and relative coordinates, keeping every finding blocking', () => {
  const result = {ruleId: 'go/example-rule', message: {text: 'PRIVATE_MESSAGE'}, codeFlows: ['PRIVATE_FLOW'],
    locations: [{physicalLocation: {artifactLocation: {uri: 'controllers/api/example.go'}, region: {startLine: 12, snippet: {text: 'PRIVATE_SOURCE'}}}}]}
  const {logs, failure} = gate([result])
  assert.match(failure, /found 1 local result/)
  assert.deepEqual(JSON.parse(logs[0]), {rule: 'go/example-rule', file: 'controllers/api/example.go', line: 12})
  assert.doesNotMatch(logs.join(''), /PRIVATE/)
  assert.equal(gate([]).failure, undefined)
})

test('CodeQL diagnostics bound output and reject unsafe identifiers without suppressing findings', () => {
  const result = {ruleId: 'bad\n::warning::PRIVATE', locations: [{physicalLocation: {artifactLocation: {uri: '../../PRIVATE'}, region: {startLine: -1}}}]}
  const {logs, failure} = gate(Array.from({length: 25}, () => result))
  assert.match(failure, /found 25 local result/)
  assert.equal(logs.length, 20)
  assert.deepEqual(JSON.parse(logs[0]), {rule: 'unavailable', file: 'unavailable', line: null})
  assert.doesNotMatch(logs.join(''), /PRIVATE/)
})
