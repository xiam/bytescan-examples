# typescript

A scanner for a subset of TypeScript: the tokens of the
[ECMAScript lexical grammar, clause 12](https://tc39.es/ecma262/2024/#sec-ecmascript-language-lexical-grammar)
and the common TypeScript keywords, built from `rules/core` and `rules/base`.
It is a mode-driver example (see
[Modes](../README.md#modes-several-scanners-over-one-cursor)).

**Technique:** two choices that no single rule set can make, both made by a
mode driver over one cursor. Whether `/` starts a regular expression or is a
division depends on the token before it (previous-token context). A template
literal `` `…${ … }…` `` holds code, which can hold templates again (a stack of
modes).

```sh
go run ./typescript < typescript/testdata/document.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `KEYWORD` | the ECMAScript reserved words `await break case catch class const continue debugger default delete do else enum export extends false finally for function if import in instanceof new null return super switch this throw true try typeof var void while with yield`; the strict-mode words `let static implements interface package private protected public`; the TypeScript words `abstract as declare keyof namespace readonly satisfies type` |
| `IDENTIFIER` | a Unicode letter or letter number, `$` or `_`, then those, marks, decimal digits, connector punctuation, ZWNJ and ZWJ; not a keyword |
| `PUNCTUATOR` | `{ } ( ) [ ] . ... ; , @ < > <= >= == != === !== + - * % ** ++ -- << >> >>> & \| ^ ! ~ && \|\| ?? ? ?. : = += -= *= %= **= <<= >>= >>>= &= \|= ^= &&= \|\|= ??= =>`; `?.` only when no digit follows |
| `DIV_PUNCTUATOR` | `/` and `/=`, where the driver expects a division |
| `REGEX` | `/body/flags`, where the driver expects a regular expression: the body is on one line, a backslash escapes the rune after it, and a `/` inside `[…]` does not end it; the flags are identifier runes |
| `NUMBER` | a decimal number with optional fraction and exponent (`42`, `3.14`, `3.`, `.5`, `2.5e-3`), a hexadecimal, octal or binary integer (`0xff`, `0o17`, `0b1010`), a BigInt (`123n`, `0xffn`); `_` between two digits |
| `STRING` | a string in `"…"` or `'…'` on one line; a backslash escapes the rune after it |
| `COMMENT` | `//` to the end of the line, or `/* … */` across lines |
| `TEMPLATE_OPEN` | the `` ` `` that opens a template literal |
| `TEMPLATE_TEXT` | the text of a template, line breaks included; `$` not followed by `{` and a backslash with the rune after it are text |
| `TEMPLATE_EXPR_OPEN` | the `${` that opens a substitution |
| `TEMPLATE_EXPR_CLOSE` | the `}` that closes a substitution |
| `TEMPLATE_CLOSE` | the `` ` `` that closes a template literal |
| `WHITESPACE` | a run of space, tab, `\v` and `\f` |
| `NEWLINE` | one line break: `\n`, `\r\n` or `\r` |
| `UNKNOWN` | one rune that no rule of the current mode matches |

The specification lexes a template as one token per piece between
substitutions (`NoSubstitutionTemplate`, `TemplateHead`, `TemplateMiddle`,
`TemplateTail`). This example splits each piece into its delimiters and its
text, so the driver can change mode at each delimiter.

## The mode driver

`driver.go` runs on a `modes.Driver` over one cursor, with these scanners:

- **Code mode** (`newCodeScanner`) lexes code. In division mode (`regexGoal`
  false), `/` and `/=` are `DIV_PUNCTUATOR`: the `InputElementDiv` goal. In
  regex mode (`regexGoal` true), `/…/flags` is `REGEX`: the
  `InputElementRegExp` goal. With `exprClose` true, `}` is
  `TEMPLATE_EXPR_CLOSE`, the end of a substitution (the
  `InputElementTemplateTail` goal). Every other token lexes the same way.
- **Template mode** (`newTemplateScanner`) lexes the inside of a template:
  `TEMPLATE_TEXT`, `TEMPLATE_EXPR_OPEN` and `TEMPLATE_CLOSE`.

### Regex or division

The driver keeps one fact about the previous significant token: whether it
can end an expression. Whitespace, line breaks and comments are not
significant. After an identifier, a literal (`NUMBER`, `STRING`, `REGEX`, a
template's `TEMPLATE_CLOSE`), `)`, `]`, `}`, or one of the keywords `this`,
`super`, `true`, `false` and `null`, the driver picks division mode. After any
other token, and at the start of the input, it picks regex mode. So
`a / b / c` is two divisions, and a `/` after `=`, `(`, `,`, `!`, `&&`, `?`,
`:`, `${`, `return` or `typeof` starts a regular expression.

The keywords are the case that a rule of only "identifier before `/` means
division" gets wrong: `return /re/` is a regular expression, although `return`
looks like an identifier. Here `return` is a `KEYWORD`, and only the five
keywords that are values pick division mode.

The rule looks at one token. ECMAScript decides from the syntactic context,
which a scanner does not see, so the rule is wrong in these cases (shown in
`regex-wrong`):

- **A `}` that ends a block,** not an object literal: in `if (ok) {}` and a
  `/ab+c/.test(s)` on the next line, the `/` is lexed as a division.
- **A `)` that ends the condition of `if`, `while` or `for`:** in
  `if (x) /re/.exec(s)` the `/` is lexed as a division.
- **Postfix `++` and `--`:** in `a++ / 2` the `/` is in regex mode; no regular
  expression follows, so it is `UNKNOWN "/"`.
- **A keyword used as a name:** TypeScript allows `type` and the other
  TypeScript words as names, so in `type / 2` the `/` is in regex mode and is
  `UNKNOWN "/"`.

### Template nesting

The driver keeps a stack of modes; the bottom is top-level code.
`TEMPLATE_OPEN` pushes template mode and `TEMPLATE_CLOSE` pops it.
`TEMPLATE_EXPR_OPEN` pushes a code frame with a brace count of 0. In that frame
`{` adds one and `}` takes one away. At count 0 the driver picks the code scanner
in which `}` is `TEMPLATE_EXPR_CLOSE`, so that `}` closes the substitution and
the driver pops the frame. This is the lexical rule of the specification, which
lexes the `}` that ends a substitution with the `InputElementTemplateTail`
goal. The count exists only to find that `}`. It never judges whether the
braces balance, and the top-level frame does not count at all. A template
inside a substitution pushes a new template frame, so templates nest to any
depth.

### The three rules

The `modes` package applies the three rules of a mode driver:

1. **Release on every switch.** `modes.Driver` calls `Release()` on the
   outgoing scanner at every switch. An idle scanner that holds its checkpoints
   pins all later input in the shared cursor. `TestRelease` lexes over a
   cursor that retains at most 64 bytes while the driver switches on almost
   every token.
2. **Cache one scanner per mode.** A `modes.Cache` builds each code scanner the
   first time the driver needs its goal and its `}`, and returns the same
   scanner afterwards; the template scanner is built once. The driver never
   builds a scanner per token and never drops one.
3. **One `Next()` at a time.** The scanners share one cursor, so no two of them
   may run at the same time. The `Next` of `modes.Driver` holds a mutex.

Every scanner returns a plain `io.EOF` at the end of the input, whatever its
mode. So only the driver's stack knows that the input ended inside a template
or a substitution. It does not report that: the template text or the code runs
to the end of the input, no closer follows, and lexing ends normally.

## Covered subset

The tokens above. Not covered:

- **JSX.** `<div>{x}</div>` lexes as punctuators, identifiers and an
  `UNKNOWN "/"` (see `not-typescript`).
- **Types.** `<` and `>` are always punctuators; nothing parses a type.
- **Decorators** beyond `@` as a punctuator.
- **Automatic semicolon insertion.** No token is added for a line break.
- **Private names** (`#x`) and a hashbang line: `#` is `UNKNOWN`.
- **Unicode escapes in identifiers** (`a`): the backslash is `UNKNOWN`.
- **Unicode layout.** No-break space, `U+FEFF` and the other space separators
  are `UNKNOWN`; `U+2028` and `U+2029` are not line breaks.
- **Escapes and bodies are not checked.** A backslash takes the rune after it
  in a string, a template and a regular expression. The body and the flags of
  a regular expression are not checked against the pattern grammar.
- **A line continuation in a string** (a backslash before a line break): a
  string is one line here.
- **Numbers are not checked** against the rules that the grammar adds: a
  separator after a leading `0` (`0_1`) and a legacy octal (`017`) lex as
  decimals, and the identifier or digit that may not follow a number is lexed
  as the next token.
- **Contextual keywords** such as `async`, `of`, `get`, `set` and `from` are
  identifiers.

## How invalid input lexes

- **An unterminated string** is not a `STRING`: its quote is `UNKNOWN`, and
  lexing goes on with the next rune. So `"unterminated` lexes as
  `UNKNOWN "\""` and `IDENTIFIER "unterminated"`.
- **An unterminated block comment** is not a `COMMENT`: `/*` lexes as `/` and
  `*`, and the text after it as code.
- **A `/` in regex mode that starts no regular expression** (no closing `/` on
  the line, or `*` or `/` right after it, which would be a comment) is
  `UNKNOWN "/"`, and lexing goes on with the next rune.
- **An unterminated template** gives the opener and its text to the end of the
  input, with no closer. **An unterminated substitution** gives its code to
  the end of the input.
- **A backslash at the end of the input in a template** is `UNKNOWN`.
- **Numbers take the longest form the grammar allows,** and the next rule
  takes the rest: `0x` lexes as `NUMBER "0"` and `IDENTIFIER "x"`, `1_` as
  `NUMBER "1"` and `IDENTIFIER "_"`, `3in` as `NUMBER "3"` and
  `KEYWORD "in"`, `1.5n` as `NUMBER "1.5"` and `IDENTIFIER "n"`, and `1.2.3`
  as `NUMBER "1.2"` and `NUMBER ".3"`.
- **Characters outside the subset,** such as `#`, `\`, `€` and `¿`, are
  `UNKNOWN`.

## Test data

| File | Shows |
| --- | --- |
| `document` | a small module with every kind of token |
| `tokens` | every keyword, contextual words as identifiers, identifiers with `$`, `_` and non-ASCII letters, every punctuator, `?.` before a digit |
| `numbers` | each number form: decimal, fraction, exponent, `3.`, `.5`, hexadecimal, octal, binary, separators, BigInt, `1..toString()` |
| `bad-numbers` | `0x`, `0xg`, `0b2`, `0o8`, `1_`, `1__0`, `_1`, `1e`, `1e+`, `3in`, `1.5n`, `.e1`, `08.5`, `1.2.3` |
| `strings` | strings in both quotes, escapes, non-ASCII text |
| `bad-strings` | strings cut by a line break, and a line continuation |
| `comments` | line and block comments, one across lines, `/** … ***/`, an unterminated block comment |
| `division` | division chains (`a / b / c`), `/=`, a `/` after `]`, `)`, a number, a string, `this`, `true`, `null` and a template |
| `regex` | a regular expression at the start of the input and after `=`, `(`, `,`, `return`, `typeof`, `!`, `&&`, `?`, `:` and `${`; `/` inside a class, escaped `/` and `]`, flags |
| `regex-wrong` | the cases the rule gets wrong, as they are lexed: `}` that ends a block, `)` of `if`, postfix `++`, `type` as a name |
| `bad-regex` | a `/` in regex mode with no closing `/` on the line, `/*/` and `/[/` |
| `templates` | a template with text only, an empty one, `$`, `` \` `` and `\${` as text, one across lines, a substitution with an object literal (`${ {a: 1}.a }`), templates nested in a substitution, adjacent substitutions, a division after a template |
| `unterminated-template` | a template nested in a substitution that the input ends inside |
| `unterminated-expression` | a substitution with an open object literal that the input ends inside |
| `layout` | `\r\n` line breaks, also inside a template; tab, `\v` and `\f` |
| `not-typescript` | `#`, a Unicode escape, characters outside the subset, JSX, a backslash at the end of the input in a template |
