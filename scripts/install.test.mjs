import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

const installer = fs.readFileSync(new URL('../install.sh', import.meta.url), 'utf8');

test('installer fails closed and targets the supported native layout', () => {
  assert.match(installer, /set -Eeuo pipefail/);
  assert.match(installer, /INSTALL_DIR="\/opt\/darkphish"/);
  assert.match(installer, /CONFIG_DIR="\/etc\/darkphish"/);
  assert.match(installer, /STATE_DIR="\/var\/lib\/darkphish"/);
  assert.match(installer, /dirty Git checkout|working tree is not clean/);
  assert.match(installer, /fresh-install only/);
  assert.match(installer, /systemd is required/);
});

test('installer verifies a pinned Go toolchain before building', () => {
  assert.match(installer, /GO_REQUIRED="1\.27\.1"/);
  assert.match(installer, /https:\/\/go\.dev\/dl\/go\$\{GO_REQUIRED\}/);
  assert.match(installer, /sha256sum --check --status/);
  assert.match(installer, /GOTOOLCHAIN=local/);
  assert.match(installer, /CGO_ENABLED=1/);
  assert.match(installer, /GOFLAGS='-mod=readonly'/);
});

test('installer creates an unprivileged hardened systemd service', () => {
  assert.match(installer, /User=\$\{APP_USER\}/);
  assert.match(installer, /NoNewPrivileges=true/);
  assert.match(installer, /ProtectSystem=strict/);
  assert.match(installer, /CapabilityBoundingSet=CAP_NET_BIND_SERVICE/);
  assert.match(installer, /AmbientCapabilities=CAP_NET_BIND_SERVICE/);
  assert.match(installer, /KillMode=control-group/);
});

test('installer generates production secrets without printing them', () => {
  assert.match(installer, /openssl rand -base64 32/);
  assert.match(installer, /"production_mode": true/);
  assert.match(installer, /session-auth\.key/);
  assert.match(installer, /session-encryption\.key/);
  assert.match(installer, /secrets\.key/);
  assert.match(installer, /audit-signing\.key/);
  assert.doesNotMatch(installer, /cat .*session-auth\.key/);
  assert.doesNotMatch(installer, /cat .*session-encryption\.key/);
  assert.doesNotMatch(installer, /cat .*secrets\.key/);
  assert.doesNotMatch(installer, /cat .*audit-signing\.key/);
});
