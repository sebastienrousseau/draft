// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package claims

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSchemaMarshalsAndDescribesAClaimList(t *testing.T) {
	s := Schema()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Schema() does not marshal to JSON: %v", err)
	}
	// Round-trip so the assertions below run against parsed JSON, not the
	// Go literal, catching anything the marshal step would change.
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	// The root is an object holding the list, not the list itself: Claude's
	// --json-schema and OpenAI structured outputs (Codex --output-schema)
	// both reject a schema whose root is not an object.
	if got["type"] != "object" || got["additionalProperties"] != false {
		t.Errorf("root = type %v, additionalProperties %v; want a closed object", got["type"], got["additionalProperties"])
	}
	if req, _ := got["required"].([]any); !reflect.DeepEqual(req, []any{"claims"}) {
		t.Errorf("root required = %v, want [claims]", got["required"])
	}
	list, ok := got["properties"].(map[string]any)["claims"].(map[string]any)
	if !ok || list["type"] != "array" {
		t.Fatalf("claims property = %v, want an array", got["properties"])
	}
	items, ok := list["items"].(map[string]any)
	if !ok {
		t.Fatalf("items is %T, want object", got["items"])
	}
	if items["type"] != "object" {
		t.Errorf("items.type = %v, want object", items["type"])
	}
	if items["additionalProperties"] != false {
		t.Errorf("items.additionalProperties = %v, want false", items["additionalProperties"])
	}
	req, ok := items["required"].([]any)
	if !ok || len(req) != 4 {
		t.Fatalf("required = %v, want four names", items["required"])
	}
	wantOrder := []any{"claim", "source_quote", "type", "strength"}
	if !reflect.DeepEqual(req, wantOrder) {
		t.Errorf("required order = %v, want %v", req, wantOrder)
	}
}

func TestSchemaEnumsMatchCanonicalValues(t *testing.T) {
	items := Schema()["properties"].(map[string]any)["claims"].(map[string]any)["items"].(map[string]any)
	props := items["properties"].(map[string]any)
	for field, want := range map[string][]string{
		"type":     ClaimTypes,
		"strength": ClaimStrengths,
	} {
		p := props[field].(map[string]any)
		if p["type"] != "string" {
			t.Errorf("%s.type = %v, want string", field, p["type"])
		}
		enum, ok := p["enum"].([]any)
		if !ok || len(enum) != len(want) {
			t.Fatalf("%s.enum = %v, want %d values", field, p["enum"], len(want))
		}
		for i, v := range want {
			if enum[i] != v {
				t.Errorf("%s.enum[%d] = %v, want %q", field, i, enum[i], v)
			}
		}
	}
}

// Each call must return an independent map so a caller marshalling or editing
// one cannot corrupt another's.
func TestSchemaReturnsAFreshMap(t *testing.T) {
	a := Schema()
	a["type"] = "mutated"
	if Schema()["type"] != "object" {
		t.Error("Schema() shares state between calls")
	}
}

func TestClaimEnumsAreNonEmptyAndDistinct(t *testing.T) {
	for name, vals := range map[string][]string{"ClaimTypes": ClaimTypes, "ClaimStrengths": ClaimStrengths} {
		seen := map[string]bool{}
		for _, v := range vals {
			if v == "" {
				t.Errorf("%s contains an empty value", name)
			}
			if seen[v] {
				t.Errorf("%s contains a duplicate: %q", name, v)
			}
			seen[v] = true
		}
	}
}

// Claude's --json-schema refuses a schema that names the 2020-12 dialect
// ("no schema with key or ref"), so Schema() must not declare one.
func TestSchemaDeclaresNoDialect(t *testing.T) {
	if _, ok := Schema()["$schema"]; ok {
		t.Error("Schema() declares $schema, which Claude's validator rejects")
	}
}
