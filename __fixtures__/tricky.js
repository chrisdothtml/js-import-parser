#!/usr/bin/env node
// cases that trip up naive scanners

// '//' inside a string is not a comment
const slashes = "//"; const r1 = require('INCLUDED-1');
// '/*' inside a string (e.g. a glob) is not a comment
const glob = "src/**/*.js";
import a from 'INCLUDED-2';
// quotes inside a regex literal don't open a string
const re = /['"]/; const r2 = require('INCLUDED-3');
// requires as array elements / object values
const deps = [require('INCLUDED-4'), require('INCLUDED-5')];
const obj = { dep: require('INCLUDED-6') };
// '+' is legal in a literal path (SvelteKit route files)
const page = await import('./+INCLUDED-7.js');
// code inside template substitutions is still code
const tpl = `${require('INCLUDED-8')}`;
// concatenation isn't statically analyzable
const concat = require('EXCLUDED-1' + suffix);
// only the exact identifiers count
notRequire('EXCLUDED-2');
obj.require('EXCLUDED-3');
console.log(import.meta.url);
// comments and newlines inside the call
const multi = require(
  // a comment inside the call
  'INCLUDED-9'
);
import {
  one,
  two as three,
} from 'INCLUDED-10';
// type-only re-exports and imports get their own kinds
export type { T } from 'INCLUDED-TYPE-1';
import type { U } from 'INCLUDED-TYPE-2';
const last = require("INCLUDED-11")
