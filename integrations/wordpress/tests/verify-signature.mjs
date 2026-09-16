import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import {createPublicKey, verify} from 'node:crypto'
const fixture = JSON.parse(readFileSync(process.argv[2], 'utf8'))
const envelope = fixture.envelope
const publicRaw = Buffer.from(fixture.keyring.keys[envelope.key_id], 'base64')
assert.equal(publicRaw.length, 32)
const key = createPublicKey({key: Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), publicRaw]), format: 'der', type: 'spki'})
const payload = Buffer.from(envelope.payload, 'base64url')
assert.ok(verify(null, payload, key, Buffer.from(envelope.signature, 'base64url')))
const lease = JSON.parse(payload)
assert.equal(lease.schema, 'darkphish-license-lease/v1')
assert.equal(lease.product, 'darkphish')
assert.equal(lease.edition, 'community')
assert.equal(lease.installation_id, fixture.installation_id)
assert.deepEqual(lease.entitlements, {managed_users: 100, active_campaigns: 1})
console.log('PHP Ed25519 envelope independently verified by Node.js.')
