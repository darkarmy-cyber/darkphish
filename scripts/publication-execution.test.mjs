import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'

// Execute the real guard functions with in-memory I/O; never run its CLI entry.
const source = readFileSync(new URL('../.github/scripts/release-publication-guard.mjs', import.meta.url), 'utf8')
  .replace(/^import [\s\S]*? from "[^"]+"\r?\n/gm, '')
  .split('const command = process.argv[2]')[0]
const sha = 'a'.repeat(40), next = 'b'.repeat(40)
function fixture() {
  const calls = [], release = {id: 12, tag_name: 'v0.12.0', draft: false, prerelease: false}
  const state = {main: sha, release, calls, outputs: []}
  const sandbox = {
    process: {env: {RECOVERY_EXECUTION_SHA: sha, RECOVERY_PRECHECK_TAG_ABSENT: `false:commit:${sha}`, GITHUB_OUTPUT: 'memory'}},
    console: {warn() {}, log() {}}, setTimeout: fn => {
      if(calls.length > 3) throw Error('Unexpected withdrawal retry loop in test')
      fn()
    }, Buffer,
    readFileSync: () => '0.12.0', appendFileSync: (_, text) => state.outputs.push(text),
    mkdtempSync: () => 'memory', tmpdir: () => 'memory', join: (...parts) => parts.join('/'), rmSync() {},
    repository: () => 'owner/repo', versionTag: version => `v${version}`,
    greenCommit: async () => true, verifyCodeQLBaseline: async () => {},
    api: async (path, options = {}) => {
      if(options.method === 'PATCH') { calls.push(path); release.draft = true; return {...release} }
      if(path === 'repos/owner/repo') return {default_branch:'main', private:false, fork:false}
      if(path.endsWith('/branches/main')) return {protected:true, commit:{sha:state.main}}
      if(path.includes('/git/ref/tags/')) return {object:{type:'commit', sha}}
      if(path.includes('/releases/')) return {...release}
      throw Error(`Unexpected API ${path}`)
    },
    pages: async path => { if(path.endsWith('/releases')) return [{...release}]; throw Error(`Unexpected pages ${path}`) },
    peelTagToCommit: async ref => ref.object.sha,
  }
  const context = vm.createContext({...sandbox, probe: state})
  vm.runInContext(source, context)
  const run = code => vm.runInContext(code, context)
  run('commonPublishedState = async () => ({source: "a".repeat(40), assets: []})')
  return {state, context, run}
}

test('execution changes and unavailable authorization become non-provenance errors', async () => {
  for(const setup of [
    f => { f.state.main = next },
    f => f.run('greenCommit = async () => false'),
    f => f.run('verifyCodeQLBaseline = async () => { throw Error("API unavailable") }'),
    f => f.run('verifyCodeQLBaseline = async () => { probe.main = "b".repeat(40) }'),
  ]) {
    const f = fixture(); setup(f)
    await assert.rejects(f.run('executionMain("owner/repo", process.env.RECOVERY_EXECUTION_SHA)'), {name:'RecoveryExecutionError'})
    assert.deepEqual(f.state.calls, [])
  }
})

test('preflight propagates execution failure through common, recovery and native fallback without writes', async () => {
  for(const failing of ['commonPublishedState','verifyRecoveryPublication','verifyNativePublication']) {
    const f = fixture()
    f.run('verifyRecoveryPublication = async () => { throw Error("not a recovery publication") }; verifyNativePublication = async () => ({id:1})')
    f.run(`${failing} = async () => { throw new RecoveryExecutionError(Error("main moved")) }`)
    await assert.rejects(f.run('preflight()'), {name:'RecoveryExecutionError'})
    assert.deepEqual(f.state.calls, [])
    assert.deepEqual(f.state.outputs, [])
  }
})

test('real native and recovery run loops do not swallow final execution errors', async () => {
  for(const kind of ['Native','Recovery']) {
    const f = fixture(), native = kind === 'Native'
    const workflow = native ? '.github/workflows/release.yml' : '.github/workflows/release-recover.yml'
    f.context.runRecord = {id:1, run_attempt:1, name:native?'Native release':'Recover pending release', path:workflow, head_branch:'main', head_sha:sha, event:'schedule', status:'completed', conclusion:'success', created_at:'2026-09-17T10:00:00Z'}
    f.context.steps = [
      'Run node scripts/release-publish.mjs metadata', 'Generate checksums',
      'Publish verified assets without overwriting an existing release', 'Attest canonical native release artifacts',
      'Attest rebuilt recovery artifacts', 'Publish rebuilt verified recovery assets',
    ].map(name => ({name, status:'completed', conclusion:'success', started_at:'2026-09-17T10:00:00Z', completed_at:'2026-09-17T10:10:00Z'}))
    f.run(`
      exactMainChecks = exactRequiredChecksBefore = assertPublishedSnapshot = async () => {};
      pages = async path => path.endsWith('/jobs')
        ? ['metadata','verify','audit-smoke','publish',...Array.from({length:5},(_,i)=>'binaries ('+i+')')].map(name=>({name,conclusion:'success',steps}))
        : [runRecord];
      executionMain = async () => { throw new RecoveryExecutionError(Error('main moved')) };
    `)
    await assert.rejects(f.run(`verify${kind}Publication('owner/repo', {published_at:'2026-09-17T10:05:00Z'}, '0.12.0', '${sha}', {}, {source:'${sha}', assets:[]})`), {name:'RecoveryExecutionError'})
    assert.deepEqual(f.state.calls, [])
  }
})

test('invalid provenance still withdraws with stable authorized main', async () => {
  const f = fixture()
  f.run('verifyRecoveryPublication = verifyNativePublication = async () => { throw Error("invalid provenance") }')
  await f.run('preflight()')
  assert.deepEqual(f.state.calls, ['repos/owner/repo/releases/12'])
  assert.equal(f.state.release.draft, true)
  assert.ok(f.state.outputs.includes('verified=false\n'))
})

test('main moving after provenance failure prevents withdrawal PATCH', async () => {
  const f = fixture()
  f.run('verifyRecoveryPublication = verifyNativePublication = async () => { probe.main = "b".repeat(40); throw Error("invalid provenance") }')
  await assert.rejects(f.run('preflight()'), {name:'RecoveryExecutionError'})
  assert.deepEqual(f.state.calls, [])
})

test('every target and ambiguous retry rechecks execution outside PATCH catch', async () => {
  for(const mode of ['next target','retry']) {
    const f = fixture()
    f.run(`api = async (path, options={}) => {
      if(options.method === 'PATCH') { probe.calls.push(path); probe.main = 'b'.repeat(40); throw Error('ambiguous write') }
      if(path === 'repos/owner/repo') return {default_branch:'main',private:false,fork:false};
      if(path.endsWith('/branches/main')) return {protected:true,commit:{sha:probe.main}};
      return {...probe.release};
    }`)
    const releases = mode === 'next target' ? '[{id:12},{id:13}]' : '[{id:12}]'
    await assert.rejects(f.run(`withdrawUnverified('owner/repo','v0.12.0',undefined,${releases})`), {name:'RecoveryExecutionError'})
    assert.deepEqual(f.state.calls, ['repos/owner/repo/releases/12'])
  }
})
