// `import type`/`import typeof` are reported as type-only imports
import type Foo from 'TYPE-1';
import typeof Bar from 'TYPE-2';
import type { Baz } from 'TYPE-3';
// a `type` modifier inside the braces makes that binding a type import
import { type Qux } from 'TYPE-4';
// mixed imports are reported as both a value and a type import
import { type Foo2, bar } from 'MIXED-1';
// a binding literally named `type` is a value import
import { type } from 'VALUE-1';
import { type as alias } from 'VALUE-2';
// `type` used as a default binding name
import type from 'VALUE-3';
// type-only re-exports
export type { T } from 'TYPE-5';
export { type A } from 'TYPE-6';
// mixed re-export
export { type B, c } from 'MIXED-2';
// TS type alias, not an import
export type Alias = string;
