#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
fail=0

for test_script in "$SCRIPT_DIR"/*-tests.sh
do
    base="$(basename "$test_script")"
    if [[ $base != "all-tests.sh" ]]; then
        echo >&2 "RUNNING $base"
        "$test_script"
        if [[ $? != 0 ]]; then
            echo "${base//.sh/} failed."
            fail=1
        fi
    fi
done

if [[ $fail == 0 ]]; then
    echo >&2 "All tests passed."
fi

exit $fail
