# js-import-parser

> A fast parser for analyzing relationships between JavaScript modules

The purpose of this package is to quickly parse JavaScript/TypeScript/Flow files and return the list of modules it imports (other local modules, external dependencies, etc.). For context, it was written to be used in a large monorepo to very quickly determine the specific test files that need to be run given a list of changed files.

It is not a full JavaScript parser. It's a single-pass lexical scanner that tracks just enough context to know when it's looking at real code (as opposed to strings or comments), and only then matches `import`/`export`/`require` syntax. This keeps it small and fast, while very effective for its purpose.

## Usage

```go
package main

import (
	"fmt"
	"os"

	jsimports "github.com/chrisdothtml/js-import-parser"
)

func main() {
	src, err := os.ReadFile("foo.js")
	if err != nil {
		panic(err)
	}

	result := jsimports.Parse(src)
	for _, imp := range result.Imports {
		// e.g. `lodash (static, line 3)`
		fmt.Printf("%s (%s, line %d)\n", imp.Specifier, imp.Kind, imp.Line)
	}
	for _, d := range result.Diagnostics {
		fmt.Printf("warning: %s on line %d\n", d.Message, d.Line)
	}
}
```

`Parse` performs no I/O and the returned specifiers are copies, so the source buffer can be reused. When parsing many files, run one `Parse` per goroutine (the scanner has no shared state).

### What gets detected

| Syntax | Kind |
| --- | --- |
| `import ... from "x"`, `import "x"` | `static` |
| `import("x")` | `dynamic` |
| `require("x")` | `require` |
| `require.resolve("x")` | `require.resolve` |
| `export ... from "x"` | `re-export` (it creates a dependency on `x`) |
| `import type ... from "x"` | `type-static` |
| `export type { T } from "x"` | `type-re-export` |

Non-literal module paths (`require(someVar)`, `import('./' + name)`, `` import(`./${name}`) ``) can't be statically analyzed. They are skipped and reported as `Diagnostics` instead.

### TypeScript/Flow

Type syntax doesn't need to be fully understood to find imports, so `.ts`/`.tsx`/Flow files work as-is. Type-only imports are reported with their own kinds so the consumer can decide how to treat them:

- `import type ... from 'x'`, `import typeof ... from 'x'`, and `import { type Foo } from 'x'` → `KindTypeStatic`
- `export type { T } from 'x'` and `export { type T } from 'x'` → `KindTypeReExport`
- Mixed clauses like `import { type Foo, bar } from 'x'` are recorded **twice** (once as the value kind and once as the type kind), so consumers can filter on either axis.
- A binding literally named `type` (`import { type } from 'x'`, `import type from 'x'`) is correctly treated as a value import.

See [types.js fixture](__fixtures__/types.js) for the full matrix.

## Caveats

The scanner assumes the input is valid JavaScript; it does not validate syntax. Known limitations:

- **Renamed `require`**: `const r = require; r('x')` is not detected, and any call to an identifier literally named `require` is reported whether or not it's the CommonJS global. (Property accesses like `obj.require('x')` are correctly ignored.)
- **Regex vs. division** is decided with a one-token lookbehind heuristic rather than full parsing. A misclassification is contained to a single line (string and regex scanning stop at newlines), but a pathological regex could in principle hide an import on the same line.
- Lone `\r` line endings are not counted in line numbers; `\n` and `\r\n` are.

See [tricky.js fixture](__fixtures__/tricky.js) for the cases the context tracking handles: comment-like text inside strings (`"//"`, globs like `"src/**/*.js"`), quotes inside regexes, imports inside template substitutions, concatenated paths, and more.

## License

[MIT](LICENSE)
