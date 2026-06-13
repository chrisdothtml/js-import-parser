// Package jsimports extracts module imports from JavaScript/TypeScript/Flow
// source without building an AST. The scanner makes a single forward pass
// over the source bytes.
//
// Its primary job is context tracking. At every position, it knows whether
// it's in code, a string, a template literal, a comment, or a regex. Only
// in code context does it look for the `import`/`export`/`require` keywords,
// which makes it immune to import-like text inside strings and comments.
package jsimports

import (
	"bytes"
)

// Kind describes the syntax through which a module was imported.
type Kind uint8

const (
	// KindStatic is `import ... from "x"` or `import "x"`.
	KindStatic Kind = iota
	// KindDynamic is `import("x")`.
	KindDynamic
	// KindRequire is `require("x")`.
	KindRequire
	// KindRequireResolve is `require.resolve("x")`.
	KindRequireResolve
	// KindReExport is `export ... from "x"`.
	KindReExport
	// KindTypeStatic is a TS/Flow type-only import: (e.g. `import type ... from "x"`,
	// `import typeof ... from "x"`, `import { type Foo } from "x"`). A mixed
	// import like `import { type Foo, bar } from "x"` is recorded twice: once
	// as KindStatic and once as KindTypeStatic.
	KindTypeStatic
	// KindTypeReExport is a TS type-only re-export: `export type { T } from "x"`
	// or `export { type T } from "x"`. Mixed re-exports are recorded as both
	// KindReExport and KindTypeReExport.
	KindTypeReExport
)

func (k Kind) String() string {
	switch k {
	case KindStatic:
		return "static"
	case KindDynamic:
		return "dynamic"
	case KindRequire:
		return "require"
	case KindRequireResolve:
		return "require.resolve"
	case KindReExport:
		return "re-export"
	case KindTypeStatic:
		return "type-static"
	case KindTypeReExport:
		return "type-re-export"
	}
	return "unknown"
}

// Import is a single module reference found in the source.
type Import struct {
	// Specifier is the raw module path string (e.g. "./foo" or "lodash").
	Specifier string
	Kind      Kind
	// Line is the 1-based line of the import/export/require keyword.
	Line int
}

// Diagnostic reports source the parser saw but could not statically analyze,
// such as `require(someVariable)` or `import('./' + name)`.
type Diagnostic struct {
	Message string
	Line    int
}

// Result holds everything found in one source file.
type Result struct {
	Imports     []Import
	Diagnostics []Diagnostic
}

// Parse scans src and returns the modules it imports. It never reads outside
// src and performs no I/O; the returned specifiers are copies, so src may be
// reused after Parse returns.
func Parse(src []byte) Result {
	s := scanner{src: src, lineNum: 1}

	// skip past BOM
	if bytes.HasPrefix(src, utf8BOM) {
		s.index = len(utf8BOM)
	}

	// skip past shebang line
	if s.index+1 < len(src) && src[s.index] == '#' && src[s.index+1] == '!' {
		if k := bytes.IndexByte(src[s.index:], '\n'); k >= 0 {
			s.index += k
		} else {
			s.index = len(src)
		}
	}

	s.scan()
	return s.res
}

var (
	utf8BOM   = []byte{0xef, 0xbb, 0xbf}
	starSlash = []byte("*/")
	newline   = []byte{'\n'}
)

// interesting marks the bytes the code-context loop has to stop at: quotes,
// slash (comment or regex), braces (template substitution tracking), and the
// first letters of import/export/require. Everything else is skipped.
var interesting [256]bool

// identChar marks bytes that can appear in an identifier. All bytes >= 0x80
// are treated as identifier characters, which conservatively prevents keyword
// matches adjacent to multi-byte runes.
var identChar [256]bool

func init() {
	for _, char := range []byte{'\'', '"', '`', '/', '{', '}', 'i', 'r', 'e'} {
		interesting[char] = true
	}

	for char := '0'; char <= '9'; char++ {
		identChar[char] = true
	}
	for char := 'a'; char <= 'z'; char++ {
		identChar[char] = true
	}
	for char := 'A'; char <= 'Z'; char++ {
		identChar[char] = true
	}
	identChar['_'] = true
	identChar['$'] = true
	for char := 0x80; char < 256; char++ {
		identChar[char] = true
	}
}

