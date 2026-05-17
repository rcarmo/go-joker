# Reader/parser extraction audit

Updated: 2026-05-17

## Current extracted reader package ownership

`core/reader` owns root-independent mechanics only:

- rune-window history and `RuneStream` `Get`/`Unget`/`Peek` position accounting;
- reader position-stack snapshots;
- line rune reading and raw file/buffer/buffered/IO wrappers;
- character, delimiter, whitespace, comment, dispatch, tagged-literal, top-level, read-form, conditional, namespaced-map, unquote, syntax-quote prefix, and symbolic-value classifiers;
- identifier scanning, literal classification, validation predicates/configuration, validation issue enumeration, and token validation;
- regex/string/comment/token scanning and string/unicode/octal escape decoding;
- number-token classification without concrete numeric object construction.

The package still does **not** import root `core` and must remain that way.

## Remaining root-bound reader wrapper

The former tiny `core/reader.go` wrapper has been folded into `core/read.go` so root reader ownership is not split across a standalone file. The remaining root wrapper responsibilities are still:

- root `Reader` embeds `reader.RuneStream`;
- root keeps filename interning through `STRINGS.Intern`;
- root converts underlying rune-read errors to `RT.NewError`.

Moving this wrapper completely requires a reader construction adapter for filename/string interning and error conversion, or accepting non-interned filename storage in `core/reader` plus a root wrapper for core errors.

## Remaining root-bound `core/read.go`

`read.go` still owns concrete object semantics and global reader behavior, but most construction now goes through `ReaderConstructionAdapter`:

- `ReadError`, root `ObjectInfo`, `MakeReadObject`, and `DeriveReadObject` adapter seams;
- `FORMAT_MODE`, `LINTER_MODE`, `DIALECT`, `SUPPRESS_READ`, `PROBLEM_COUNT`, and lint warning/error emission;
- number parsing into `Int`, `BigInt`, `BigFloat`, `Ratio`, `Double`, and `Boolean`/`Nil` literals through adapter seams;
- string/regex/character/comment object construction through adapter seams;
- list/vector/map/set construction via root reader/collection adapters;
- duplicate key/set reporting using root equality/printing semantics through root-owned adapter methods;
- metadata construction and application through root `Meta`/`ArrayMap` adapter seams;
- arg-literal and syntax-quote construction through root `Symbol`, `Seq`, `Vec`, `List`, namespace resolution, and `GLOBAL_ENV`;
- tagged literals, data-reader lookup, default data-reader calls, and reader conditional behavior;
- namespaced map auto-resolution through current namespace aliases and namespace usage flags;
- top-level read orchestration and conversion of reader/parser/eval exceptions.

Safe next extractions from this file are limited to pure decision/configuration helpers or further adapterization of explicitly root-owned semantics. Concrete reader orchestration remains blocked by namespace/tagged-literal/runtime side effects even though scalar/collection construction is now mostly adapterized.

## Remaining parser-adjacent `core/parse.go` blockers

`parse.go` is still evaluator/compiler front-end code rather than reader mechanics:

- `Callable` call helpers and dynamic argument dispatch;
- `LocalEnv`, `Bindings`, frame and closure/capture handling;
- `Expr` construction and parse-time namespace/var resolution;
- linter warnings/errors and position construction from root `Reader`;
- macro expansion, eval handoff, and global namespace state.

No parser orchestration should move to `core/reader` until `Object`, `Expr`, `Var`, namespace, metadata, and error construction seams are explicit and acyclic.

## Next safe reader steps

1. Continue extracting pure decision/configuration helpers from `read.go` when they do not mention root `Object`, `Symbol`, `Seq`, `Meta`, `GLOBAL_ENV`, or root collection types.
2. Continue extending `ReaderConstructionAdapter` only for stable root-owned semantics; do not move tagged literal or top-level orchestration while namespace/runtime side effects remain direct.
3. Keep package guards ensuring `core/reader` never imports root `core`.
