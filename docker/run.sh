#!/bin/sh
set -eu

exec /opt/darkphish/darkphish --config "${DARKPHISH_CONFIG:-/etc/darkphish/config.json}" "$@"