type scanner struct {
	src   []byte
	index int

	// templates is the substitution stack: one entry per `${` we are inside,
	// holding the count of unmatched '{' seen within that substitution so we
	// know which '}' closes it.
	templates []int

	// line numbers are computed lazily, only when something is recorded:
	// lineNum is the line at byte offset linePos.
	linePos int
	lineNum int

	res Result
}

func (s *scanner) scan() {
	src := s.src
	srcLen := len(src)

	for s.index < srcLen {
		char := src[s.index]
		if !interesting[char] {
			s.index++
			continue
		}

		switch char {
		case '\'', '"':
			s.index++
			s.skipString(char)

		case '`':
			s.index++
			s.scanTemplate()

		case '/':
			s.slash()

		case '{':
			if t := len(s.templates) - 1; t >= 0 {
				s.templates[t]++
			}
			s.index++

		case '}':
			if t := len(s.templates) - 1; t >= 0 {
				if s.templates[t] > 0 {
					s.templates[t]--
					s.index++
				} else {
					// end of a `${...}` substitution: back to template text
					s.templates = s.templates[:t]
					s.index++
					s.scanTemplate()
				}
			} else {
				s.index++
			}

		default: // 'i', 'r', 'e'
			s.word()
		}
	}
}

// word is called at a byte that could start a keyword. It reads the whole
// identifier and dispatches if it is one of the three we care about.
func (s *scanner) word() {
	startIdx := s.index
	if !s.boundaryBefore(startIdx) {
		// middle of a larger identifier, e.g. the 'i' in "main"
		s.index++
		return
	}

	switch string(s.readWord()) {
	case "import":
		if !s.precededByDot(startIdx) {
			s.importKeyword(startIdx)
		}

	case "export":
		if !s.precededByDot(startIdx) {
			s.exportKeyword(startIdx)
		}

	case "require":
		if !s.precededByDot(startIdx) {
			s.requireKeyword(startIdx)
		}
	}
}

func (s *scanner) importKeyword(kwStartIdx int) {
	s.skipWsComments()
	if s.index >= len(s.src) {
		return
	}

	switch char := s.src[s.index]; char {
	case '(':
		s.index++
		s.callSpecifier(kwStartIdx, KindDynamic, true)

	case '.':
		// import.meta
		s.index++

	case '\'', '"':
		// side-effect import: `import "x"`
		if spec, ok := s.readString(char); ok {
			s.record(spec, KindStatic, kwStartIdx)
		} else {
			s.diag("unterminated module specifier", kwStartIdx)
		}

	default:
		s.staticImportClause(kwStartIdx)
	}
}

// staticImportClause parses the bindings of `import <clause> from "x"`,
// starting just after the `import` keyword. It tracks whether the clause
// carries value bindings, type bindings, or both, and records the specifier
// accordingly (a mixed clause is recorded as both kinds).
func (s *scanner) staticImportClause(kwStartIdx int) {
	src := s.src
	typePrefix := false // `import type ...` / `import typeof ...`
	hasValue := false
	hasType := false
	first := true

	for {
		s.skipWsComments()
		if s.index >= len(src) {
			s.diag("unterminated import statement", kwStartIdx)
			return
		}

		switch char := src[s.index]; {
		case char == '{':
			s.index++
			hv, ht, ok := s.scanBindingBraces()
			if !ok {
				s.diag("unterminated import statement", kwStartIdx)
				return
			}
			hasValue = hasValue || hv
			hasType = hasType || ht

		case char == '*':
			// namespace import
			s.index++
			hasValue = true

		case char == ',':
			s.index++

		case identChar[char]:
			wStart := s.index
			switch string(s.readWord()) {
			case "from":
				s.skipWsComments()
				if s.index < len(src) && (src[s.index] == '\'' || src[s.index] == '"') {
					if spec, ok := s.readString(src[s.index]); ok {
						s.recordTyped(spec, KindStatic, KindTypeStatic, kwStartIdx,
							typePrefix, hasValue, hasType)
					} else {
						s.diag("unterminated module specifier", wStart)
					}
					return
				}

				// `from` used as a binding name; keep scanning
				hasValue = true

			case "type", "typeof":
				// TS/Flow `import type ...` / `import typeof ...`
				if first && s.typeOnlyClauseFollows() {
					typePrefix = true
				} else {
					// a binding literally named `type`, e.g. `import type, {x}`
					hasValue = true
				}

			case "as":
				// alias keyword, not a binding

			default:
				// a default-import binding name
				hasValue = true
			}

		default:
			// '=' (TS import-equals), ';', or anything else that can't be
			// part of an import clause
			return
		}

		first = false
	}
}

