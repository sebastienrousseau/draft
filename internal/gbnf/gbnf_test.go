// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package gbnf

import (
	"regexp"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/claims"
)

// str is a JSON-Schema string node.
func str() map[string]any { return map[string]any{"type": "string"} }

func TestClaimSchemaCompiles(t *testing.T) {
	g, err := FromJSONSchema(claims.Schema())
	if err != nil {
		t.Fatal(err)
	}
	// The start symbol and the shared lexical rules are present.
	for _, want := range []string{
		"root ::= arr0",
		`ws ::= [ \t\n]*`,
		`string ::= "\"" char* "\""`,
	} {
		if !strings.Contains(g, want) {
			t.Errorf("grammar missing %q:\n%s", want, g)
		}
	}
	// Every enum value appears as a quoted JSON literal.
	for _, v := range append(append([]string{}, claims.ClaimTypes...), claims.ClaimStrengths...) {
		lit := `"\"` + v + `\""`
		if !strings.Contains(g, lit) {
			t.Errorf("grammar missing enum literal %s", lit)
		}
	}
}

// The object rule must emit properties in required order: claim, its quote,
// then type and strength — the order the ledger and grammar share.
func TestObjectPropertyOrder(t *testing.T) {
	g, err := FromJSONSchema(claims.Schema())
	if err != nil {
		t.Fatal(err)
	}
	obj := ruleBody(t, g, "obj0")
	positions := []string{`"\"claim\""`, `"\"source_quote\""`, `"\"type\""`, `"\"strength\""`}
	last := -1
	for _, p := range positions {
		at := strings.Index(obj, p)
		if at < 0 {
			t.Fatalf("obj0 missing property %s", p)
		}
		if at < last {
			t.Errorf("property %s is out of order in obj0", p)
		}
		last = at
	}
}

// A grammar that references an undefined rule is unusable. Every identifier on
// a right-hand side must be a defined rule.
func TestGrammarIsClosed(t *testing.T) {
	g, err := FromJSONSchema(claims.Schema())
	if err != nil {
		t.Fatal(err)
	}
	defined := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(g), "\n") {
		name, _, ok := strings.Cut(line, " ::= ")
		if !ok {
			t.Fatalf("malformed rule line: %q", line)
		}
		defined[strings.TrimSpace(name)] = true
	}
	ident := regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`)
	for _, line := range strings.Split(strings.TrimSpace(g), "\n") {
		_, body, _ := strings.Cut(line, " ::= ")
		for _, ref := range ident.FindAllString(stripTerminals(body), -1) {
			if !defined[ref] {
				t.Errorf("rule references undefined %q in: %s", ref, line)
			}
		}
	}
}

func TestDeterministic(t *testing.T) {
	a, _ := FromJSONSchema(claims.Schema())
	b, _ := FromJSONSchema(claims.Schema())
	if a != b {
		t.Error("compilation is not deterministic")
	}
}

// A quote or backslash in an enum value must be JSON- then GBNF-escaped so the
// terminal still matches exactly the bytes the model emits.
func TestEnumValueEscaping(t *testing.T) {
	schema := map[string]any{
		"type": "string",
		"enum": []any{`a"b`, `c\d`},
	}
	g, err := FromJSONSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"\"a\\\"b\""`, `"\"c\\\\d\""`} {
		if !strings.Contains(g, want) {
			t.Errorf("missing escaped literal %s in:\n%s", want, g)
		}
	}
}

func TestNestedObjectAndSortedExtraProps(t *testing.T) {
	// Two required props in a fixed order, plus a non-required one that must
	// sort after them; a nested object value exercises recursion.
	schema := map[string]any{
		"type":     "object",
		"required": []any{"b", "a"},
		"properties": map[string]any{
			"a": str(),
			"b": map[string]any{"type": "object", "required": []any{"x"},
				"properties": map[string]any{"x": str()}},
			"z": str(),
		},
	}
	g, err := FromJSONSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	// The nested object b is compiled depth-first, so it is obj0 and the outer
	// object is obj1.
	if !strings.Contains(g, "obj0 ::=") {
		t.Errorf("nested object rule not emitted:\n%s", g)
	}
	obj := ruleBody(t, g, "obj1")
	// required order b, a; then z (sorted, not required).
	bi, ai, zi := strings.Index(obj, `"\"b\""`), strings.Index(obj, `"\"a\""`), strings.Index(obj, `"\"z\""`)
	if bi >= ai || ai >= zi {
		t.Errorf("property order wrong: b=%d a=%d z=%d\n%s", bi, ai, zi, obj)
	}
}

func TestErrorPaths(t *testing.T) {
	cases := map[string]map[string]any{
		"unsupported type":       {"type": "number"},
		"empty type":             {},
		"enum not array":         {"enum": "nope"},
		"empty enum":             {"enum": []any{}},
		"enum non-string":        {"enum": []any{1}},
		"array without items":    {"type": "array"},
		"object without props":   {"type": "object"},
		"object empty props":     {"type": "object", "properties": map[string]any{}},
		"required not string":    {"type": "object", "required": []any{1}, "properties": map[string]any{"a": str()}},
		"required undeclared":    {"type": "object", "required": []any{"missing"}, "properties": map[string]any{"a": str()}},
		"property not a schema":  {"type": "object", "properties": map[string]any{"a": "nope"}},
		"array item unsupported": {"type": "array", "items": map[string]any{"type": "number"}},
		"object value unsupported": {"type": "object", "required": []any{"a"},
			"properties": map[string]any{"a": map[string]any{"type": "number"}}},
	}
	for name, schema := range cases {
		if _, err := FromJSONSchema(schema); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

// ruleBody returns the right-hand side of a named rule, failing if absent.
func ruleBody(t *testing.T, grammar, name string) string {
	t.Helper()
	for _, line := range strings.Split(grammar, "\n") {
		if n, body, ok := strings.Cut(line, " ::= "); ok && strings.TrimSpace(n) == name {
			return body
		}
	}
	t.Fatalf("rule %q not found in:\n%s", name, grammar)
	return ""
}

// stripTerminals removes GBNF string terminals and character classes so only
// rule-name identifiers remain for the closure check.
func stripTerminals(body string) string {
	terminal := regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	class := regexp.MustCompile(`\[[^\]]*\]`)
	return class.ReplaceAllString(terminal.ReplaceAllString(body, " "), " ")
}
