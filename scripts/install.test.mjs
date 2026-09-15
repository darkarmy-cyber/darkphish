import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

const installer = fs.readFileSync(new URL('../install.sh', import.meta.url), 'utf8');

test('installer fails closed and targets the supported native layout', () => {
  assert.match(installer, /set -Eeuo pipefail/);
  assert.match(installer, /INSTALL_DIR="\/opt\/darkphish"/);
  assert.match(installer, /CONFIG_DIR="\/etc\/darkphish"/);
  assert.match(installer, /STATE_DIR="\/var\/lib\/darkphish"/);
  assert.match(installer, /Git working-tree cleanliness could not be verified/);
  assert.match(installer, /working tree is not clean/);
  assert.match(installer, /fresh production installation only/);
  assert.match(installer, /systemd is required/);
  assert.match(installer, /cannot connect to the running systemd manager/);
  assert.match(installer, /systemd 245 or newer is required/);
  assert.match(installer, /path_entry_exists/);
  assert.match(installer, /SERVICE_DROPIN_ETC/);
  assert.match(installer, /SERVICE_DROPIN_RUN/);
});

test('installer pins the Go toolchain independently before executing it', () => {
  assert.match(installer, /GO_REQUIRED="1\.27\.1"/);
  assert.match(installer, /GO_SHA256_AMD64="63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445"/);
  assert.match(installer, /GO_SHA256_ARM64="3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec"/);
  assert.match(installer, /https:\/\/go\.dev\/dl\/go\$\{GO_REQUIRED\}/);
  assert.match(installer, /GO_EXPECTED_SHA256/);
  assert.match(installer, /sha256sum --check --status/);
  assert.match(installer, /env -i PATH="\$\{PATH\}" HOME="\/root" GOENV=off GOTOOLCHAIN=local go version/);
  assert.match(installer, /env -i PATH="\$\{PATH\}" HOME="\/root" GOENV=off GOTOOLCHAIN=local "\$\{GO_BIN\}" version/);
  assert.doesNotMatch(installer, /\.sha256"/);
});

test('installer builds only the committed source snapshot with isolated Git and Go state', () => {
  assert.match(installer, /git --no-replace-objects/);
  assert.match(installer, /-c core\.fsmonitor=false/);
  assert.match(installer, /git_source archive --format=tar/);
  assert.match(installer, /SOURCE_SNAPSHOT/);
  assert.match(installer, /env -i \\\s+PATH=/);
  assert.match(installer, /GOENV=off/);
  assert.match(installer, /GOWORK=off/);
  assert.match(installer, /GOTOOLCHAIN=local/);
  assert.match(installer, /GOFLAGS='-mod=readonly'/);
  assert.match(installer, /GOSUMDB='sum\.golang\.org'/);
  assert.match(installer, /CGO_ENABLED=1/);
  assert.doesNotMatch(installer, /GOAUTH=/);
});

test('installer sanitizes privileged helper environments', () => {
  assert.match(installer, /git_source\(\).*env -i/s);
  assert.match(installer, /safe_tar\(\).*env -i/s);
  assert.match(installer, /openssl_safe\(\).*env -i/s);
  assert.match(installer, /curl --disable/);
  assert.match(installer, /safe_tar -xzf/);
  assert.doesNotMatch(installer, /^\s*tar\s+-xzf/m);
  assert.doesNotMatch(installer, /openssl rand -base64 32/);
});

test('installer requires an executable private build filesystem before mutation', () => {
  assert.match(installer, /prepare_build_root/);
  assert.match(installer, /darkphish-install\.XXXXXX/);
  assert.match(installer, /exec-probe/);
  assert.match(installer, /no executable private temporary filesystem is available/);
  assert.match(installer, /refuse_existing_install\n    prepare_build_root\n\n    verify_source_tree\n    prepare_source_snapshot\n    install_system_packages/);
});

test('installer creates an unprivileged hardened systemd service and rejects overrides', () => {
  assert.match(installer, /User=\$\{APP_USER\}/);
  assert.match(installer, /NoNewPrivileges=true/);
  assert.match(installer, /ProtectSystem=strict/);
  assert.match(installer, /ProtectClock=true/);
  assert.match(installer, /CapabilityBoundingSet=CAP_NET_BIND_SERVICE/);
  assert.match(installer, /AmbientCapabilities=CAP_NET_BIND_SERVICE/);
  assert.match(installer, /KillMode=control-group/);
  assert.match(installer, /DropInPaths/);
  assert.match(installer, /unexpected systemd drop-ins/);
  assert.match(installer, /FragmentPath/);
});

test('installer requires stable readiness from both listeners and bootstrap completion', () => {
  assert.match(installer, /https:\/\/127\.0\.0\.1:3333\/readyz/);
  assert.match(installer, /http:\/\/127\.0\.0\.1:80\//);
  assert.match(installer, /--noproxy '\*'/);
  assert.match(installer, /--cacert/);
  assert.match(installer, /darkphish_initial_admin_password/);
  assert.match(installer, /ready_streak >= 5/);
  assert.match(installer, /MainPID/);
  assert.match(installer, /both admin and simulation listeners/);
});

test('installer rolls back only artifacts it created on failure', () => {
  assert.match(installer, /INSTALL_COMMITTED=0/);
  assert.match(installer, /rollback_install/);
  assert.match(installer, /SERVICE_FILE_CREATED/);
  assert.match(installer, /APP_PATHS_CREATED/);
  assert.match(installer, /APP_USER_CREATED/);
  assert.match(installer, /APP_GROUP_CREATED/);
  assert.match(installer, /systemctl disable --now/);
  assert.match(installer, /INSTALL_COMMITTED=1/);
});

test('installer generates production secrets without printing them', () => {
  assert.match(installer, /openssl_safe rand -base64 32/);
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

test('installer uses OpenSSL configuration compatible with supported command lines', () => {
  assert.match(installer, /openssl-admin\.cnf/);
  assert.match(installer, /subjectAltName = @alt_names/);
  assert.match(installer, /-extensions v3_req/);
  assert.doesNotMatch(installer, /-addext/);
});