// recordTyped records a specifier under the value kind, the type kind, or
// both, based on what the clause contained. A `type` prefix makes the whole
// statement type-only regardless of the bindings.
func (s *scanner) recordTyped(spec []byte, valueKind, typeKind Kind, kwStartIdx int, typePrefix, hasValue, hasType bool) {
	switch {
	case typePrefix || (hasType && !hasValue):
		s.record(spec, typeKind, kwStartIdx)

	case hasType: // mixed, e.g. `import { type Foo, bar } from "x"`
		s.record(spec, valueKind, kwStartIdx)
		s.record(spec, typeKind, kwStartIdx)

	default:
		s.record(spec, valueKind, kwStartIdx)
	}
}

// typeOnlyClauseFollows decides, positioned right after `import type`,
// whether this is a type-only import or `type` is being used as a regular
// binding name, as in `import type from 'x'` or `import type, {x}`.
// The position is restored before returning.
func (s *scanner) typeOnlyClauseFollows() bool {
	save := s.index
	defer func() { s.index = save }()

	s.skipWsComments()
	if s.index >= len(s.src) {
		return false
	}

	switch char := s.src[s.index]; {
	case char == ',':
		return false

	case char == '{' || char == '*':
		return true

	case identChar[char]:
		if string(s.readWord()) == "from" {
			s.skipWsComments()
			return !(s.index < len(s.src) && (s.src[s.index] == '\'' || s.src[s.index] == '"'))
		}

		return true
	}

	return false
}

func (s *scanner) exportKeyword(kwStartIdx int) {
	src := s.src
	typePrefix := false // `export type { ... } from` / `export type * from`
	hasValue := false
	hasType := false

	s.skipWsComments()
	if s.index >= len(src) {
		return
	}

	if identChar[src[s.index]] {
		origIdx := s.index
		if matched := s.trySkipWord("type"); !matched {
			// export const/function/default/...: nothing to record.
			return
		}

		s.skipWsComments()
		if s.index >= len(src) || (src[s.index] != '{' && src[s.index] != '*') {
			// TS type alias: `export type Foo = ...`
			s.index = origIdx
			return
		}

		typePrefix = true
	}

	switch src[s.index] {
	case '{':
		s.index++
		hv, ht, ok := s.scanBindingBraces()
		if !ok {
			return
		}
		hasValue, hasType = hv, ht

		s.skipWsComments()
		if matched := s.trySkipWord("from"); !matched {
			// plain `export { ... }` with no `from`
			return
		}

	case '*':
		s.index++
		hasValue = true

		s.skipWsComments()
		origIdx := s.index
		if string(s.readWord()) == "as" {
			s.skipWsComments()
			if s.index < len(src) && (src[s.index] == '\'' || src[s.index] == '"') {
				// `export * as "string name" from "x"`
				if _, ok := s.readString(src[s.index]); !ok {
					return
				}
			} else {
				s.readWord()
			}
		} else {
			s.index = origIdx
		}

		s.skipWsComments()
		if matched := s.trySkipWord("from"); !matched {
			return
		}

	default:
		return
	}

	s.skipWsComments()
	if s.index >= len(src) || (src[s.index] != '\'' && src[s.index] != '"') {
		s.diag("expected module specifier after `from`", s.index)
		return
	}

	if spec, ok := s.readString(src[s.index]); ok {
		s.recordTyped(spec, KindReExport, KindTypeReExport, kwStartIdx,
			typePrefix, hasValue, hasType)
	} else {
		s.diag("unterminated module specifier", s.index)
	}
}

