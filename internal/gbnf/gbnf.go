// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package gbnf compiles the subset of JSON Schema that the claim schema uses
// into a GBNF grammar, the format llama.cpp (and Ollama through it) accepts to
// constrain a model's output token by token.
//
// The point is correctness, not generality: constrained decoding makes a
// malformed extraction impossible rather than repaired after the fact, which
// is the last non-grounding way the claim step can lose a good claim. The
// compiler therefore supports exactly what claims.Schema() emits — arrays,
// objects with required string and enum properties — and returns an error for
// anything outside that, so an unsupported schema fails loudly instead of
// producing a grammar that silently permits the wrong shape.
package gbnf

import (
	"fmt"
	"sort"
	"strings"
)

// FromJSONSchema compiles a JSON Schema (as produced by json.Unmarshal into
// map[string]any, or built directly) into a GBNF grammar string.
//
// The grammar's start symbol is always "root". Shared lexical rules (a JSON
// string, and whitespace) are emitted once and only when referenced. The
// output is deterministic: the same schema always compiles to the same
// grammar, byte for byte, so it can be checked in as a golden fixture.
func FromJSONSchema(schema map[string]any) (string, error) {
	c := &compiler{rules: map[string]string{}, counts: map[string]int{}}
	root, err := c.compile(schema)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "root ::= %s\n", root)
	for _, name := range c.order {
		fmt.Fprintf(&b, "%s ::= %s\n", name, c.rules[name])
	}
	return b.String(), nil
}

// compiler accumulates named rules in creation order so the output is stable.
type compiler struct {
	rules  map[string]string
	order  []string
	counts map[string]int
}

// define registers a rule body under a fresh name derived from kind (obj0,
// enum0, enum1, ...) and returns the name. Shared rules (string, ws) are
// registered under their own stable name via defineNamed and never duplicated.
func (c *compiler) define(kind, body string) string {
	name := fmt.Sprintf("%s%d", kind, c.counts[kind])
	c.counts[kind]++
	c.defineNamed(name, body)
	return name
}

// defineNamed registers body under an exact name, once.
func (c *compiler) defineNamed(name, body string) {
	if _, ok := c.rules[name]; ok {
		return
	}
	c.rules[name] = body
	c.order = append(c.order, name)
}

// compile returns the rule name (or inline terminal) that matches node, and
// registers any rules it needs.
func (c *compiler) compile(node map[string]any) (string, error) {
	// An enum is a fixed set of literal values regardless of its declared
	// type, so it is matched before the type switch.
	if raw, ok := node["enum"]; ok {
		return c.compileEnum(raw)
	}
	typ, _ := node["type"].(string)
	switch typ {
	case "array":
		return c.compileArray(node)
	case "object":
		return c.compileObject(node)
	case "string":
		return c.stringRule(), nil
	default:
		return "", fmt.Errorf("gbnf: unsupported schema type %q", typ)
	}
}

// compileEnum turns an enum into an alternation of quoted string literals.
func (c *compiler) compileEnum(raw any) (string, error) {
	vals, ok := raw.([]any)
	if !ok || len(vals) == 0 {
		return "", fmt.Errorf("gbnf: enum must be a non-empty array, got %T", raw)
	}
	alts := make([]string, len(vals))
	for i, v := range vals {
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("gbnf: enum value %d is %T, want string", i, v)
		}
		alts[i] = jsonStringLiteral(s)
	}
	return c.define("enum", strings.Join(alts, " | ")), nil
}

// compileArray produces a rule for a possibly-empty, comma-separated JSON
// array of the item schema.
func (c *compiler) compileArray(node map[string]any) (string, error) {
	items, ok := node["items"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("gbnf: array schema needs an items object")
	}
	item, err := c.compile(items)
	if err != nil {
		return "", err
	}
	ws := c.wsRule()
	body := fmt.Sprintf(`"[" %s ( %s ( %s "," %s %s )* )? %s "]"`, ws, item, ws, ws, item, ws)
	return c.define("arr", body), nil
}

// compileObject produces a rule matching a JSON object with exactly the
// schema's properties, emitted in required order (then any remaining property
// names sorted, so the output stays deterministic).
func (c *compiler) compileObject(node map[string]any) (string, error) {
	props, ok := node["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		return "", fmt.Errorf("gbnf: object schema needs a non-empty properties map")
	}
	order, err := propertyOrder(node, props)
	if err != nil {
		return "", err
	}
	ws := c.wsRule()
	tokens := []string{`"{"`, ws}
	for i, name := range order {
		valSchema, ok := props[name].(map[string]any)
		if !ok {
			return "", fmt.Errorf("gbnf: property %q is not a schema object", name)
		}
		val, err := c.compile(valSchema)
		if err != nil {
			return "", err
		}
		if i > 0 {
			tokens = append(tokens, ws, `","`, ws)
		}
		tokens = append(tokens, jsonStringLiteral(name), ws, `":"`, ws, val)
	}
	tokens = append(tokens, ws, `"}"`)
	return c.define("obj", strings.Join(tokens, " ")), nil
}

// propertyOrder lists the object's properties in the order the grammar emits
// them: the required array first (in its own order), then any property not in
// required, sorted. Every required entry must be a declared property.
func propertyOrder(node map[string]any, props map[string]any) ([]string, error) {
	var order []string
	seen := map[string]bool{}
	if req, ok := node["required"].([]any); ok {
		for _, r := range req {
			name, ok := r.(string)
			if !ok {
				return nil, fmt.Errorf("gbnf: required entry is %T, want string", r)
			}
			if _, ok := props[name]; !ok {
				return nil, fmt.Errorf("gbnf: required property %q is not declared", name)
			}
			order = append(order, name)
			seen[name] = true
		}
	}
	var rest []string
	for name := range props {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(order, rest...), nil
}

// stringRule defines the shared JSON string rule and returns its name.
func (c *compiler) stringRule() string {
	c.defineNamed("hex", `[0-9a-fA-F]`)
	c.defineNamed("char", `[^"\\] | "\\" ( ["\\/bfnrt] | "u" hex hex hex hex )`)
	c.defineNamed("string", `"\"" char* "\""`)
	return "string"
}

// wsRule defines the shared whitespace rule and returns its name.
func (c *compiler) wsRule() string {
	c.defineNamed("ws", `[ \t\n]*`)
	return "ws"
}

// jsonStringLiteral renders s as a single GBNF string terminal that matches
// the JSON encoding of s — the double-quoted, escaped bytes a model must emit
// for that value. The surrounding JSON quotes appear as \" inside the GBNF
// terminal; a backslash or quote within s is first JSON-escaped (to \\ or \")
// and then each of those bytes is GBNF-escaped again, so the terminal matches
// exactly the two bytes the model would produce.
func jsonStringLiteral(s string) string {
	var b strings.Builder
	b.WriteString(`"\"`) // GBNF terminal open, then the JSON opening quote
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\\\`) // JSON \\ , GBNF-escaped
		case '"':
			b.WriteString(`\\\"`) // JSON \" , GBNF-escaped
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString(`\""`) // JSON closing quote, then GBNF terminal close
	return b.String()
}
