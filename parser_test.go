package jsimports

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func parseWithTimeout(t *testing.T, src []byte) Result {
	t.Helper()

	done := make(chan Result, 1)
	go func() { done <- Parse(src) }()

	select {
	case r := <-done:
		return r

	case <-time.After(5 * time.Second):
		t.Fatal("Parse did not terminate")
		return Result{}
	}
}

func parseFixture(t *testing.T, name string) Result {
	t.Helper()

	src, err := os.ReadFile(filepath.Join("__fixtures__", name))
	if err != nil {
		t.Fatal(err)
	}

	return parseWithTimeout(t, src)
}

func assertImports(t *testing.T, got []Import, want []Import) {
	t.Helper()

	for i := 0; i < len(got) && i < len(want); i++ {
		if got[i].Specifier != want[i].Specifier || got[i].Kind != want[i].Kind {
			t.Errorf("import %d: got %q (%s), want %q (%s)",
				i, got[i].Specifier, got[i].Kind, want[i].Specifier, want[i].Kind)
		}
	}

	if len(got) != len(want) {
		t.Errorf("got %d imports, want %d: %v", len(got), len(want), got)
	}

	for _, imp := range got {
		if strings.Contains(imp.Specifier, "EXCLUDED") {
			t.Errorf("excluded import leaked into result: %q", imp.Specifier)
		}
	}
}

func TestMainFixture(t *testing.T) {
	res := parseFixture(t, "main.js")

	var want []Import
	add := func(n int, kind Kind) {
		want = append(want, Import{Specifier: fmt.Sprintf("INCLUDED-%d", n), Kind: kind})
	}

	for n := 1; n <= 11; n++ {
		add(n, KindStatic)
	}
	for n := 12; n <= 16; n++ {
		add(n, KindReExport)
	}
	add(17, KindRequire)
	add(18, KindRequireResolve)
	add(19, KindRequireResolve) // the inner require.resolve of require(require.resolve(...))
	add(20, KindDynamic)

	assertImports(t, res.Imports, want)
	// the outer require(require.resolve(...)) has a non-literal argument
	if len(res.Diagnostics) != 1 {
		t.Errorf("want 1 diagnostic, got %v", res.Diagnostics)
	}
}

func TestTrickyFixture(t *testing.T) {
	res := parseFixture(t, "tricky.js")

	assertImports(t, res.Imports, []Import{
		{Specifier: "INCLUDED-1", Kind: KindRequire},
		{Specifier: "INCLUDED-2", Kind: KindStatic},
		{Specifier: "INCLUDED-3", Kind: KindRequire},
		{Specifier: "INCLUDED-4", Kind: KindRequire},
		{Specifier: "INCLUDED-5", Kind: KindRequire},
		{Specifier: "INCLUDED-6", Kind: KindRequire},
		{Specifier: "./+INCLUDED-7.js", Kind: KindDynamic},
		{Specifier: "INCLUDED-8", Kind: KindRequire},
		{Specifier: "INCLUDED-9", Kind: KindRequire},
		{Specifier: "INCLUDED-10", Kind: KindStatic},
		{Specifier: "INCLUDED-TYPE-1", Kind: KindTypeReExport},
		{Specifier: "INCLUDED-TYPE-2", Kind: KindTypeStatic},
		{Specifier: "INCLUDED-11", Kind: KindRequire},
	})

	// the concatenated require path
	if len(res.Diagnostics) != 1 {
		t.Errorf("want 1 diagnostic, got %v", res.Diagnostics)
	}
}

func TestTypesFixture(t *testing.T) {
	res := parseFixture(t, "types.js")

	assertImports(t, res.Imports, []Import{
		{Specifier: "TYPE-1", Kind: KindTypeStatic},
		{Specifier: "TYPE-2", Kind: KindTypeStatic},
		{Specifier: "TYPE-3", Kind: KindTypeStatic},
		{Specifier: "TYPE-4", Kind: KindTypeStatic},
		// mixed: recorded as both kinds
		{Specifier: "MIXED-1", Kind: KindStatic},
		{Specifier: "MIXED-1", Kind: KindTypeStatic},
		{Specifier: "VALUE-1", Kind: KindStatic},
		{Specifier: "VALUE-2", Kind: KindStatic},
		{Specifier: "VALUE-3", Kind: KindStatic},
		{Specifier: "TYPE-5", Kind: KindTypeReExport},
		{Specifier: "TYPE-6", Kind: KindTypeReExport},
		{Specifier: "MIXED-2", Kind: KindReExport},
		{Specifier: "MIXED-2", Kind: KindTypeReExport},
	})

	if len(res.Diagnostics) != 0 {
		t.Errorf("want 0 diagnostics, got %v", res.Diagnostics)
	}
}

func TestNoImportsFixture(t *testing.T) {
	res := parseFixture(t, "no-imports.js")
	assertImports(t, res.Imports, nil)
}

func TestLineNumbers(t *testing.T) {
	src := "import a from 'a';\n\nconst b = require('b');\nconst c = `${import('c')}`;\n"
	res := parseWithTimeout(t, []byte(src))

	wantLines := []int{1, 3, 4}
	if len(res.Imports) != len(wantLines) {
		t.Fatalf("got %v", res.Imports)
	}
	for i, want := range wantLines {
		if res.Imports[i].Line != want {
			t.Errorf("import %q: got line %d, want %d", res.Imports[i].Specifier, res.Imports[i].Line, want)
		}
	}
}

// truncated/malformed files must terminate, never hang or panic
func TestTruncatedInputs(t *testing.T) {
	inputs := []string{
		"import foo",
		"export * as name1",
		"import {a",
		"import {a} from",
		"import {a} from '",
		"require('x",
		"require(",
		"export {a} from",
		"const s = 'abc",
		"/* unclosed",
		"// unclosed",
		"`unclosed ${",
		"`unclosed ${ require('x'",
		"import",
		"export",
		"require",
		"/",
		"\\",
		"import * ",
		"export * from",
	}

	for _, in := range inputs {
		parseWithTimeout(t, []byte(in))
	}
}

func TestBOM(t *testing.T) {
	src := "\uFEFFimport a from 'real-dep';\n"
	res := parseWithTimeout(t, []byte(src))

	assertImports(t, res.Imports, []Import{{Specifier: "real-dep", Kind: KindStatic}})
}

func TestCRLF(t *testing.T) {
	src := "import a from 'a';\r\nconst b = require('b');\r\n"
	res := parseWithTimeout(t, []byte(src))

	assertImports(t, res.Imports, []Import{
		{Specifier: "a", Kind: KindStatic},
		{Specifier: "b", Kind: KindRequire},
	})
	if res.Imports[1].Line != 2 {
		t.Errorf("got line %d, want 2", res.Imports[1].Line)
	}
}

func TestMinified(t *testing.T) {
	src := `var a=require("a"),b=require("b");import{x}from"c";export{y}from"d";Promise.all([import("e"),import("f")])`
	res := parseWithTimeout(t, []byte(src))

	assertImports(t, res.Imports, []Import{
		{Specifier: "a", Kind: KindRequire},
		{Specifier: "b", Kind: KindRequire},
		{Specifier: "c", Kind: KindStatic},
		{Specifier: "d", Kind: KindReExport},
		{Specifier: "e", Kind: KindDynamic},
		{Specifier: "f", Kind: KindDynamic},
	})
}
