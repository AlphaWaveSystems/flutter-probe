---
title: ProbeScript Grammar (EBNF)
description: The complete formal grammar of ProbeScript in ISO-style EBNF - lexical rules, layout, every statement, selector, modifier and block form the parser accepts - with a runnable example for each production.
---

This page is the normative grammar of ProbeScript. It was derived from the parser source
(`internal/parser/lexer.go`, `token.go`, `parser.go`, `ast.go`), not from the other docs, and it is
kept honest by a conformance test (`internal/parser/grammar_test.go`) that:

- parses every code block on this page that is tagged `probe`,
- parses at least one example for every statement production defined below,
- fails when a keyword, compound keyword or filler word is added to or removed from `token.go`
  without this page being updated.

The [Syntax](/probescript/syntax/) and [Dictionary](/probescript/dictionary/) pages are the friendly
reference; where they disagree with the parser, see [Where the docs and the parser disagree](#where-the-docs-and-the-parser-disagree).

## Notation

The grammar uses ISO/IEC 14977 style EBNF.

| Symbol | Meaning |
|---|---|
| `name = ... ;` | production; every production ends with `;` |
| `a , b` | concatenation (a followed by b) |
| `a \| b` | alternation |
| `[ a ]` | optional |
| `{ a }` | zero or more repetitions |
| `( a )` | grouping |
| `"text"` | terminal; a keyword or word, matched **case-insensitively** (except where noted) |
| `'"'` | a terminal containing a double quote |
| `a - b` | `a` except anything that matches `b` |
| `? ... ?` | special sequence: an informal description of a character or token class |
| `(* ... *)` | comment |

Two levels are described. Section 1 turns characters into tokens. Sections 2 onward are written over
tokens. The token classes are `STRING`, `INT`, `FLOAT`, `ORDINAL`, `ID`, `TAG`, `COLON`, `PUNCT`, `WORD`,
`NEWLINE`, `INDENT` and `DEDENT`. A quoted terminal in a syntactic production denotes the keyword token
with that spelling, or - for words that are not in the keyword table, such as `"native"` or `"visible"` -
a `WORD` token with that spelling. A terminal with spaces, such as `"go back"`, is a
[compound keyword](#compound-keywords) and is a single token.

The parser is a hand-written recursive-descent parser that dispatches on the first token of a line.
Where two alternatives overlap, **the alternative listed first wins** (ordered choice); the
[dispatch table](#statement-dispatch) gives the exact order.

## 1. Lexical grammar

### Characters, lines and comments

```ebnf
letter      = ? any Unicode letter ? ;
digit       = ? any Unicode decimal digit ? ;
TAB         = ? U+0009 ? ;
LF          = ? U+000A ? ;
CR          = ? U+000D ? ;
line-break  = CR , LF | LF | CR ;
not-eol     = ? any character except CR and LF ? ;
blank       = " " | TAB ;

source      = { source-line } ;
source-line = indent , [ comment | token-run ] , ( line-break | ? end of input ? ) ;
indent      = { blank } ;                       (* width = number of spaces + 4 per TAB *)
comment     = "#" , { not-eol } ;               (* a line whose first non-blank character is "#" is always a comment *)
token-run   = { blank | lexeme } , [ inline-comment ] ;
inline-comment = "#" , { not-eol } ;            (* only where the "#" is NOT followed by a letter or "_" ; otherwise it starts an ID *)
```

A line that is empty, blank or comment-only produces no tokens at all. Every other line produces its
tokens followed by one `NEWLINE`.

### Lexemes

```ebnf
lexeme  = STRING | FLOAT | ORDINAL | INT | ID | TAG | COLON | PUNCT | identifier | ignored ;
          (* FLOAT, ORDINAL and INT all start with a digit: the longest form wins, tried in that order *)

STRING  = '"' , { string-char } , '"' ;
string-char = ? any character except '"', "\" and LF ? | escape ;
escape  = "\" , ( '"' | "n" | "t" | ? any other character, which stands for itself ? ) ;

INT     = digit , { digit } ;
FLOAT   = digit , { digit } , "." , digit , { digit } ;
ORDINAL = digit , { digit } , ( "st" | "nd" | "rd" | "th" ) ;   (* lower case, not followed by another letter *)

ID      = "#" , ( letter | "_" ) , { letter | digit | "_" } ;
TAG     = "@" , { letter | digit | "_" } ;
COLON   = ":" ;
PUNCT   = "(" | ")" | "," | "-" ;

identifier = ( letter | "_" | "'" ) ,
             { letter | digit | "_" | "'" | "-" , ( letter | digit ) } ;
             (* a hyphen is part of a word only when a letter or digit follows it: "looking-for" *)

ignored = ? any other character except "<": it is silently dropped, e.g. / . = ; { } [ ] > ! * + $ ? ;
          (* an unquoted "<" outside a string is a lexical error: write "<email>" with quotes *)
```

An `identifier` whose lower-cased spelling is in the [keyword table](#keywords) becomes a keyword
token; any other identifier becomes a `WORD`. `PUNCT`, `TAG` and the four listed words
(`verify`, `set`, `location`, `clipboard`) are also `WORD`-class tokens. Keywords are
case-insensitive (`TAP "x"` is `tap "x"`).

```ebnf
WORD = identifier - keyword ;
```

Strings cannot contain a raw line break. Placeholders such as `"<email>"` are ordinary string content;
see [Placeholders and variables](#placeholders-and-variables).

### Layout: NEWLINE, INDENT, DEDENT

```ebnf
(* For every token-run, compare its indent width w with the top s of the indent stack (initially 0):
     w > s : push w and emit INDENT
     w < s : pop while w < top of stack, emitting one DEDENT per pop (a width that matches no
             enclosing level is NOT an error: it is treated as the next lower level)
     w = s : nothing
   then emit the line's tokens and one NEWLINE.
   At end of input emit one DEDENT per open level, then EOF. *)
```

```ebnf
block = INDENT , { statement | NEWLINE } , DEDENT ;
body  = [ block ] ;          (* a header line is followed by NEWLINE; a missing block is an empty body *)
```

### Keywords

```ebnf
keyword = "test" | "recipe" | "use" | "before" | "after" | "on" | "open" | "tap" | "type"
        | "see" | "assert" | "wait" | "swipe" | "scroll" | "clear" | "close" | "drag"
        | "pinch" | "rotate" | "toggle" | "shake" | "press" | "if" | "otherwise" | "else"
        | "repeat" | "retry" | "times" | "optional" | "for" | "each" | "when" | "respond"
        | "the" | "a" | "an" | "in" | "into" | "at" | "of" | "to" | "from" | "is" | "are"
        | "that" | "this" | "it" | "with" | "as" | "and" | "until" | "appears" | "disappears"
        | "enabled" | "disabled" | "checked" | "contains" | "exactly" | "button" | "field"
        | "app" | "page" | "network" | "idle" | "load" | "seconds" | "second" | "dart" | "run"
        | "get" | "post" | "put" | "delete" | "body" | "examples" | "matching" | "between"
        | "take" | "compare" | "screenshot" | "called" | "dump" | "widget" | "tree" | "save"
        | "device" | "logs" | "pause" | "log" | "back" | "home" | "failure" | "restart"
        | "allow" | "deny" | "grant" | "revoke" | "permission" | "permissions" | "all"
        | "kill" | "copy" | "paste" | "call" | "below" | "above" | "left" | "right"
        | "focused" | "link" | "animations" | "animation" | "store" | "composite" | "sync"
        | "biometric" | "enroll" | "deliver" | "signal" | "read" | "travel" | "over" ;

listed-word = "verify" | "set" | "location" | "language" | "clipboard" ;
              (* in the keyword table, but they lex as plain WORD tokens *)
```

`"else"` is an alias of `"otherwise"`, `"animation"` of `"animations"`, `"permissions"` of `"permission"`.
Because every `keyword` is reserved, a keyword cannot be used where a plain `WORD` is required (a recipe
parameter, a `store ... as` variable, a type selector); quote it or choose another word. Keywords are
fine inside recipe-call names and inside strings. A number of keywords (`press`, `pinch`, `between`,
`home`, `back`, `load`, `widget`, ...) have no built-in statement of their own; a line starting with
one is a [recipe call](#recipe-calls).

### Compound keywords

The lexer joins these word sequences, separated by one or more spaces, into a single token before
keyword lookup:

```ebnf
compound-keyword = "don't see" | "dont see" | "go back" | "press enter" | "long press" | "double tap"
                 | "before all tests" | "after all tests" | "before all" | "after all"
                 | "before each test" | "after each test" | "before each" | "after each" | "on failure" | "with examples"
                 | "clear app data" | "grant all permissions" | "revoke all permissions"
                 | "allow permission" | "deny permission" | "set location" | "set language"
                 | "verify external browser" | "add media" ;
```

`"dont see"` (no apostrophe) is an alias of `"don't see"`. Matching is greedy in the order above, so
`before all tests` wins over `before all`.

### Filler words

Filler words are skipped by the forgiving parser at fixed positions (the start of a statement and
between operands). They make steps read like English: `tap the "Login" button` is
meant to read the same as `tap "Login"`.

```ebnf
filler  = "the" | "a" | "an" | "on" | "in" | "into" | "at" | "of" | "from" | "is" | "are"
        | "that" | "this" | "it" | "for" | "to" ;
fillers = { filler } ;
```

Fillers are only skipped where a production below writes `fillers`; they are not stripped
everywhere, and a few of them are given meaning by the production that follows
(`"in"` in a positional selector, `"of"` in `left of`, `"into"` in `read ... into`).

## 2. File structure

```ebnf
program      = { top-item } ;
top-item     = use-decl | recipe-def | test-def | composite-def | hook-def | NEWLINE | skipped-token ;
skipped-token = ? any other token at file level (for example a stray statement); it is ignored, not an error ? ;

text-operand = STRING | WORD | ID ;      (* a "string-ish" operand: quoted, a bare word, or an #id *)
```

### Imports

```ebnf
use-decl = "use" , text-operand , NEWLINE ;
```

```probe
use "recipes/auth.probe"

test "file imports a recipe file"
  open the app
  see "Welcome"
```

### Recipes

```ebnf
recipe-def = "recipe" , text-operand , [ param-list ] , NEWLINE , body ;
param-list = "(" , [ param , { [ "," ] , param } ] , ")" ;
param      = WORD | STRING ;
```

```probe
recipe "sign in as" (email, password)
  type "<email>" into "Email"
  type "<password>" into "Password"
  tap "Continue"
  see "Dashboard"

test "calls a recipe with parameters"
  sign in as "user@test.com" with "secret123"
```

A recipe without parameters:

```probe
recipe "dismiss onboarding"
  if "Skip" appears
    tap "Skip"
  wait for the page to load

test "calls a recipe without parameters"
  dismiss onboarding
  see "Home"
```

### Tests, tags and data tables

```ebnf
test-def  = "test" , text-operand , test-head , [ examples-block ] ;
test-head = TAG , { TAG } , NEWLINE , body                                   (* tags on the header line *)
          | NEWLINE , INDENT , TAG , { TAG } , NEWLINE , { statement | NEWLINE } , DEDENT
                                                                              (* tags as the first body line *)
          | NEWLINE , body ;

examples-block = ( "with examples" | "examples" ) , [ COLON ] ,
                 ( "from" , fillers , [ STRING ] , NEWLINE                    (* external CSV file *)
                 | NEWLINE , examples-table ) ;
examples-table = INDENT , example-row , { example-row } , DEDENT ;
example-row    = example-cell , { example-cell } , NEWLINE ;
example-cell   = ? any token except NEWLINE and DEDENT; STRING cells contribute their unquoted content ? ;
```

The first row of an inline table holds the column names; every following row is one run of the test.
The examples block must directly follow the test it belongs to.

```probe
test "test with a name only"
  open the app
```

```probe
test "tags on the header line" @smoke @critical
  open the app
  see "Home"
```

```probe
test "tags on the first body line"
  @regression @ui
  open the app
  see "Home"
```

```probe
test "login validation"
  type "<email>" into "Email"
  type "<password>" into "Password"
  tap "Continue"
  see "<expected>"

with examples:
  email             password    expected
  "user@test.com"   "pass123"   "Dashboard"
  ""                "pass123"   "Email is required"
```

```probe
test "login with CSV data"
  type "<email>" into "Email"
  tap "Sign In"
  see "<expected>"

with examples from "fixtures/users.csv"
```

### Hooks

```ebnf
hook-def  = hook-head , NEWLINE , body ;
hook-head = "before all tests" | "before all" | "after all tests" | "after all"
          | "before each test" | "after each test" | "before each" | "after each" | "on failure"
          | ( "before" | "after" ) , fillers ;       (* bare form: a before-each / after-each hook *)
```

`before each` and `after each` (with or without `test`) are the same hook; `before all` / `after all`
likewise. Before 0.16.7 the forms without `test` were accepted but their body was silently discarded.

```probe
before all tests
  open the app
  tap "Accept Terms"

before each test
  see "Home"

after each test
  take screenshot "after_test"

after all tests
  log "suite done"

on failure
  take screenshot "failure_state"
  save logs
  dump tree

test "hooks wrap this test"
  tap "Settings"
  see "Account"
```

The short suite forms:

```probe
before all
  open the app

after all
  log "done"

test "short suite hooks"
  see "Home"
```

### Composite (multi-device) tests

```ebnf
composite-def = "composite" , [ "test" ] , text-operand , { TAG } , NEWLINE ,
                [ INDENT , [ devices-block ] , { composite-item | NEWLINE } , DEDENT ] ;
devices-block = "devices" , [ COLON ] , NEWLINE , [ INDENT , { device-decl | NEWLINE } , DEDENT ] ;
device-decl   = alias , COLON , { ? any token except NEWLINE ? } , NEWLINE ;   (* the rest is the device target *)
composite-item = sync-step | device-block | skipped-token ;
device-block  = alias , COLON , NEWLINE , [ INDENT , { statement | NEWLINE } , DEDENT ] ;
alias         = ? any single token directly followed by COLON ? ;
sync-step     = "sync" , [ text-operand ] , NEWLINE ;
```

Steps in a composite test are only recognised inside an `alias:` block (and `sync` between blocks).
Anything else at composite level is skipped.

```probe
composite test "alice messages bob" @smoke
  devices:
    A: iPhone 15 Simulator
    B: Pixel 9 Emulator
  A:
    open the app
    tap "Sign in as Alice"
  B:
    open the app
    tap "Sign in as Bob"
  sync "both signed in"
  A:
    tap "New message"
    type "hello bob" into "Message"
    tap "Send"
  B:
    wait until "hello bob" appears
```

## 3. Statements

```ebnf
statement  = fillers , ( system-dialog-step | line-step ) , NEWLINE
           | fillers , block-step ;

line-step  = tap-native-step | tap-step | type-native-step | type-step
           | long-press-step | double-tap-step | clear-step | toggle-step | drag-step
           | swipe-step | scroll-step
           | see-native-step | see-step | dont-see-step | assert-defects-step
           | wait-step
           | open-app-step | open-link-step | close-step | restart-step | kill-step | clear-data-step
           | go-back-step | press-enter-step | shake-step | pause-step | log-step | rotate-step
           | permission-step | grant-revoke-step
           | copy-step | paste-step | set-location-step | verify-browser-step | add-media-step
           | take-screenshot-step | compare-screenshot-step | dump-tree-step | save-logs-step
           | store-step | read-ai-step | deliver-signal-step
           | biometric-step | enroll-biometric-step
           | http-call-step | recipe-call-step ;

block-step = if-step | repeat-step | retry-step | dart-step | mock-step | travel-step ;

wait-step  = wait-until-step | wait-any-step | wait-idle-step | wait-animations-step
           | wait-network-step | wait-duration-step | wait-id-step | wait-page-step ;

any-token  = ? any token except NEWLINE and end of input ? ;
rest-of-line = { any-token } ;
```

### Statement dispatch

The parser reads the first token of a statement (after leading fillers) and chooses as follows.
The first matching row wins.

| First token / condition | Production |
|---|---|
| line contains `system dialog`, `system field` or `sign in sandbox tester` (outside strings) | `system-dialog-step` |
| `open` followed by `app` or `link` | `open-app-step`, `open-link-step` |
| `open` followed by anything else | `recipe-call-step` (a recipe whose name starts with "open") |
| `tap` followed by the word `native` | `tap-native-step`; otherwise `tap-step` |
| `type` followed by the word `native` | `type-native-step`; otherwise `type-step` |
| `see` / `don't see` / `dont see` followed by the word `native` | `see-native-step`; otherwise `see-step` / `dont-see-step` |
| `assert` | `assert-defects-step` |
| `wait` | `wait-step` (see below for its order) |
| `swipe`, `scroll`, `long press`, `double tap`, `drag`, `toggle` | the step of the same name |
| `clear` followed by a bare `WORD` | `recipe-call-step` (a recipe whose name starts with "clear") |
| `clear` otherwise | `clear-step` |
| `clear app data` | `clear-data-step` |
| `close`, `restart`, `kill`, `go back`, `press enter`, `shake`, `pause`, `log`, `rotate` | the step of the same name |
| `allow`, `deny`, `allow permission`, `deny permission` | `permission-step` |
| `grant`, `revoke`, `grant all permissions`, `revoke all permissions` | `grant-revoke-step` |
| `copy`, `paste`, `set location`, `set language`, `verify external browser`, `add media` | the step of the same name |
| `take`, `compare`, `dump`, `save` | `take-screenshot-step`, `compare-screenshot-step`, `dump-tree-step`, `save-logs-step` |
| `store`, `read`, `deliver` | `store-step`, `read-ai-step`, `deliver-signal-step` |
| `biometric`, `enroll` | `biometric-step`, `enroll-biometric-step` |
| `call` | `http-call-step` |
| `if`, `repeat`, `retry`, `run`, `when`, `travel` | `if-step`, `repeat-step`, `retry-step`, `dart-step`, `mock-step`, `travel-step` |
| an empty line (NEWLINE) | nothing (ignored) |
| anything else, including `press`, `pinch`, `sync` outside composite tests, unknown words | `recipe-call-step` |

Inside `wait-step` the order is: `until` (then `any`, then `network`, `idle`, otherwise a text target),
`idle`, `animations`, `network`, `page`, a number, an `#id`, and finally the catch-all `wait-page-step`.

### Selectors

```ebnf
selector         = fillers , ( ordinal-selector | id-selector | text-selector | type-selector ) ;
target-selector  = id-selector | text-selector ;      (* the only forms accepted by type, swipe, scroll and "until" *)

id-selector      = ID ;
text-selector    = STRING , [ positional-suffix | relational-suffix ] ;
positional-suffix = "in" , fillers , STRING ;                       (* "Price" in "Product Card" *)
relational-suffix = ( "below" | "above" | ( "left" | "right" ) , [ "of" ] ) , fillers , [ STRING ] ;
ordinal-selector = ORDINAL , fillers , [ STRING | ID | WORD ] , [ "in" , fillers , [ STRING ] ] ;
type-selector    = WORD ;                                           (* widget type name: ElevatedButton *)
```

- `text-selector`: matches a widget whose text contains the string.
- `id-selector`: matches `Key('name')` or `Semantics(identifier: 'name')`.
- `positional-suffix`: the text must be inside the container widget.
- `relational-suffix`: the text is below / above / left of / right of the anchor.
- `ordinal-selector`: the n-th match; it composes with a text, an `#id` or a bare type word.
  A trailing `in "Container"` is parsed but **ignored** by the parser (the container is dropped).
- `type-selector`: a bare widget-type word (`tap ElevatedButton`). It is **not** written with angle brackets.

```probe
test "selector forms"
  tap "Login"
  tap #login_button
  tap ElevatedButton
  tap 2nd "Add"
  tap 1st #card
  tap 3rd item
  tap "Edit" in "Settings"
  tap "Submit" below "Email"
  tap "Submit" above "Footer"
  tap "Cancel" left of "Submit"
  tap "Help" right of "Submit"
```

### Modifiers

```ebnf
if-visible   = "if" , ( "visible" | "present" ) ;       (* exactly lower case; skips the step when the target is absent *)
optional-mod = "optional" ;                             (* always attempts the step; a failure becomes a warning *)
```

The modifiers are written at the end of a step, `if visible` first, then `optional`. Which steps
accept them is listed on each production.

### Tap and gestures

```ebnf
tap-step         = "tap" , selector , [ if-visible ] , [ optional-mod ] ;
tap-native-step  = "tap" , "native" , fillers , text-operand ;           (* Android native UI *)
long-press-step  = "long press" , selector , [ if-visible ] , [ optional-mod ] ;
double-tap-step  = "double tap" , selector , [ if-visible ] , [ optional-mod ] ;
clear-step       = "clear" , selector , [ if-visible ] , [ optional-mod ] ;
toggle-step      = "toggle" , selector ;
drag-step        = "drag" , selector , fillers , selector ;
swipe-step       = "swipe" , fillers , [ direction ] , fillers , [ target-selector ] ;
scroll-step      = "scroll" , fillers , [ direction ] , fillers , [ target-selector ] ,
                   [ "until" , fillers , target-selector , fillers , [ "appears" | "visible" | "present" ] ] ;
direction        = "up" | "down" | "left" | "right" ;       (* default: down *)
```

`drag` separates its two selectors with the filler `"to"`. `scroll ... until` keeps scrolling until
the target is on screen and is an error without a target.

```probe
test "tap family"
  tap "Submit"
  tap "Rate this app" optional
  tap "Aceptar" if visible
  tap "Cerrar" if visible optional
  tap native "Choose from Gallery"
  long press "Item"
  long press #row if visible
  double tap "Image" optional
  clear "Search"
  clear #email if visible
  clear 2nd "Field"
  toggle "Dark Mode"
  toggle #unit_toggle
  drag "Item A" to "Item B"
  drag #source to #target
```

```probe
test "swipe and scroll"
  swipe left
  swipe up on "Card"
  swipe down
  swipe right on #pager
  scroll down
  scroll
  scroll up on "ListView"
  scroll down until "Rate this app" appears
  scroll until #share_button is visible
  scroll down on "Feed" until "Footer" appears
```

### Typing

```ebnf
type-step        = "type" , text-operand , fillers , [ target-selector ] ,
                   { "field" | "button" } , [ if-visible ] , [ optional-mod ] ;
type-native-step = "type" , "native" , fillers , text-operand , fillers , text-operand ;
```

```probe
test "typing"
  type "hello@world.com" into "Email"
  type "secret123" into the "Password" field
  type "42" into #age
  type "x" into "Field" if visible
  type "x" into "Field" optional
  type "no target"
  type native "wifi" into "Search settings"
```

### Assertions

```ebnf
see-step      = "see" , ( see-tail , [ "with" , "ai" ] | see-any-tail ) , [ optional-mod ] ;
dont-see-step = ( "don't see" | "dont see" ) , ( see-tail | see-any-tail ) , [ optional-mod ] ;
see-any-tail  = fillers , "any" , fillers , [ "of" ] , alt-list ;       (* passes when any alternative is on screen; don't see: when none is *)
see-native-step = ( "see" | "don't see" | "dont see" ) , "native" , fillers , text-operand ;

see-tail      = fillers , [ "exactly" , [ INT ] , fillers ] , selector ,
                { "button" | "field" | filler } , [ state-check ] , [ "matching" , text-operand ] ;
state-check   = "enabled" | "disabled" | "checked" | "focused" | "contains" , text-operand ;
```

- At most **one** `state-check` is allowed, optionally followed by one `matching` pattern (a regular expression).
- `with ai` is only valid on `see`; `don't see ... with ai` is a parse error. Quote the sentence as the selector text.
- `with ai` is written in lower case and needs an `ai:` block in `probe.yaml`.

```probe
test "assertions"
  see "Dashboard"
  see the "Submit" button
  see "Submit" is enabled
  see "Submit" is disabled
  see "Terms" is checked
  see "Email" is focused
  see "Price" contains "$9.99"
  see "Email" matching ".*@.*"
  see "Price" contains "$" matching "[0-9]+"
  see exactly 3 "Item"
  see any of "Create Your Account", "Confirm Your Details"
  don't see any of "Error", "Failed"
  see exactly 2 2nd "Row"
  see #welcome_banner
  see 2nd "Item"
  see "Price" in "Product Card"
  see "Submit" below "Email"
  see "Maybe there" optional
```

```probe
test "negative assertions"
  don't see "Error"
  dont see #spinner
  don't see exactly 3 "Item"
  don't see "Promo" optional
  see native "IMG_0001.jpg"
  don't see native "Error"
```

### AI-assisted steps

```ebnf
assert-defects-step = "assert" , fillers , "no" , fillers , [ "visual" , fillers ] ,
                      "defects" , "with" , "ai" ;                   (* "no", "visual", "defects", "ai": lower case *)
read-ai-step        = "read" , fillers , text-operand , fillers , "with" , "ai" , "into" , fillers ,
                      ( WORD | STRING ) ;
```

`see "..." with ai` is described under [Assertions](#assertions). All three steps need `ai:` configured in
`probe.yaml`. `read ... with ai into <var>` stores the extracted text in a variable that later steps read as
`"<var>"`.

```probe
test "ai steps"
  see "the checkout total looks correct" with ai
  see "the banner is readable" with ai optional
  assert no visual defects with ai
  assert no defects with ai
  read "the 6-digit OTP code" with ai into otp
  type "<otp>" into "Code"
```

### Waits

```ebnf
wait-duration-step   = "wait" , fillers , ( INT | FLOAT ) , fillers , [ "seconds" | "second" ] ;
wait-until-step      = "wait" , fillers , "until" , fillers , text-operand , fillers ,
                       [ "appears" , fillers , [ "matching" , fillers , STRING ]
                       | "disappears" ] ;                            (* default: appears; matching = regex the text must match *)
wait-any-step        = "wait" , fillers , "until" , fillers , "any" , fillers ,
                       alt-list , fillers , [ "appears" ] ;
alt-list             = STRING , { alt-sep } , STRING , { { alt-sep } , STRING } ;
alt-sep              = filler | "," | "or" ;
wait-idle-step       = "wait" , fillers , ( "until" , fillers , "idle" | "idle" , rest-of-line ) ;
wait-animations-step = "wait" , fillers , ( "animations" | "animation" ) , rest-of-line ;
wait-network-step    = "wait" , fillers , ( "until" , fillers , "network" , fillers
                                          | "network" , rest-of-line ) ;
wait-id-step         = "wait" , fillers , ID , fillers , [ "appears" | "disappears" ] ;
wait-page-step       = "wait" , fillers , [ "page" ] , rest-of-line ;   (* "page ..." and the catch-all *)
```

- `wait-any-step` needs at least two quoted alternatives, separated by commas, `or` or just spaces.
- `wait-page-step` is the catch-all: **any** `wait ...` line that no earlier form matched
  (`wait for the page to load`, `wait for the app to be idle`, `wait for "Foo"`) waits for the page to settle.
- `wait until network` is the only until-form for the network; a trailing `is idle` leaves the word
  `idle` behind as a stray recipe call. Write `wait for network idle`.
- Numbers need no unit: `wait 2` equals `wait 2 seconds`.

```probe
test "waits"
  wait 5 seconds
  wait 1 second
  wait 1.5 seconds
  wait 2
  wait until "Dashboard" appears
  wait until "0 ml" appears matching "^0 ml$"
  wait until "Loading" disappears
  wait until Dashboard
  wait until #spinner disappears
  wait until #home_title appears
  wait #toast disappears
  wait until any of "Got it", "Login", "Home" appears
  wait until any of "A" or "B" or "C"
  wait for idle
  wait until idle
  wait for animations to end
  wait for network idle
  wait until network
  wait for the page to load
  wait for the app to be idle
```

### System dialogs

OS-level dialogs live outside the Flutter widget tree. The parser recognises these statements by
scanning the whole line (outside strings) for `system dialog`, `system field` or
`sign in sandbox tester`; the plain words around the quoted strings are ignored. The first word picks
the operation, the quoted strings are taken in order.

```ebnf
system-dialog-step = system-tap-step | system-type-step | system-see-step
                   | system-wait-step | system-dismiss-step | system-sandbox-step ;
system-tap-step     = "tap" , STRING , fillers , "system" , "dialog" , [ STRING ] , [ optional-mod ] ;
system-type-step    = "type" , STRING , fillers , "system" , "field" , STRING ,
                      [ fillers , "system" , "dialog" , STRING ] , [ optional-mod ] ;
system-see-step     = ( "see" | "don't see" | "dont see" ) , "system" , "dialog" , [ STRING ] , [ optional-mod ] ;
system-wait-step    = "wait" , fillers , "system" , "dialog" , [ STRING ] ,
                      [ "appears" | "disappears" ] , [ optional-mod ] ;
system-dismiss-step = "dismiss" , "system" , "dialog" , [ STRING ] , [ optional-mod ] ;
system-sandbox-step = "sign" , "in" , "sandbox" , "tester" , [ optional-mod ] ;
```

The optional trailing string is a dialog-title filter. A value typed into a system field may be an
environment variable (`"$NAME"` or `"${NAME}"`) and is always masked in output.

```probe
test "system dialogs"
  tap "Allow" in system dialog
  tap "OK" in system dialog "Apple Account"
  type "$PROBE_SANDBOX_PASSWORD" into system field "Password"
  see system dialog "Sign in to Apple Account"
  don't see system dialog "Notifications"
  wait for system dialog "Notifications" appears
  wait for system dialog "Notifications" disappears
  dismiss system dialog
  sign in sandbox tester
  tap "Allow" in system dialog optional
```

### App lifecycle, device actions and utilities

```ebnf
open-app-step   = "open" , fillers , "app" ;
open-link-step  = "open" , fillers , "link" , fillers , text-operand , [ fillers , "app" ] ;
close-step      = "close" , fillers , [ "app" | WORD | STRING ] ;
restart-step    = "restart" , fillers , [ "app" ] ;
kill-step       = "kill" , fillers , [ "app" ] ;
clear-data-step = "clear app data" ;
go-back-step    = "go back" ;
press-enter-step = "press enter" ;                  (* the keyboard action key on the focused text field *)
shake-step      = "shake" ;
pause-step      = "pause" ;
log-step        = "log" , [ STRING ] ;
rotate-step     = "rotate" , fillers , [ WORD | STRING ] ;           (* default: portrait; landscape | portrait *)

permission-step   = ( "allow permission" | "deny permission" | "allow" | "deny" ) , fillers ,
                    [ "permission" | "permissions" ] , fillers , STRING , fillers ,
                    [ "permission" | "permissions" ] ;
grant-revoke-step = ( "grant all permissions" | "revoke all permissions" | "grant" | "revoke" ) , fillers ,
                    [ "all" ] , fillers , [ "permission" | "permissions" ] ;

copy-step            = "copy" , fillers , [ STRING ] , rest-of-line ;    (* "to clipboard" is swallowed *)
paste-step           = "paste" , rest-of-line ;                         (* "from clipboard" is swallowed *)
set-location-step    = "set location" , fillers , coordinate ;
set-language-step    = "set language" , fillers , STRING ;               (* "de", "pt-BR", "system" *)
verify-browser-step  = "verify external browser" , rest-of-line ;       (* "opened" is swallowed *)
add-media-step       = "add media" , fillers , text-operand ;

coordinate = [ "-" ] , number , ( "," , [ "-" ] , number | number ) ;
number     = INT | FLOAT ;
```

- `open-link-step` ends with an optional `in the app` (also `into the app`, `in app`): the URL is routed
  through the OS intent / URL handler instead of the external browser.
- `open "x"` (anything after `open` that is not `app` or `link`) is a recipe call.
- `close` takes the word `keyboard`, the word `app`, or a quoted name.
- `rotate` lower-cases its argument; `landscape` and `portrait` are the ones the runtime understands.
- `coordinate` is `latitude, longitude`. The comma may be omitted only when the longitude is positive
  (`37 -122` is rejected, `37, -122` is fine). Values are validated at run time, not by the parser.

```probe
test "app lifecycle"
  open the app
  open app
  restart the app
  restart
  kill the app
  kill
  clear app data
  close the app
  close keyboard
  close "Dialog"
  close
  go back
  press enter
  shake
  pause
  log "checkpoint reached"
  log
  rotate landscape
  rotate portrait
  rotate
```

```probe
test "deep links and browser"
  open link "https://example.com"
  verify external browser opened
  open link "myapp://profile/42" in the app
  open link "myapp://profile/43" into the app
  open link "myapp://profile/44" in app
```

```probe
test "permissions"
  allow permission "notifications"
  deny permission "camera"
  allow "microphone"
  grant all permissions
  revoke all permissions
```

```probe
test "clipboard"
  copy "user@example.com" to clipboard
  paste from clipboard
  type "<clipboard>" into "Email"
```

```probe
test "location and media"
  set location 37.7749, -122.4194
  set language "de"
  set location -33.8688, 151.2093
  add media "fixtures/photo.jpg"
```

### Screenshots, diagnostics and variables

```ebnf
take-screenshot-step    = "take" , fillers , [ "screenshot" ] ,
                          [ "called" , text-operand | STRING ] ;
compare-screenshot-step = "compare" , fillers , [ "screenshot" ] , fillers ,
                          ( "called" , text-operand | STRING ) , [ "of" , selector ] ;
dump-tree-step          = "dump" , fillers , [ "widget" ] , [ "tree" ] ;
save-logs-step          = "save" , fillers , [ "device" ] , [ "logs" ] ;
store-step              = "store" , fillers , text-operand , fillers , [ "as" ] , fillers , [ WORD | STRING ] ;
deliver-signal-step     = "deliver" , fillers , "signal" , fillers , text-operand , fillers , [ STRING ] ;
```

`compare screenshot` requires a name. `of <selector>` scopes it to one widget. `deliver signal`
defaults its value to `"true"`.

```probe
test "screenshots and diagnostics"
  take screenshot "checkout_page"
  take screenshot called "named"
  take screenshot
  compare screenshot "baseline"
  compare screenshot "price_tag" of "Price Label"
  compare screenshot called "hero"
  dump tree
  dump the widget tree
  save logs
  save device logs
```

```probe
test "variables and signals"
  store "Alice" as username
  type "<username>" into "Name"
  deliver signal "payment_ready"
  deliver signal "payment_ready" "true"
```

### Biometrics

```ebnf
biometric-step        = "biometric" , fillers , [ "no" ] , fillers , [ "match" ] ;
enroll-biometric-step = "enroll" , fillers , [ "biometric" ] ;
```

```probe
test "biometrics"
  enroll biometric
  biometric match
  biometric no match
```

### HTTP calls

```ebnf
http-call-step = "call" , fillers , [ "get" | "post" | "put" | "delete" ] , fillers , [ STRING ] , fillers ,
                 [ "with" , fillers , [ "body" ] , fillers , [ STRING ] ] ;      (* method default: GET *)
```

Calls run on the CLI, not on the device. The response is available as `"<response.status>"` and
`"<response.body>"`.

```probe
test "http calls"
  call GET "https://api.example.com/health"
  call POST "https://api.example.com/seed" with body "{\"env\":\"test\"}"
  call PUT "https://api.example.com/users/1" with body "{\"name\":\"updated\"}"
  call DELETE "https://api.example.com/sessions"
  see "<response.status>"
```

## 4. Block statements

A block statement is a header line followed by an indented block (`body`).

### Conditionals

```ebnf
if-step = "if" , fillers , text-operand , fillers , [ "appears" ] , NEWLINE , body ,
          [ ( "otherwise" | "else" ) , NEWLINE , body ] ;
```

The condition is a text (or `#id`) that may be on screen. `otherwise` / `else` must follow the `if`
body directly, at the same indentation as the `if`.

```probe
test "conditionals"
  if "Accept Cookies" appears
    tap "Accept Cookies"
  if "Welcome Back" appears
    tap "Continue"
  otherwise
    tap "Sign In"
  if #promo_banner appears
    tap "Close"
  else
    log "no promo"
  if "Outer" appears
    if "Inner" appears
      tap "Inner"
```

### Loops and retries

```ebnf
repeat-step = "repeat" , [ INT ] , fillers , [ "times" ] , NEWLINE , body ;   (* count default: 1 *)
retry-step  = "retry" , [ INT ] , fillers , [ "times" ] , NEWLINE , body ;    (* attempts default: 1 *)
```

`repeat` always runs every iteration. `retry` re-runs the whole block on failure, up to N attempts, and
stops at the first success.

```probe
test "loops and retries"
  repeat 3 times
    swipe left
    wait 1 second
  repeat 2
    tap "Next"
  retry 3 times
    tap "Submit"
    see "Success"
  retry
    tap "Refresh"
```

### Dart escape hatch

```ebnf
dart-step = "run" , fillers , [ "dart" ] , [ COLON ] , NEWLINE ,
            [ INDENT , { dart-line | NEWLINE } , DEDENT ] ;
dart-line = { ? any token except NEWLINE and DEDENT ? } , NEWLINE ;
```

The block must start with `run` (a bare `dart:` is not a statement). Each line is rebuilt from its tokens
joined by single spaces, so the lexer's [ignored characters](#lexemes) (`=`, `;`, `.`, braces, ...)
are not part of the captured code.

```probe
test "dart block"
  open the app
  run dart:
    print("hello")
  see "Dashboard"
```

### HTTP mocking

```ebnf
mock-step = "when" , { ? any token except get, post, put, delete, STRING, ID and NEWLINE ? } ,
            [ "get" | "post" | "put" | "delete" ] , [ STRING | WORD ] , NEWLINE ,
            [ INDENT , [ respond-clause ] , NEWLINE , DEDENT ] ;
respond-clause = "respond" , fillers , [ "with" ] , [ INT ] , fillers , [ "and" ] , [ "body" ] , [ STRING ] ;
```

The words between `when` and the method (`the app calls`) are ignored. The method defaults to
`GET`, the status to `200`. The indented body is exactly one `respond` line.

```probe
test "http mocking"
  when the app calls POST "/api/auth/login"
    respond with 503 and body "{ \"error\": \"Service Unavailable\" }"
  when the app calls GET "/api/users"
    respond with 200 and body "[]"
  when the app calls DELETE "/api/session"
    respond with 204
  tap "Sign In"
```

### GPS routes

```ebnf
travel-step   = "travel" , fillers , NEWLINE , [ INDENT , { waypoint-line | NEWLINE } , DEDENT ] ,
                [ "over" , [ number ] , fillers , [ "seconds" | "second" ] , NEWLINE ] ;
waypoint-line = coordinate , NEWLINE ;
```

The optional `over N seconds` line is a **sibling** of `travel to`, at the same indentation, not part
of the waypoint block.

```probe
test "gps route"
  travel to
    37.7749, -122.4194
    37.7849, -122.4094
    37.7949, -122.3994
  over 10 seconds
  see "Arrived"
```

## 5. Recipe calls

```ebnf
recipe-call-step = call-token , { call-token } ;
call-token       = STRING | INT | ? any other token except NEWLINE and DEDENT ? ;
```

Any line that is not one of the statements above is a recipe call. The parser records the words
lower-cased and joined by spaces; every quoted string becomes the placeholder `<arg>` in the name and an
argument in order. A bare number is kept both ways: as part of the name (`step 2 of onboarding`) and as
an argument (`increment counter "Likes" 3`); the runtime tries the name first.

```probe
recipe "sign in as" (email, password)
  type "<email>" into "Email"
  type "<password>" into "Password"

recipe "dismiss onboarding"
  tap "Skip"

recipe "increment counter" (label, amount)
  tap "<label>"
  log "<amount>"

recipe "step 2 of onboarding"
  tap "Next"

recipe "clear search"
  clear "Search"

recipe "open most recent post"
  tap 1st "Post"

test "recipe call forms"
  dismiss onboarding
  sign in as "a@b.co" with "pw"
  sign in as "a@b.co" and "pw"
  increment counter "Likes" 3
  step 2 of onboarding
  clear search
  open most recent post
  the dismiss onboarding
```

## 6. Lexical conventions in practice

```probe
# A comment line: ignored, may start with anything, even a letter
test "comments and strings"   # inline comment: "#" followed by a space
  tap "Submit"               # trailing comment
  tap #submit_button         # "#id" (a letter after #) is a selector, not a comment
  type "line one\nline two" into "Notes"
  type "tab\there" into "Notes"
  type "say \"hi\"" into "Notes"
  tap "has # hash inside a string"
  wait 1.5 seconds
  tap 3rd "Item"
```

```probe
test "case-insensitive keywords"
  TAP "Login"
  Wait Until "Dashboard" Appears
  Don't See "Error"
  See Exactly 2 "Item"
```

```probe
test "tabs count as four spaces"
	tap "Tab indented"
	if "Dialog" appears
		tap "OK"
```

```probe
recipe "looking-for item"
  tap "Search"

test "hyphens and apostrophes in words"
  looking-for item
  don't see "Error"
```

## 7. Placeholders and variables

Variables are written inside strings as `"<name>"`; the parser never looks inside a string, the runtime
substitutes them when the step runs.

```ebnf
placeholder      = "<" , placeholder-name , ">" ;     (* only inside the content of a STRING *)
placeholder-name = variable | "clipboard" | "response.status" | "response.body" | random ;
variable         = identifier ;                       (* recipe parameter, example column, store target, read-into target *)
random           = "random.email" | "random.name" | "random.phone" | "random.uuid"
                 | "random.number" , [ "(" , INT , "," , INT , ")" ]
                 | "random.text" , [ "(" , INT , ")" ] ;
```

| Source of a variable | How it is bound |
|---|---|
| recipe parameter | by position from the quoted arguments of the call |
| `with examples` column | one value per row, by column header |
| `store "v" as name`, `read ... into name` | assigned when the step runs |
| `paste from clipboard` | `"<clipboard>"` |
| `call ...` | `"<response.status>"`, `"<response.body>"` |
| `"<random.*>"` | generated fresh each time the string is resolved |

An unquoted `<name>` is a lexical error, because it would otherwise be typed or matched as literal text.

```probe
recipe "fill profile" (name, city)
  type "<name>" into "Name"
  type "<city>" into "City"

test "placeholders"
  fill profile "Alice" with "Lisbon"
  type "<random.email>" into "Email"
  type "<random.name>" into "Full Name"
  type "<random.phone>" into "Phone"
  type "<random.uuid>" into "Reference"
  type "<random.number(1,100)>" into "Age"
  type "<random.text(8)>" into "Code"
  store "Alice" as username
  see "<username>"
```

Filler words around operands are skipped:

```probe
test "filler words"
  the tap "Login"
  tap the "Login"
  type "x" into the "Email" field
  see the "Welcome" button
  scroll down on the "Feed"
  wait for the page to load
```

## 8. Semantic notes that EBNF cannot express

**Environment variables.** At run time `${NAME}` inside any quoted text (a `STRING`, including selectors) is
replaced by the environment variable `NAME`; the step line prints the template and the value is scrubbed from
errors. The parser does not see this: a `${...}` is ordinary string content to the grammar.

**Bare `type` and `clear` act on the focused field.** `type "x"` without `into`, and `clear` without a
selector, target the text field that has focus (an error when none has).

**Loose text matching.** With `--match-loose` (or `defaults.match: loose`) text selectors compare after folding case,
accents and other diacritics, typographic apostrophes and dashes, full-width forms, invisible characters and whitespace
(the same folding on both sides; ids are never folded). It is a run setting, not syntax.

**Implicit wait.** With `--implicit-wait <d>` (or `defaults.implicit_wait`) a `tap`, `type`, `long press`, `double tap`,
`clear`, `drag` or plain `see` whose target is not on screen is retried for up to `<d>` before it fails. It is a run
setting, not syntax; steps with `if visible` / `optional`, `don't see` and the `wait` steps are never retried.

**Text selectors match substrings.** A `STRING` selector matches every widget whose text contains it
(`see "0 ml"` is satisfied by "250 ml"). `matching "<regex>"` then requires that at least one of the matched
widgets has text matching the regular expression, so `see "0 ml" matching "^0 ml$"` is the exact form.

**Statement boundaries.** A statement ends at the end of its line. A built-in step consumes only what its
production names; any tokens left on the line are parsed as a **separate** statement, which is usually a
recipe call that fails at run time with "unknown recipe call". This is how `tap the "Login" button`
(trailing `button`), `wait 1 minute` (trailing `minute`) and `wait until "x" is gone` (trailing `gone`)
go wrong. Lines never continue onto the next line.

**Ordered choice and lookahead.** The grammar is ambiguous as written; the parser resolves it by the
dispatch order above, by peeking at the rest of the line (system dialogs, `open`, `clear`), and by the
lexer's compound-keyword lookahead (`before all tests` is one token, `before all` another).

**Case.** Keywords and contextual words (`native`, `dismiss`, `system`, `dialog`, `devices`, `any`, `or`,
`match`) match case-insensitively. The words `visible`, `present`, `ai`, `no`, `visual` and `defects`
must be lower case. Recipe names are stored exactly as written in `recipe "Name"`, while a call is
lower-cased, so give recipes lower-case names.

**Filler stripping.** At parse time fillers are skipped only where the grammar says `fillers`, and at the
start of every statement. Inside a recipe call they are kept as words. At run time the recipe lookup
additionally ignores the words `and`, `with`, `the`, `then` and the `<arg>` markers, in both the call and
the definition name, so `sign in as "a" with "b"` finds `recipe "sign in as"`. The other connecting words
(`into`, `to`, ...) are **not** ignored by the lookup.

**Recipe resolution.** Recipes come from the file itself and from `use`d files and the project's
`recipes_folder`. A call is resolved by exact name, then with `<arg>` and the words above stripped, then
with the same stripping applied to definition names; a call containing a bare number is tried with the
number as part of the name first and as an argument second. No match is a run-time error. Arguments are
bound to parameter names by position into a flat variable map; there is no per-recipe scope.

**Silent leniency.** The parser prefers to accept and default over reporting an error. These are accepted
without complaint and yield empty values: `use`, `recipe`, `test` or `composite` without a name; `if`
without a condition; a missing selector (`tap`); `store` without a name; `log` without a message; a
`with examples from` without a file name; a `recipe` parameter list that is never closed (it swallows the
rest of the file); unexpected tokens at file level and inside `composite` bodies;
Inconsistent indentation does not raise an error
either: an over-indented line becomes a recipe call named `<indent> ...` and a stray dedent can end a
block early.

**Hooks.** Hooks are file-scoped. `before all` and `after all` run once; `before each` / `after each`
run around every test; `on failure` runs after a failed test.

**Examples.** `with examples` repeats the preceding test once per row; `"<column>"` placeholders in the
body are replaced by that row's cells. Row width is not validated against the header.

**Platform notes.** Native UI steps (`tap native`, `type native`, `see native`) are Android only.
Location, travel, media, biometric and permission steps apply to simulators and emulators.

## Where the docs and the parser disagree

These are behaviours of the parser at the time of writing, found by cross-checking the
[Syntax](/probescript/syntax/), [Dictionary](/probescript/dictionary/), [Recipes](/probescript/recipes/),
[Hooks](/probescript/hooks/) and [Data-driven](/probescript/data-driven/) pages against the source.
The grammar above follows the parser.

| Documented | What the parser does | Use instead |
|---|---|---|
| `type <email> into "Email"`, `see <expected>` (unquoted placeholders in Recipes and Data-driven pages) | lexical error: "unquoted placeholder" | `type "<email>" into "Email"` |
| `log in as "u" with "p"` as a recipe call (Recipes and Hooks pages) | `log` is a keyword: parsed as `log` plus a call named `as <arg> with <arg>` | name recipes so the first word is not a keyword (`sign in as`) |
| `tap the "Login" button` "is equivalent to" `tap "Login"` (Dictionary, Filler Words) | trailing `button` / `field` is only consumed by `type`, `see` and `don't see`; after `tap` it becomes a stray recipe call | `tap "Login"` |
| `see 3 "Item"` (Syntax, Assertions) | no count is parsed; an empty assertion plus a recipe call `3 <arg>` | `see exactly 3 "Item"` |
| `tap <ElevatedButton>` (Syntax, Selectors table) | lexical error (unquoted `<`) | `tap ElevatedButton` |
| `dart:` on its own line (Syntax, Dart Escape Hatch) | not a statement; becomes junk recipe calls | `run dart:` |
| state suffixes "compose": `see "F" is enabled contains "y" matching "z"` (Dictionary) | one state check, then optional `matching`; the second state word starts a stray recipe call | `see "F" is enabled`, or `see "F" contains "y" matching "z"` |
| `wait until network is idle`, `wait for the app to be idle` (parser comments) | the first leaves a stray `idle` recipe call; the second is the page-load catch-all | `wait for network idle`, `wait for idle` |
| filler words list lacks `to` (Dictionary) | `to` is a filler (needed by `drag A to B`, `travel to`) | n/a |
| recipe args "matched using connecting words like and, with, or into" (Recipes) | the run-time lookup ignores `and`, `with`, `the`, `then` only | use `and` / `with` |
| `Press` and `Pinch` DSL steps (Annotations) | `press` and `pinch` are reserved words with no statement; a line starting with them is a recipe call | `go back` |
| a `Dart` block "executes arbitrary Dart" (Syntax) | code is re-assembled from tokens; characters such as `=`, `;`, `.` and braces never reach the agent | keep Dart snippets to token-safe code |
| `ordinal ... in "Container"` (Annotations: `Ordinal(2, 'Item', container: 'List')`) | the container is parsed but dropped | use a positional selector |
| `rotate landscape` / `rotate portrait` (Dictionary) | any word or string is accepted; unknown values are only rejected at run time | `landscape` or `portrait` |

## Keyword reference

All reserved words are in the [`keyword`](#keywords) production; the compound forms are in
[`compound-keyword`](#compound-keywords). The words below are not reserved, but the parser gives them
a meaning in one position: `native`, `visible`, `present`, `ai`, `any`, `or`, `no`, `match`, `visual`,
`defects`, `devices`, `system`, `dialog`, `dismiss`, `sign`, `sandbox`, `tester`, `up`, `down`.
