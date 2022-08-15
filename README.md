# js-import-parser

> A fast parser for analyzing relationships between JavaScript modules

The purpose of this package is to quickly parse a JavaScript file and return a list of modules it imports (other local modules, external dependencies, etc.). For context, it was written to be used in a large monorepo to very quickly determine the specific test files that need to be run given a list of changed files.

This is not a fully robust JavaScript parser, so it will work best in codebases that entirely use static imports/exports and don't rely on bundler magic. See the caveats below for more info on this.

## Caveats

Since this is a lazy parser (as opposed to a full ast parser/analyzer), it's unaware of most of the syntax in JavaScript files, and only concerns itself with the syntax related to module imports. It assumes your files contain valid Javascript (but does not enforce it).

While this allows it to be compact and very fast, it's not perfect and can be tricked. One example is if valid import code exists inside a string within your file, it will be included in the list of imports (see [main.js fixture](__fixtures__/main.js) for examples of this). Another example is if `require` is reassigned or renamed to something else; these cases will not work as expected (any existence of `require('...')` in your code will be included, regardless of if it actually imports a module at runtime).

### Typescript/Flow

One upside to the lazy parsing is that it can easily support Typescript or Flow files without having to be aware of the full language specs. One thing to note is that `import type ...` imports will be excluded from the list, whereas `import { type Foo, ... }` imports will still be included (as they may also contain non-type imports). See [types.js fixture](__fixtures__/types.js) for examples of this.

### Exports

Since `export * from '...'` creates a dependency from this module to the module being exported, these are also included in the list of imports for the file.

### Dynamic imports

Dynamic imports are supports, however, dynamic import *paths* are not.

```js
// supported
const component = await import('./component.js');
// NOT supported
const component = await import(`./${componentName}.js`);
```

## Usage

Here is the simplest usage example (when parsing a large number of files you'll likely want to use multi-threading though):

```go
package main

import (
  "fmt"
  "io/ioutil"
  "log"

  parser "github.com/chrisdothtml/js-import-parser"
)

func main() {
  filePath := "/.../foo.js"
  importedModules := parser.GetImportsFromFile(readFile(filePath), filePath)

  // prints list of imports for foo.js
  fmt.Printf("%s:\n", filePath)
  for _, importString := range importedModules {
    fmt.Printf("  %s\n", importString)
  }
}

func readFile(filePath string) string {
  content, err := ioutil.ReadFile(filePath)
  if err != nil {
    log.Fatal("Error reading file: ", err)
  }

  return string(content)
}
```

## License

[MIT](LICENSE)
