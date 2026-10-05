#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/project-env.sh"
exec /bin/bash "$@"
