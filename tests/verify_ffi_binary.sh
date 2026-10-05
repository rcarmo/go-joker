#!/usr/bin/env bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/scripts/project-env.sh" || exit 1
# Exercise the normal distributed CLI, not a feature-tagged build.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
binary=${1:?binary is required}
binary=$(cd "$(dirname "$binary")" && pwd)/$(basename "$binary")
metadata=$(go version -m "$binary")
grep -Fq $'\tbuild\tCGO_ENABLED=0' <<<"$metadata" || { echo 'binary must be built without cgo' >&2; exit 1; }
grep -Fq $'\tdep\tgithub.com/ebitengine/purego\t' <<<"$metadata" || { echo 'binary is missing default FFI backend' >&2; exit 1; }

case $(go env GOOS) in
 linux)
   library=${JOKER_FFI_SMOKE_LIBRARY:-}
   if [[ -z $library ]]; then
     case $(go env GOARCH) in amd64) triplet=x86_64-linux-gnu ;; arm64) triplet=aarch64-linux-gnu ;; *) echo 'unsupported smoke architecture' >&2; exit 1 ;; esac
     for path in /lib/$triplet/libc.so.6 /usr/lib/$triplet/libc.so.6 /lib64/libc.so.6 /lib/libc.so.6; do
       if [[ -f $path ]]; then library=$path; break; fi
     done
   fi
   ;;
 darwin) library=${JOKER_FFI_SMOKE_LIBRARY:-/usr/lib/libSystem.B.dylib} ;;
 windows) library=${JOKER_FFI_SMOKE_LIBRARY:-${WINDIR:?}/System32/msvcrt.dll} ;;
 *) echo 'FFI runtime smoke unavailable on this platform' >&2; exit 1 ;;
esac
[[ -n $library ]] || { echo 'Set JOKER_FFI_SMOKE_LIBRARY to an absolute C runtime path' >&2; exit 1; }
profile_root=${PROFILE_ROOT:-$root/.cache/test-profiles/ffi-binary}
mkdir -p "$profile_root"
run=$(mktemp -d "$profile_root/run-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")
run=$(cd "$run" && pwd)
cp "$binary" "$run/joker.bin"
printf 'library=%s\nCGO_ENABLED=0\nheap_sampling=524288 bytes\ncpu_sampling=100Hz\n' "$library" > "$run/command.txt"
status=0
JOKER_FFI_SMOKE_LIBRARY="$library" "$run/joker.bin" --cpuprofile "$run/cpu.pprof" --memprofile "$run/heap.pprof" "$root/tests/ffi_smoke.joke" > "$run/test.log" 2>&1 || status=$?
cat "$run/test.log"
analysis=0
for metric in alloc_space alloc_objects; do
  go tool pprof -top "-$metric" "$run/joker.bin" "$run/heap.pprof" > "$run/$metric.txt" 2>&1 || analysis=1
done
go tool pprof -top -cum "$run/joker.bin" "$run/cpu.pprof" > "$run/cpu.txt" 2>&1 || analysis=1
printf 'test_exit=%s\nanalysis_exit=%s\n' "$status" "$analysis" > "$run/status.txt"
((status==0 && analysis==0)) || { echo "FFI smoke failed; profiles/logs: $run" >&2; exit 1; }
echo "verified default FFI in $binary; profiles: $run"
