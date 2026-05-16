# Reader/parser extraction audit

Updated: 2026-05-16

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

## Remaining root-bound `core/reader.go`

`core/reader.go` is now a thin root adapter:

- root `Reader` embeds `reader.RuneStream`;
- root keeps filename interning through `STRINGS.Intern`;
- root converts underlying rune-read errors to `RT.NewError`.

Moving this file completely requires a reader construction adapter for filename/string interning and error conversion, or accepting non-interned filename storage in `core/reader` plus a root wrapper for core errors.

## Remaining root-bound `core/read.go`

`read.go` still owns concrete object construction and global reader behavior:

- `ReadError`, root `ObjectInfo`, `MakeReadObject`, and `DeriveReadObject`;
- `FORMAT_MODE`, `LINTER_MODE`, `DIALECT`, `SUPPRESS_READ`, `PROBLEM_COUNT`, and lint warning/error emission;
- number parsing into `Int`, `BigInt`, `BigFloat`, `Ratio`, `Double`, and `Boolean`/`Nil` literals;
- string/regex/character object construction;
- list/vector/map/set construction via root collection adapters;
- duplicate key/set reporting using root equality/printing semantics;
- metadata construction and application through root `Meta`/`ArrayMap`;
- arg-literal and syntax-quote construction through root `Symbol`, `Seq`, `Vec`, `List`, namespace resolution, and `GLOBAL_ENV`;
- tagged literals, data-reader lookup, default data-reader calls, and reader conditional behavior;
- namespaced map auto-resolution through current namespace aliases and namespace usage flags;
- top-level read orchestration and conversion of reader/parser/eval exceptions.

Safe next extractions from this file are limited to pure decision/configuration helpers. Concrete read-number/string/symbol parsing remains blocked until a root adapter can construct objects and errors.

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
2. Define a `ReaderConstructionAdapter` extension for root-owned object/error construction before moving read-number, read-string, read-symbol, tagged literal, or top-level orchestration code.
3. Keep package guards ensuring `core/reader` never imports root `core`.
