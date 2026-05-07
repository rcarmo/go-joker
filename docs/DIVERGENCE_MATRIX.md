# Clojure parity divergence matrix

_Generated: 2026-05-07_

**222/222 pass** (100%), 0 fail, 0 error

## arithmetic (12/12)

| Test | Status | Expected | Got |
|---|---|---|---|
| add-ints | pass | `6` | `6` |
| add-mixed | pass | `3.0` | `3.0` |
| sub | pass | `7` | `7` |
| mul | pass | `12` | `12` |
| div-int | pass | `3.3333333333333335` | `3.3333333333333335` |
| rem | pass | `1` | `1` |
| mod | pass | `1` | `1` |
| inc | pass | `6` | `6` |
| dec | pass | `4` | `4` |
| max | pass | `5` | `5` |
| min | pass | `1` | `1` |
| abs | pass | `5` | `5` |

## atom (3/3)

| Test | Status | Expected | Got |
|---|---|---|---|
| atom-deref | pass | `42` | `42` |
| atom-swap | pass | `1` | `1` |
| atom-reset | pass | `42` | `42` |

## binding (4/4)

| Test | Status | Expected | Got |
|---|---|---|---|
| let | pass | `3` | `3` |
| let-destructure-vec | pass | `3` | `3` |
| let-destructure-map | pass | `3` | `3` |
| letfn | pass | `42` | `42` |

## comparison (12/12)

| Test | Status | Expected | Got |
|---|---|---|---|
| lt | pass | `true` | `true` |
| gt | pass | `true` | `true` |
| lte | pass | `true` | `true` |
| gte | pass | `true` | `true` |
| eq-num | pass | `true` | `true` |
| eq-str | pass | `true` | `true` |
| not-eq | pass | `true` | `true` |
| zero? | pass | `true` | `true` |
| pos? | pass | `true` | `true` |
| neg? | pass | `true` | `true` |
| even? | pass | `true` | `true` |
| odd? | pass | `true` | `true` |

## control (11/11)

| Test | Status | Expected | Got |
|---|---|---|---|
| if-true | pass | `1` | `1` |
| if-false | pass | `2` | `2` |
| if-nil | pass | `2` | `2` |
| when-true | pass | `42` | `42` |
| when-false | pass | `nil` | `nil` |
| cond | pass | `:b` | `:b` |
| case | pass | `:b` | `:b` |
| and | pass | `false` | `false` |
| or | pass | `42` | `42` |
| not | pass | `true` | `true` |
| do | pass | `3` | `3` |

## fn (10/10)

| Test | Status | Expected | Got |
|---|---|---|---|
| defn | pass | `25` | `25` |
| fn-literal | pass | `11` | `11` |
| apply | pass | `6` | `6` |
| comp | pass | `7` | `7` |
| partial | pass | `15` | `15` |
| identity | pass | `42` | `42` |
| constantly | pass | `42` | `42` |
| complement | pass | `true` | `true` |
| juxt | pass | `[13 72 3 6]` | `[13 72 3 6]` |
| memoize | pass | `25` | `25` |

## kw-sym (5/5)

| Test | Status | Expected | Got |
|---|---|---|---|
| keyword | pass | `:foo` | `:foo` |
| name | pass | `foo` | `foo` |
| namespace-kw | pass | `foo` | `foo` |
| symbol | pass | `foo` | `foo` |
| gensym | pass | `true` | `true` |

## list (6/6)

| Test | Status | Expected | Got |
|---|---|---|---|
| literal | pass | `(1 2 3)` | `(1 2 3)` |
| cons | pass | `(0 1 2 3)` | `(0 1 2 3)` |
| first | pass | `1` | `1` |
| rest | pass | `(2 3)` | `(2 3)` |
| count | pass | `3` | `3` |
| list? | pass | `true` | `true` |

## loop (2/2)

| Test | Status | Expected | Got |
|---|---|---|---|
| basic | pass | `10` | `10` |
| defn-recur | pass | `5050` | `5050` |

## macro (13/13)

| Test | Status | Expected | Got |
|---|---|---|---|
| when | pass | `(if true (do 1 2) nil)` | `(if true (do 1 2) nil)` |
| when-not | pass | `(if false nil 42)` | `(if false nil 42)` |
| or-empty | pass | `nil` | `nil` |
| and-empty | pass | `true` | `true` |
| ->thread | pass | `3` | `3` |
| ->>thread | pass | `15` | `15` |
| if-not | pass | `42` | `42` |
| doto | pass | `1` | `1` |
| if-let | pass | `42` | `42` |
| if-let-nil | pass | `-1` | `-1` |
| when-let | pass | `43` | `43` |
| when-first | pass | `1` | `1` |
| when-first-nil | pass | `nil` | `nil` |

