#!/usr/bin/env bash
# Rendered Import Site with the real Chromium of this host.
#
# The regular test suite never starts a browser. This script prepares what the
# render path requires on Linux, a small tmpfs for the render profile and a
# delegated cgroup, and runs TestRenderedImportRealChromium against two public
# pages. Run it before a release that touches controllers/api/import_render*;
# the "Rendered import smoke" workflow runs it in CI.
#
# Usage, from anywhere inside the repository, as a user with sudo:
#   scripts/render-smoke.sh [go test flags]
# Set DARKPHISH_CHROMIUM_PATH to choose the browser and
# DARKPHISH_CHROMIUM_NO_SANDBOX=true where the Chromium sandbox cannot work.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [[ "$(uname -s)" != Linux ]]; then
  echo "render-smoke: Rendered Import Site runs on Linux only" >&2
  exit 2
fi
for tool in systemd-run systemctl findmnt go; do
  command -v "$tool" >/dev/null || { echo "render-smoke: $tool is required" >&2; exit 2; }
done

uid="$(id -u)"
gid="$(id -g)"
if [[ "$uid" -eq 0 ]]; then sudo=(); else sudo=(sudo); fi

root="${DARKPHISH_RENDER_PROFILE_ROOT:-/run/darkphish-render-smoke}"
chrome="${DARKPHISH_CHROMIUM_PATH:-}"
if [[ -z "$chrome" ]]; then
  for candidate in chromium chromium-browser google-chrome google-chrome-stable; do
    if command -v "$candidate" >/dev/null; then
      chrome="$(command -v "$candidate")"
      break
    fi
  done
fi
[[ -n "$chrome" ]] || { echo "render-smoke: no Chromium found; install one or set DARKPHISH_CHROMIUM_PATH" >&2; exit 2; }

unit="darkphish-render-smoke-$$"
mounted=""
created=""
cleanup() {
  # systemd-run only waits for the service. A running Chromium would keep the
  # profile root busy, so the service is killed and awaited first; killed
  # processes let go of the root a moment after the unit reports inactivity.
  "${sudo[@]}" systemctl kill --kill-whom=all --signal=SIGKILL "$unit" 2>/dev/null || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    unit_state="$("${sudo[@]}" systemctl is-active "$unit" 2>/dev/null || true)"
    [[ "$unit_state" != active && "$unit_state" != deactivating ]] && break
    sleep 0.5
  done
  if [[ -n "$mounted" ]]; then
    for _ in 1 2 3 4 5 6 7 8 9 10; do
      "${sudo[@]}" umount "$root" 2>/dev/null && break
      sleep 0.5
    done
    if findmnt -no FSTYPE --mountpoint "$root" >/dev/null 2>&1; then
      echo "render-smoke: unmount $root by hand" >&2
    fi
  fi
  if [[ -n "$created" ]]; then "${sudo[@]}" rmdir "$root" 2>/dev/null || true; fi
}
trap cleanup EXIT
# A handled signal returns from the wait below at once. Without a handler the
# shell would die on the spot and leave the service and the mount behind.
trap ':' INT TERM HUP

if [[ ! -d "$root" ]]; then
  "${sudo[@]}" mkdir -p "$root"
  created=1
fi
# /run itself is often a tmpfs, so look for a mount at the root itself, not at
# the filesystem that contains it.
if [[ "$(findmnt -no FSTYPE --mountpoint "$root" || true)" != tmpfs ]]; then
  "${sudo[@]}" mount -t tmpfs -o "size=96m,nr_inodes=4096,mode=0700,uid=$uid,gid=$gid" tmpfs "$root"
  mounted=1
fi

environment=(--setenv="HOME=$HOME" --setenv="PATH=$PATH"
  --setenv="DARKPHISH_RENDER_PROFILE_ROOT=$root" --setenv="DARKPHISH_CHROMIUM_PATH=$chrome")
if [[ -n "${DARKPHISH_CHROMIUM_NO_SANDBOX:-}" ]]; then
  environment+=(--setenv="DARKPHISH_CHROMIUM_NO_SANDBOX=$DARKPHISH_CHROMIUM_NO_SANDBOX")
fi

# A plain shell has no delegated cgroup; the transient unit provides one, the
# way the native service unit does.
"${sudo[@]}" systemd-run --quiet --wait --pipe --collect \
  --unit="$unit" --uid="$uid" --gid="$gid" \
  --property="Delegate=cpu memory pids" --property=ProtectControlGroups=no \
  --property=RestrictNamespaces=no --property="WorkingDirectory=$PWD" \
  "${environment[@]}" \
  go test -count=1 ./controllers/api/ -run '^TestRenderedImportRealChromium$' -v "$@" &
wait "$!"
