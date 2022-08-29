package jsImportParser

import (
	"log"
	"os"
	"unicode/utf8"
)

var warnLog = log.New(os.Stderr, "[warning] ", 0)
var chars_as = []rune{'a', 's'}
var chars_export = []rune{'e', 'x', 'p', 'o', 'r', 't'}
var chars_from = []rune{'f', 'r', 'o', 'm'}
var chars_import = []rune{'i', 'm', 'p', 'o', 'r', 't'}
var chars_of = []rune{'o', 'f'}
var chars_require = []rune{'r', 'e', 'q', 'u', 'i', 'r', 'e'}
var chars_requireresolve = []rune{'r', 'e', 'q', 'u', 'i', 'r', 'e', '.', 'r', 'e', 's', 'o', 'l', 'v', 'e'}
var chars_type = []rune{'t', 'y', 'p', 'e'}

type stateStackItem struct {
	currentChar          rune
	currentCharIsEscaped bool
	currentLine          int
	isInsideString       bool
	pointer              int
	prevChar             rune
	prevCharIsEscaped    bool
}

type lexer struct {
	fileName string
	source   string

	currentChar          rune
	currentCharIsEscaped bool
	currentLine          int
	isInsideString       bool
	pointer              int
	prevChar             rune
	prevCharIsEscaped    bool
	stateStack           []stateStackItem
}

// GetImportsFromFile takes the string contents of a file and the filename (for logging purposes)
// and returns a slice of strings containing the modules imported by the file
func GetImportsFromFile(contents string, fileName string) []string {
	result := []string{}
	l := lexer{
		currentChar:          -1,
		currentCharIsEscaped: false,
		currentLine:          1,
		fileName:             fileName,
		isInsideString:       false,
		prevCharIsEscaped:    false,
		source:               contents,
		stateStack:           []stateStackItem{},
	}

	l.step()
parseLoop:
	for {
		switch l.currentChar {
		// EOF
		case -1:
			break parseLoop

		case 'r':
			var requireSrc string
			var ok bool

			l.pushState()
			requireSrc, ok = l.processDynamicImportOrRequire(chars_requireresolve)

			if !ok {
				l.popState()
				requireSrc, ok = l.processDynamicImportOrRequire(chars_require)
			}

			if ok {
				result = append(result, requireSrc)
			}

		case 'i':
			var importSrc string
			var ok bool

			l.pushState()
			importSrc, ok = l.processDynamicImportOrRequire(chars_import)

			if !ok {
				l.popState()
				importSrc, ok = l.processStaticImportStatement()
			}

			if ok {
				result = append(result, importSrc)
			}

		case 'e':
			if exportSrc, ok := l.processExportStatement(); ok {
				// for simplicity, we consider `export ... from 'foo'`
				// to just be an import of `foo`
				result = append(result, exportSrc)
			}

		default:
			l.step()
		}
	}

	return result
}

func (l *lexer) processDynamicImportOrRequire(fnChars []rune) (string, bool) {
	if l.prevChar != -1 && !isWhitespace(l.prevChar) && l.prevChar != ';' && l.prevChar != '=' && l.prevChar != '(' {
		l.step()
		return "", false
	}

	if ok := l.tryStepPastChars(fnChars); !ok {
		return "", false
	}

	if l.currentChar != '(' {
		return "", false
	}
	l.step()
	l.stepPastWhitespace()

	if !isQuote(l.currentChar) {
		return "", false
	}

	if result, ok := l.getImportSrcString(); ok {
		return result, true
	}

	return "", false
}