## map (12/12)

| Test | Status | Expected | Got |
|---|---|---|---|
| literal | pass | `{:b 2, :a 1}` | `{:b 2, :a 1}` |
| assoc | pass | `2` | `2` |
| dissoc | pass | `{:b 2}` | `{:b 2}` |
| get | pass | `1` | `1` |
| get-default | pass | `42` | `42` |
| contains? | pass | `true` | `true` |
| keys | pass | `(:a :b)` | `(:a :b)` |
| vals | pass | `(1 2)` | `(1 2)` |
| merge | pass | `2` | `2` |
| select-keys | pass | `{:a 1, :c 3}` | `{:a 1, :c 3}` |
| count | pass | `2` | `2` |
| map? | pass | `true` | `true` |

## misc (8/8)

| Test | Status | Expected | Got |
|---|---|---|---|
| pr-str | pass | `[1 "two" :three]` | `[1 "two" :three]` |
| hash-map | pass | `{:b 2, :a 1}` | `{:b 2, :a 1}` |
| set-fn | pass | `#{1 2 3}` | `#{1 2 3}` |
| vec | pass | `[1 2 3]` | `[1 2 3]` |
| seq-fn | pass | `(1 2 3)` | `(1 2 3)` |
| not-empty | pass | `[1]` | `[1]` |
| not-empty-nil | pass | `nil` | `nil` |
| rand-int | pass | `true` | `true` |

## protocol (4/4)

| Test | Status | Expected | Got |
|---|---|---|---|
| defprotocol | pass | `false` | `false` |
| extend-dispatch | pass | `42` | `42` |
| satisfies?-true | pass | `true` | `true` |
| satisfies?-false | pass | `false` | `false` |

## reader (28/28)

| Test | Status | Expected | Got |
|---|---|---|---|
| quote | pass | `foo` | `foo` |
| deref-atom | pass | `42` | `42` |
| anonymous-fn | pass | `7` | `7` |
| set-literal | pass | `3` | `3` |
| regex | pass | `123` | `123` |
| char-literal | pass | `97` | `97` |
| char-newline | pass | `10` | `10` |
| char-space | pass | `32` | `32` |
| char-tab | pass | `9` | `9` |
| nil-literal | pass | `true` | `true` |
| true-literal | pass | `true` | `true` |
| false-literal | pass | `true` | `true` |
| keyword-ns | pass | `foo` | `foo` |
| symbol-ns | pass | `foo` | `foo` |
| vector-literal | pass | `true` | `true` |
| map-literal | pass | `true` | `true` |
| empty-list | pass | `true` | `true` |
| neg-number | pass | `-42` | `-42` |
| float-e | pass | `1000.0` | `1000.0` |
| hex-int | pass | `255` | `255` |
| octal-int | pass | `255` | `255` |
| ratio | pass | `0.3333333333333333` | `0.3333333333333333` |
| string-escape | pass | `3` | `3` |
| string-unicode | pass | `65` | `65` |
| meta-reader | pass | `true` | `true` |
| varquote | pass | `true` | `true` |
| comment | pass | `3` | `3` |
| nested-coll | pass | `42` | `42` |

## record (11/11)

| Test | Status | Expected | Got |
|---|---|---|---|
| defrecord-ctor | pass | `1` | `1` |
| defrecord-get | pass | `20` | `20` |
| defrecord-assoc | pass | `99` | `99` |
| defrecord-ext | pass | `3` | `3` |
| defrecord-count | pass | `2` | `2` |
| defrecord-eq | pass | `true` | `true` |
| defrecord-neq | pass | `false` | `false` |
| record? | pass | `true` | `true` |
| record?-no | pass | `false` | `false` |
| map-ctor | pass | `10` | `10` |
| dissoc-base | pass | `true` | `true` |

## regex (3/3)

| Test | Status | Expected | Got |
|---|---|---|---|
| re-find | pass | `123` | `123` |
| re-matches | pass | `123` | `123` |
| re-seq | pass | `[1 2 3]` | `[1 2 3]` |

## seq (34/34)

