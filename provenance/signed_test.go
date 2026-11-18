// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package provenance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// boundManifest is a manifest with all three bound assertions, and the
// assertions a credential over it would carry.
func boundManifest(t *testing.T) ([]byte, map[string]json.RawMessage) {
	t.Helper()
	body := "# T\n\nA sentence.\n"
	mb, _ := json.Marshal(NewManifest(ManifestInput{Title: "x", Body: body, Engine: "claude"}))
	mb, _, err := Attest(mb, body, "Ada", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Assertions []struct {
			Label string          `json:"label"`
			Data  json.RawMessage `json:"data"`
		} `json:"assertions"`
	}
	if err := json.Unmarshal(mb, &m); err != nil {
		t.Fatal(err)
	}
	signed := map[string]json.RawMessage{}
	for _, a := range m.Assertions {
		signed[a.Label] = a.Data
	}
	return mb, signed
}

func TestUnbound(t *testing.T) {
	manifest, signed := boundManifest(t)
	for _, label := range boundLabels {
		if _, ok := signed[label]; !ok {
			t.Fatalf("fixture is wrong: no %s assertion", label)
		}
	}
	if why := Unbound(manifest, signed); why != "" {
		t.Fatalf("a credential over this exact manifest must vouch for it: %s", why)
	}

	// c2patool hands the data back with its keys reordered and respaced.
	var g map[string]any
	_ = json.Unmarshal(signed[GroundingLabel], &g)
	reordered, _ := json.MarshalIndent(g, "", "    ")
	withReordered := copyAssertions(signed)
	withReordered[GroundingLabel] = reordered
	if why := Unbound(manifest, withReordered); why != "" {
		t.Errorf("key order must not matter: %s", why)
	}

	cases := map[string]struct {
		manifest []byte
		signed   map[string]json.RawMessage
		want     string
	}{
		"unreadable manifest": {[]byte("not json"), signed, "cannot be read"},
		"nothing signed":      {manifest, nil, "was not signed"},
		"grounding differs": {manifest, func() map[string]json.RawMessage {
			c := copyAssertions(signed)
			c[GroundingLabel] = json.RawMessage(strings.Replace(string(c[GroundingLabel]), `"claude"`, `"forged"`, 1))
			return c
		}(), GroundingLabel + " assertion differs"},
		"review not signed": {manifest, func() map[string]json.RawMessage {
			c := copyAssertions(signed)
			delete(c, ReviewLabel)
			return c
		}(), ReviewLabel + " assertion was not signed"},
		"signed review removed from the manifest": {
			[]byte(strings.Replace(string(manifest), `"`+ReviewLabel+`"`, `"x.removed"`, 1)),
			signed, ReviewLabel + " assertion is missing from the manifest"},
		"signed data unreadable": {manifest, func() map[string]json.RawMessage {
			c := copyAssertions(signed)
			c[AIDisclosureLabel] = json.RawMessage("{")
			return c
		}(), AIDisclosureLabel + " assertion differs"},
	}
	for name, c := range cases {
		if got := Unbound(c.manifest, c.signed); !strings.Contains(got, c.want) {
			t.Errorf("%s: Unbound = %q, want it to contain %q", name, got, c.want)
		}
	}
}

func copyAssertions(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