func (l *lexer) processStaticImportStatement() (string, bool) {
	if l.prevChar != -1 && !isNewline(l.prevChar) && l.prevChar != ';' {
		l.step()
		return "", false
	}

	if ok := l.tryStepPastChars(chars_import); !ok {
		return "", false
	}

	l.stepPastWhitespace()

	if !isQuote(l.currentChar) {
		// `import type ...`
		if ok := l.tryStepPastChars(chars_type); ok {
			// `import typeof ...`
			l.tryStepPastChars(chars_of)

			if isWhitespace(l.currentChar) {
				// just ignore if the entire import is types
				return "", false
			}
		}

		l.pushState()
		for {
			if l.currentChar == ',' {
				l.step()
				l.stepPastWhitespace()
			}

			switch l.currentChar {
			case -1:
				warnLog.Printf("Invalid import in %s:%d", l.fileName, l.currentLine)
				l.popState()
				l.step()
				return "", false
			case '{':
				if l.prevChar == '$' && !l.prevCharIsEscaped {
					// unsupported import inside template string
					l.step()
					return "", false
				} else {
					l.stepUntil('}')
					l.step()
				}
			case '*':
				l.step()
				l.stepPastWhitespace()
				if ok := l.tryStepPastChars(chars_as); ok {
					l.stepPastWhitespace()
				} else {
					// you can't do `import *` without ` as <something>`
					warnLog.Printf("Invalid import in %s:%d", l.fileName, l.currentLine)
					return "", false
				}
				// continue here so the `default` case will handle it in the next iteration
				continue
			default:
				l.stepUntilWhitespaceOrComma()
			}

			l.stepPastWhitespace()
			if l.currentChar != ',' {
				break
			}
		}

		if ok := l.tryStepPastChars(chars_from); !ok {
			// import with no `from`
			warnLog.Printf("Invalid import in %s:%d", l.fileName, l.currentLine)
			return "", false
		}

		l.stepPastWhitespace()
	}

	if !isQuote(l.currentChar) {
		warnLog.Printf("Invalid import in %s:%d", l.fileName, l.currentLine)
		return "", false
	}

	if result, ok := l.getImportSrcString(); ok {
		return result, true
	}

	return "", false
}

func (l *lexer) processExportStatement() (string, bool) {
	if l.prevChar != -1 && !isNewline(l.prevChar) && l.prevChar != ';' {
		l.step()
		return "", false
	}

	if ok := l.tryStepPastChars(chars_export); !ok {
		return "", false
	}

	l.stepPastWhitespace()
	if ok := l.tryStepPastChars(chars_type); ok {
		if isWhitespace(l.currentChar) {
			// just ignore if the entire export is types
			return "", false
		}
	}

	switch l.currentChar {
	case '{':
		l.stepUntil('}')
		l.step()
		l.stepPastWhitespace()
	case '*':
		l.step()
		l.stepPastWhitespace()
		if ok := l.tryStepPastChars(chars_as); ok {
			l.stepPastWhitespace()
			l.stepUntilWhitespace()
			l.stepPastWhitespace()
		}
	default:
		// if it doesn't fit any of the above cases, it's not
		// valid syntax for an export with `from` in it
		return "", false
	}

	if ok := l.tryStepPastChars(chars_from); !ok {
		// we're not interested if there's no `from`
		return "", false
	}

	l.stepPastWhitespace()
	if !isQuote(l.currentChar) {
		warnLog.Printf("Invalid export in %s:%d", l.fileName, l.currentLine)
		return "", false
	}

	if result, ok := l.getImportSrcString(); ok {
		return result, true
	}

	return "", false
}

func (l *lexer) getImportSrcString() (string, bool) {
	quoteChar := l.currentChar
	result := ""

	l.pushState()
	l.isInsideString = true
	l.step()
	for !(l.currentChar == quoteChar && !l.currentCharIsEscaped) {
		switch l.currentChar {
		case -1:
			l.popState()
			warnLog.Printf("Invalid import in %s:%d", l.fileName, l.currentLine)
			return "", false

		// dynamic import paths aren't supported
		case '{', '+':
			l.popState()
			warnLog.Printf("Unsupported dynamic import path in %s:%d", l.fileName, l.currentLine)
			l.isInsideString = false
			return "", false

		default:
			result += string(l.currentChar)
			l.step()
		}
	}

	l.step()
	l.isInsideString = false
	return result, true
}

// returns true only if it sees each of the provided chars along the way
func (l *lexer) tryStepPastChars(chars []rune) bool {
	for _, char := range chars {
		if l.currentChar != char {
			return false
		}

		l.step()
	}

	return true
}