func (s *scanner) requireKeyword(kwStartIdx int) {
	src := s.src
	kind := KindRequire

	s.skipWsComments()
	if s.index < len(src) && src[s.index] == '.' {
		s.index++
		s.skipWsComments()
		if string(s.readWord()) != "resolve" {
			// require.cache, require.main, ...
			return
		}
		kind = KindRequireResolve
		s.skipWsComments()
	}

	// optional-chained call: require?.("x")
	if s.index+1 < len(src) && src[s.index] == '?' && src[s.index+1] == '.' {
		s.index += 2
		s.skipWsComments()
	}

	if s.index >= len(src) || src[s.index] != '(' {
		return
	}

	s.index++
	s.callSpecifier(kwStartIdx, kind, false)
}

// callSpecifier parses the argument of `import(...)`/`require(...)` starting
// just after the opening paren.
//
// It records an import only when the argument is a single string literal:
// the string must be followed by ')' (or ',' for dynamic import, which accepts
// an options object), which rejects concatenation like require('a' + b).
func (s *scanner) callSpecifier(kwStartIdx int, kind Kind, allowComma bool) {
	src := s.src
	s.skipWsComments()
	if s.index >= len(src) {
		return
	}

	char := src[s.index]
	if char != '\'' && char != '"' && char != '`' {
		s.diag("non-literal module path", s.index)
		return
	}

	spec, ok := s.readString(char)
	if !ok {
		if char == '`' {
			// template with substitutions; s.index was left at the backtick so
			// the main loop rescans it with substitution tracking
			s.diag("non-literal module path", s.index)
		} else {
			s.diag("unterminated module specifier", s.index)
		}
		return
	}

	s.skipWsComments()
	if s.index < len(src) {
		if char := src[s.index]; char == ')' || (allowComma && char == ',') {
			s.record(spec, kind, kwStartIdx)
			return
		}
	}

	s.diag("non-literal module path", s.index)
}

// readString reads a string literal whose opening quote is at s.index. On
// success it returns the raw contents and leaves s.index after the closing
// quote.
//
// It fails on an unterminated string (s.index is left at the offending
// newline/EOF so the damage of unbalanced quotes is contained to one line)
// and on a template containing substitutions (s.index is left at the opening
// backtick so the main loop can rescan it as a template).
func (s *scanner) readString(quoteChar byte) ([]byte, bool) {
	src := s.src
	srcLen := len(src)
	open := s.index
	i := open + 1

	for i < srcLen {
		switch src[i] {
		case '\\':
			i += 2

		case quoteChar:
			s.index = i + 1
			return src[open+1 : i], true

		case '\n', '\r':
			if quoteChar != '`' {
				s.index = i
				return nil, false
			}
			i++

		case '$':
			if quoteChar == '`' && i+1 < srcLen && src[i+1] == '{' {
				s.index = open
				return nil, false
			}
			i++

		default:
			i++
		}
	}

	s.index = srcLen
	return nil, false
}

// skipString consumes a string literal, starting just after the opening
// quote. Unterminated ' / " strings stop at the newline so a stray quote
// can't swallow the rest of the file.
func (s *scanner) skipString(quoteChar byte) {
	src := s.src
	srcLen := len(src)

	for s.index < srcLen {
		switch src[s.index] {
		case '\\':
			s.index += 2

		case quoteChar:
			s.index++
			return

		case '\n', '\r':
			if quoteChar != '`' {
				return
			}
			s.index++

		default:
			s.index++
		}
	}
}

// scanTemplate consumes template literal text, starting just after the
// opening backtick (or just after the '}' that closed a substitution).
//
// On `${` it pushes a substitution frame and returns so the main loop
// scans the substitution as code.
func (s *scanner) scanTemplate() {
	src := s.src
	srcLen := len(src)

	for s.index < srcLen {
		switch src[s.index] {
		case '\\':
			s.index += 2

		case '`':
			s.index++
			return

		case '$':
			if s.index+1 < srcLen && src[s.index+1] == '{' {
				s.templates = append(s.templates, 0)
				s.index += 2
				return
			}
			s.index++

		default:
			s.index++
		}
	}
}

