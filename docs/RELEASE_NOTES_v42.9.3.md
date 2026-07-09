# Release Notes — v42.9.3

This patch release fixes regressions found by the full post-audit race and benchmark validation of v42.9.2.

## Runtime correctness

- Made string interning safe under concurrent use while preserving the map representation required by generated bootstrap payloads.
- Added concurrent regression coverage that verifies identical strings retain pointer identity.

## Documentation

- Added private metadata to internal tagged-literal readers.
- Added documentation metadata to the public tagged-literal reader vars.
- Restored warning-free documentation generation and pre-tag validation.

## Validation

The release passed:

- `make pretag-check`
- `make race`
- benchmark documentation validation and benchmark-runner tests
- the CI performance guard with three one-second samples per selected benchmark
- every Go benchmark across all packages with `-benchtime=1x -count=3`

The final benchmark run completed 216 benchmark samples, and the focused string-pool race regression passed 100 shuffled iterations.