func (l *lexer) stepPastWhitespace() {
	for isWhitespace(l.currentChar) {
		l.step()
	}
}

func (l *lexer) stepUntilWhitespace() {
	for !isWhitespace(l.currentChar) {
		l.step()
	}
}

func (l *lexer) stepUntilWhitespaceOrComma() {
	for !isWhitespace(l.currentChar) && l.currentChar != ',' {
		l.step()
	}
}

func (l *lexer) stepUntil(char rune) {
	for l.currentChar != char && l.currentChar != -1 {
		l.step()
	}
}

// similar to unix `pushd`/`popd`
func (l *lexer) pushState() {
	l.stateStack = append(l.stateStack, stateStackItem{
		l.currentChar,
		l.currentCharIsEscaped,
		l.currentLine,
		l.isInsideString,
		l.pointer,
		l.prevChar,
		l.prevCharIsEscaped,
	})
}

// similar to unix `pushd`/`popd`
func (l *lexer) popState() {
	if len(l.stateStack) == 0 {
		log.Fatal("Unable to 'popState' because the stack is empty")
	}

	lastStackIndex := len(l.stateStack) - 1
	poppedState := l.stateStack[lastStackIndex]
	l.stateStack = l.stateStack[:lastStackIndex]

	l.currentChar = poppedState.currentChar
	l.currentCharIsEscaped = poppedState.currentCharIsEscaped
	l.currentLine = poppedState.currentLine
	l.isInsideString = poppedState.isInsideString
	l.pointer = poppedState.pointer
	l.prevChar = poppedState.prevChar
	l.prevCharIsEscaped = poppedState.prevCharIsEscaped
}

// step forward to the next char in the source file
//
// the `dryRun` param can be used to see what the next char is
// without actually taking a step
func (l *lexer) _step(dryRun ...bool) rune {
	nextChar, width := utf8.DecodeRuneInString(l.source[l.pointer:])

	// Use -1 to indicate the end of the file
	if width == 0 {
		nextChar = -1
	}

	if len(dryRun) == 0 || dryRun[0] == false {
		if (nextChar == '\n' && l.prevChar != '\r') || nextChar == '\r' {
			l.currentLine += 1
		}

		l.prevChar = l.currentChar
		l.prevCharIsEscaped = l.currentCharIsEscaped
		l.currentCharIsEscaped = nextChar != -1 && l.prevChar == '\\' && !l.currentCharIsEscaped
		l.currentChar = nextChar
		l.pointer += width
	}

	return nextChar
}

// Wrapper around the real `_step` fn that can skip past comments it comes
// across. This is needed because it's valid syntax to scatter comments
// among the keywords in an import/export, and we don't want to have to
// worry about running into comments when we're parsing them
func (l *lexer) step() {
	l._step()

	// auto-skip past comments as long as we're not inside a string
	if !l.isInsideString {
	commentLoop:
		for {
			switch l.currentChar {
			case -1:
				break commentLoop

			// line comments
			case '/':
				if !l.currentCharIsEscaped && l._step(true) == '/' {
					for !isNewline(l.currentChar) {
						if l.currentChar == -1 {
							break commentLoop
						}
						l._step()
					}
					l._step()
				} else {
					break commentLoop
				}

			// block comments
			case '*':
				if l.prevChar == '/' {
					for !(l.currentChar == '/' && l.prevChar == '*') {
						if l.currentChar == -1 {
							break commentLoop
						}
						l._step()
					}
					l._step()
				} else {
					break commentLoop
				}

			default:
				break commentLoop
			}
		}
	}
}

func isQuote(input rune) bool {
	return input == '\'' || input == '"' || input == '`'
}

func isWhitespace(input rune) bool {
	return isNewline(input) || isSpaceOrTab(input)
}

func isNewline(input rune) bool {
	switch input {
	case '\r', '\n', '\u2028', '\u2029':
		return true
	default:
		return false
	}
}

func isSpaceOrTab(input rune) bool {
	switch input {
	case '\t', ' ':
		return true
	default:
		return false
	}
}
