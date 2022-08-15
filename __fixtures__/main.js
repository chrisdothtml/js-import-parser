// different variations of imports
// ref: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/import#syntax
import defaultExport from "INCLUDED-1";
import * as name from "INCLUDED-2";
import { export1 } from "INCLUDED-3";
import { export1 as alias1 } from "INCLUDED-4";
import { default as alias } from "INCLUDED-5";
import { export1, export2 } from "INCLUDED-6";
import { export1, export2 as alias2, /* … */ } from "INCLUDED-7";
import { "string name" as alias } from "INCLUDED-8";
import defaultExport, { export1, /* … */ } from "INCLUDED-9";
import defaultExport, * as name from "INCLUDED-10";
import "INCLUDED-11";

// different variations of exports
// ref: https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/export#syntax
export let name1, name2/*, … */;
export const name1 = 1, name2 = 2/*, … */;
export function functionName() { /* … */ }
export class ClassName { /* … */ }
export function* generatorFunctionName() { /* … */ }
export const { name1, name2: bar } = o;
export const [ name1, name2 ] = array;
export { name1, /* …, */ nameN };
export { variable1 as name1, variable2 as name2, /* …, */ nameN };
export { variable1 as "string name" };
export { name1 as default /*, … */ };
export default expression;
export default function functionName() { /* … */ }
export default class ClassName { /* … */ }
export default function* generatorFunctionName() { /* … */ }
export default function () { /* … */ }
export default class { /* … */ }
export default function* () { /* … */ }
export * from "INCLUDED-12";
export * as name1 from "INCLUDED-13";
export { name1, /* …, */ nameN } from "INCLUDED-14";
export { import1 as name1, import2 as name2, /* …, */ nameN } from "INCLUDED-15";
export { default, /* …, */ } from "INCLUDED-16";

// formatting variations
const foo = `import {foo} from 'EXCLUDED-1'`
const foo = `
// unfortunately imports in strings will be included if there's a preceeding newline
import {foo} from 'INCLUDED-17'
// but not ones with dynamic paths
import {foo} from 'EXCLUDED-${bar}'`
const foo = require('INCLUDED-18')
const foo = await import('INCLUDED-19')

const renamedRequire = require;
// sorry, doesn't work
const foo = renamedRequire('EXCLUDED-2')