// slash handles '/' in code context: line comment, block comment, regex
// literal, or division.
func (s *scanner) slash() {
	src := s.src
	srcLen := len(src)

	if s.index+1 < srcLen {
		switch src[s.index+1] {
		case '/':
			if k := bytes.IndexByte(src[s.index:], '\n'); k >= 0 {
				s.index += k
			} else {
				s.index = srcLen
			}
			return

		case '*':
			if k := bytes.Index(src[s.index+2:], starSlash); k >= 0 {
				s.index += 2 + k + 2
			} else {
				s.index = srcLen
			}
			return
		}
	}

	if s.regexAllowed() {
		s.skipRegex()
	} else {
		s.index++
	}
}

// regexAllowed decides whether the '/' at s.index starts a regex literal or is a
// division operator, based on the last significant byte before it:
//   - after a value (identifier, ')', ']') it's division
//   - after an operator or a keyword like `return` it's a regex
//
// Misclassification is contained to one line because both skipRegex and skipString
// stop at newlines.
func (s *scanner) regexAllowed() bool {
	src := s.src
	i := s.index - 1

	for i >= 0 && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
		i--
	}
	if i < 0 {
		return true
	}

	char := src[i]
	if char == ')' || char == ']' {
		return false
	}

	if identChar[char] {
		j := i
		for j >= 0 && identChar[src[j]] {
			j--
		}

		switch string(src[j+1 : i+1]) {
		case "return", "typeof", "instanceof", "new", "in", "of", "do",
			"case", "else", "void", "delete", "throw", "yield", "await":
			return true
		}

		return false
	}

	return true
}

// skipRegex consumes a regex literal starting at the '/' at s.index. '/' inside
// a character class is literal. Stops at a newline (regexes can't contain
// raw newlines, so we were wrong about it being a regex).
func (s *scanner) skipRegex() {
	src := s.src
	srcLen := len(src)
	s.index++

	inClass := false
	for s.index < srcLen {
		switch src[s.index] {
		case '\\':
			s.index += 2
		case '[':
			inClass = true
			s.index++
		case ']':
			inClass = false
			s.index++
		case '/':
			s.index++
			if !inClass {
				return
			}
		case '\n', '\r':
			return
		default:
			s.index++
		}
	}
}

// scanBindingBraces consumes through the '}' closing an import/export
// clause, starting just after the '{'.
//
// Each binding is classified as a type binding (a leading `type`/`typeof`
// modifier, as in `{ type Foo, bar }`) or a value binding.
//
// A binding literally named `type` (e.g. `{ type }`, `{ type as alias }`,
// `{ type, x }`) is a value binding: `type` only acts as a modifier when
// followed by a binding name. Returns ok=false on EOF.
func (s *scanner) scanBindingBraces() (hasValue, hasType, ok bool) {
	src := s.src
	srcLen := len(src)

	atBindingStart := true
	for s.index < srcLen {
		switch char := src[s.index]; {
		case char == '}':
			s.index++
			return hasValue, hasType, true

		case char == ',':
			s.index++
			atBindingStart = true

		case char == '\'' || char == '"' || char == '`':
			// string binding name, e.g. `{ "string name" as alias }`
			s.index++
			s.skipString(char)

			if atBindingStart {
				hasValue = true
				atBindingStart = false
			}

		case char == '/':
			if s.index+1 < srcLen && (src[s.index+1] == '/' || src[s.index+1] == '*') {
				s.skipWsComments()
			} else {
				s.index++
			}

		case identChar[char]:
			word := string(s.readWord())
			if atBindingStart {
				if (word == "type" || word == "typeof") && s.bindingNameFollows() {
					hasType = true
				} else {
					hasValue = true
				}

				atBindingStart = false
			}

		default:
			s.index++
		}
	}

	return hasValue, hasType, false
}

// bindingNameFollows decides whether the `type` word just read inside import/
// export braces is a modifier (followed by a binding name) or itself a
// binding named `type` (followed by ',', '}', or `as`). The position is
// restored before returning.
func (s *scanner) bindingNameFollows() bool {
	save := s.index
	defer func() { s.index = save }()

	s.skipWsComments()
	if s.index >= len(s.src) {
		return false
	}

	switch char := s.src[s.index]; {
	case char == '\'' || char == '"':
		return true

	case identChar[char]:
		return string(s.readWord()) != "as"
	}

	return false
}

