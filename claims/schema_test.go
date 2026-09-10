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
	if got["type"] != "array" {
		t.Errorf("top-level type = %v, want array", got["type"])
	}
	items, ok := got["items"].(map[string]any)
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
	items := Schema()["items"].(map[string]any)
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
	if Schema()["type"] != "array" {
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
