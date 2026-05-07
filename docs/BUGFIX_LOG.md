# BUGFIX_LOG

## 2026-05-07 audit remediation batch

### 1) `std/runtime`: benchmark helper could effectively hang
- **Area:** `std/runtime/runtime_native.go` (`procBenchmark`)
- **Symptom:** `TestBenchmarkFn` could run until package test timeout.
- **Fix:** bounded calibration loop with target window + max iteration cap.
- **Regression coverage:** `std/runtime/runtime_test.go::TestBenchmarkFn` now enforces timely completion and validates result shape.

### 2) `core/read`: conditional reader state corruption + empty-splice panic
- **Area:** `core/read.go` (`readCondList`, `readMulti`)
- **Symptom A:** shadowed `forms` variable in conditional reader.
- **Symptom B:** runtime panic (`index out of range`) when splice expansion produced zero forms.
- **Fix:** removed shadowing; `readMulti` now loops until queue has an item or a non-multi object is read.
- **Regression coverage:**
  - `core/read_conditional_test.go::TestReadConditionalSpliceEmptyInList`
  - `core/read_conditional_test.go::TestReadConditionalNestedSpliceNoRuntimePanic`

### 3) `core/noescape`: unsafe pointer misuse warning
- **Area:** `core/noescape.go`
- **Symptom:** `go vet` flagged unsafe pointer pattern.
- **Fix:** replaced with identity helper implementation (`noescape64` returns input slice).
- **Regression coverage:** existing `core/ir_optimization_test.go::TestNoescape64Correctness` remains valid.

### 4) `core/deps`: unchecked filesystem errors
- **Area:** `core/deps.go`
- **Symptom:** unchecked `MkdirAll`; `defer out.Close()` before `err` check.
- **Fix:** explicit `PanicOnErr` for `MkdirAll`; corrected `os.Create` error handling order.

### 5) `std/bolt`: ignored `Update`/`View` errors
- **Area:** `std/bolt/bolt_native.go`
- **Symptom:** DB-level errors (e.g. closed DB) could be silently ignored.
- **Fix:** capture and check return values from `Update`/`View` and `NextSequence`.
- **Regression coverage:**
  - `std/bolt/bolt_native_test.go::TestCreateBucketClosedDBPanics`
  - `std/bolt/bolt_native_test.go::TestPutClosedDBPanics`

### 6) `std/filepath`: ignored `filepath.Walk` error
- **Area:** `std/filepath/filepath_native.go`
- **Symptom:** missing root path could return partial/empty result without surfacing error.
- **Fix:** collect and panic on walk error.
- **Regression coverage:**
  - `std/filepath/filepath_native_test.go::TestFileSeqMissingRootPanics`
  - `std/filepath/filepath_native_test.go::TestFileSeqReturnsEntries`

### 7) `std/pdf`: ignored drawing/font/image errors
- **Area:** `std/pdf/pdf_native.go`
- **Symptom:** return errors from gopdf calls were dropped.
- **Fix:** explicit error checks and RT errors for `SetFontSize`, `Cell`, `MultiCell`, `Image`.
- **Regression coverage:** `std/pdf/pdf_test.go::TestImageMissingPathPanics`

### 8) Deprecated API cleanup (`ioutil`)
- **Areas:** `core/procs.go`, `std/http/http_native.go`, `std/os/os_native.go`, `std/os/a_os.go`
- **Fix:** migrated to `os.ReadFile`, `io.ReadAll`, `os.ReadDir`, `os.CreateTemp`, `os.MkdirTemp`.

### 9) Analyzer-driven cleanup
- **Areas:** `core/escape_analysis.go`, `core/format.go`, `core/ir_exec.go`, `core/ir_typed_exec.go`, `core/wasm_host.go`, `core/wasm_mem_nth.go`
- **Fixes:** removed empty/unreachable/ineffectual branches and assignments flagged by SA checks.

### 10) Security dependency updates
- **Module/toolchain changes:**
  - `github.com/go-git/go-git/v5` → `v5.17.1`
  - `golang.org/x/image` → `v0.39.0`
  - toolchain directive → `go1.25.9`
- **Result:** `govulncheck ./...` reports **No vulnerabilities found**.