// skipWsComments advances past whitespace (including the common multi-byte
// JS whitespace runes) and comments.
func (s *scanner) skipWsComments() {
	src := s.src
	srcLen := len(src)

	for s.index < srcLen {
		switch src[s.index] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			s.index++

		case '/':
			if s.index+1 < srcLen && src[s.index+1] == '/' {
				if k := bytes.IndexByte(src[s.index:], '\n'); k >= 0 {
					s.index += k
				} else {
					s.index = srcLen
				}
			} else if s.index+1 < srcLen && src[s.index+1] == '*' {
				if k := bytes.Index(src[s.index+2:], starSlash); k >= 0 {
					s.index += 2 + k + 2
				} else {
					s.index = srcLen
				}
			} else {
				return
			}

		case 0xc2: // U+00A0 no-break space
			if s.index+1 < srcLen && src[s.index+1] == 0xa0 {
				s.index += 2
			} else {
				return
			}

		case 0xe2: // U+2028 / U+2029 line separators
			if s.index+2 < srcLen && src[s.index+1] == 0x80 && (src[s.index+2] == 0xa8 || src[s.index+2] == 0xa9) {
				s.index += 3
			} else {
				return
			}

		case 0xef: // U+FEFF zero-width no-break space
			if s.index+2 < srcLen && src[s.index+1] == 0xbb && src[s.index+2] == 0xbf {
				s.index += 3
			} else {
				return
			}

		default:
			return
		}
	}
}

// readWord consumes and returns the identifier at s.index (empty if s.index isn't at
// an identifier character).
func (s *scanner) readWord() []byte {
	src := s.src
	srcLen := len(src)
	startIdx := s.index

	for s.index < srcLen && identChar[src[s.index]] {
		s.index++
	}

	return src[startIdx:s.index]
}

// trySkipWord skips past the word at the current index and reports whether
// it matched the provided word. On a mismatch it rewinds the index back to
// where it was.
func (s *scanner) trySkipWord(word string) bool {
	origIdx := s.index
	matchedWord := string(s.readWord()) == word

	if !matchedWord {
		s.index = origIdx
	}
	return matchedWord
}

// boundaryBefore reports whether pos can start a keyword (i.e. the previous
// byte is not part of an identifier). Known multi-byte whitespace runes are
// accepted as boundaries even though their bytes look like identifier bytes.
func (s *scanner) boundaryBefore(pos int) bool {
	if pos == 0 {
		return true
	}

	src := s.src
	prevChar := src[pos-1]

	if !identChar[prevChar] {
		return true
	}
	if prevChar < 0x80 {
		return false
	}
	if prevChar == 0xa0 && pos >= 2 && src[pos-2] == 0xc2 {
		return true // U+00A0
	}
	if (prevChar == 0xa8 || prevChar == 0xa9) && pos >= 3 && src[pos-3] == 0xe2 && src[pos-2] == 0x80 {
		return true // U+2028 / U+2029
	}
	if prevChar == 0xbf && pos >= 3 && src[pos-3] == 0xef && src[pos-2] == 0xbb {
		return true // U+FEFF
	}

	return false
}

// precededByDot reports whether the keyword at pos is a property access like
// `obj.require(...)`. A `...` spread does not count.
func (s *scanner) precededByDot(pos int) bool {
	src := s.src
	i := pos - 1

	for i >= 0 && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
		i--
	}

	return i >= 0 && src[i] == '.' && !(i > 0 && src[i-1] == '.')
}

func (s *scanner) record(spec []byte, kind Kind, pos int) {
	s.res.Imports = append(s.res.Imports, Import{
		Specifier: string(spec),
		Kind:      kind,
		Line:      s.lineAt(pos),
	})
}

func (s *scanner) diag(msg string, pos int) {
	s.res.Diagnostics = append(s.res.Diagnostics, Diagnostic{
		Message: msg,
		Line:    s.lineAt(pos),
	})
}

// lineAt returns the 1-based line at byte offset pos. Records happen in
// source order, so it counts newlines incrementally from the previous call.
func (s *scanner) lineAt(pos int) int {
	if pos > len(s.src) {
		pos = len(s.src)
	}

	if pos < s.linePos {
		return 1 + bytes.Count(s.src[:pos], newline)
	}

	s.lineNum += bytes.Count(s.src[s.linePos:pos], newline)
	s.linePos = pos
	return s.lineNum
}