| Test | Status | Expected | Got |
|---|---|---|---|
| map | pass | `[2 3 4]` | `[2 3 4]` |
| filter | pass | `[2 4]` | `[2 4]` |
| reduce | pass | `10` | `10` |
| reduce-init | pass | `16` | `16` |
| take | pass | `[0 1 2]` | `[0 1 2]` |
| drop | pass | `[4 5]` | `[4 5]` |
| take-while | pass | `[1 2 3]` | `[1 2 3]` |
| drop-while | pass | `[3 4 5]` | `[3 4 5]` |
| concat | pass | `[1 2 3 4]` | `[1 2 3 4]` |
| mapcat | pass | `[1 1 2 4 3 9]` | `[1 1 2 4 3 9]` |
| sort | pass | `(1 2 3)` | `(1 2 3)` |
| sort-by | pass | `(b cc aaa)` | `(b cc aaa)` |
| reverse | pass | `[3 2 1]` | `[3 2 1]` |
| flatten | pass | `(1 2 3 4)` | `(1 2 3 4)` |
| distinct | pass | `[1 2 3]` | `[1 2 3]` |
| interleave | pass | `[1 :a 2 :b 3 :c]` | `[1 :a 2 :b 3 :c]` |
| interpose | pass | `[1 :x 2 :x 3]` | `[1 :x 2 :x 3]` |
| partition | pass | `[[1 2] [3 4]]` | `[[1 2] [3 4]]` |
| partition-all | pass | `[[1 2] [3 4] [5]]` | `[[1 2] [3 4] [5]]` |
| group-by | pass | `[2 4]` | `[2 4]` |
| frequencies | pass | `3` | `3` |
| zipmap | pass | `2` | `2` |
| range | pass | `[0 1 2 3 4]` | `[0 1 2 3 4]` |
| range-start-end | pass | `[2 3 4]` | `[2 3 4]` |
| repeat | pass | `[42 42 42]` | `[42 42 42]` |
| repeatedly | pass | `5` | `5` |
| iterate | pass | `[0 1 2 3 4]` | `[0 1 2 3 4]` |
| cycle | pass | `[1 2 3 1 2 3]` | `[1 2 3 1 2 3]` |
| every? | pass | `true` | `true` |
| some | pass | `true` | `true` |
| not-every? | pass | `true` | `true` |
| not-any? | pass | `true` | `true` |
| keep | pass | `[2 4]` | `[2 4]` |
| map-indexed | pass | `[[0 :a] [1 :b] [2 :c]]` | `[[0 :a] [1 :b] [2 :c]]` |

## set (6/6)

| Test | Status | Expected | Got |
|---|---|---|---|
| literal | pass | `3` | `3` |
| conj | pass | `true` | `true` |
| disj | pass | `#{1 3}` | `#{1 3}` |
| contains? | pass | `true` | `true` |
| count | pass | `3` | `3` |
| set? | pass | `true` | `true` |

## string (6/6)

| Test | Status | Expected | Got |
|---|---|---|---|
| str | pass | `hello world` | `hello world` |
| count-str | pass | `5` | `5` |
| subs | pass | `el` | `el` |
| string? | pass | `true` | `true` |
| char | pass | `A` | `A` |
| int-char | pass | `65` | `65` |

## transducer (4/4)

| Test | Status | Expected | Got |
|---|---|---|---|
| transduce-map | pass | `9` | `9` |
| transduce-filter | pass | `6` | `6` |
| transduce-take | pass | `[1 2]` | `[1 2]` |
| transduce-comp | pass | `6` | `6` |

## type (15/15)

| Test | Status | Expected | Got |
|---|---|---|---|
| nil? | pass | `true` | `true` |
| true? | pass | `true` | `true` |
| false? | pass | `true` | `true` |
| number? | pass | `true` | `true` |
| integer? | pass | `true` | `true` |
| float? | pass | `true` | `true` |
| keyword? | pass | `true` | `true` |
| symbol? | pass | `true` | `true` |
| fn? | pass | `true` | `true` |
| coll? | pass | `true` | `true` |
| seq? | pass | `true` | `true` |
| vector? | pass | `true` | `true` |
| sequential? | pass | `true` | `true` |
| associative? | pass | `true` | `true` |
| counted? | pass | `true` | `true` |

## vector (13/13)

| Test | Status | Expected | Got |
|---|---|---|---|
| literal | pass | `[1 2 3]` | `[1 2 3]` |
| conj | pass | `[1 2 3]` | `[1 2 3]` |
| nth | pass | `20` | `20` |
| first | pass | `1` | `1` |
| rest | pass | `[2 3]` | `[2 3]` |
| count | pass | `3` | `3` |
| empty?-no | pass | `false` | `false` |
| empty?-yes | pass | `true` | `true` |
| assoc | pass | `[1 99 3]` | `[1 99 3]` |
| into | pass | `[1 2 3]` | `[1 2 3]` |
| subvec | pass | `[2 3]` | `[2 3]` |
| peek | pass | `3` | `3` |
| pop | pass | `[1 2]` | `[1 2]` |

