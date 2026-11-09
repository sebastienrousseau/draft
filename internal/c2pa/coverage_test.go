// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package c2pa

import (
	"context"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A real c2patool 0.27.22 report for a sidecar over a Markdown article: the
// data hash covers every byte, so the signature stands.
func TestRealReportOfAWholeArticleCredentialIsValid(t *testing.T) {
	rep, err := parseReport(fixture(t, "c2patool-0.27.22-valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.SignatureValid || rep.Uncovered != "" || rep.Trusted {
		t.Errorf("report = %+v", rep)
	}
}

func TestRealReportOfAnEditedArticleIsInvalid(t *testing.T) {
	rep, err := parseReport(fixture(t, "c2patool-0.27.22-edited.json"))
	if err != nil {
		t.Fatal(err)
	}
	if rep.SignatureValid {
		t.Errorf("an edited article verified: %+v", rep)
	}
}

// Buchanan's exclusion-range attack (October 2026): the data hash excludes
// the whole article, so it is a hash of nothing, and c2patool reports the
// credential Valid. draft must not.
func TestACredentialThatExcludesTheArticleIsInvalid(t *testing.T) {
	rep, err := parseReport(fixture(t, "exclusion-whole-article.json"))
	if err != nil {
		t.Fatal(err)
	}
	if rep.State != "Valid" {
		t.Fatalf("fixture should be one c2patool accepts, got state %q", rep.State)
	}
	if rep.SignatureValid || !strings.Contains(rep.Uncovered, "excludes 1 byte range") {
		t.Errorf("an exclusion over the article verified: %+v", rep)
	}
}

// Any exclusion, however small, leaves bytes the signature does not cover; a
// sidecar has nothing embedded in the article that needs excluding.
func TestEveryWayACredentialCanFailToCoverTheArticle(t *testing.T) {
	valid := `"validation_state":"Valid","validation_results":{"activeManifest":{"success":[{"code":"claimSignature.validated"},{"code":"assertion.dataHash.match"}]}}`
	for _, tc := range []struct {
		name, store, want string
	}{
		{"one byte excluded", `{"c2pa.hash.data":{"exclusions":[{"start":40,"length":1}]}}`, "excludes 1 byte range"},
		{"a second hash excluding", `{"c2pa.hash.data":{},"c2pa.hash.data__1":{"exclusions":[{"start":0,"length":9}]}}`, "excludes 1 byte range"},
		{"no data hash", `{"c2pa.actions.v2":{}}`, "no data hash"},
		{"another binding", `{"c2pa.hash.data":{},"c2pa.hash.boxes":{}}`, "c2pa.hash.boxes"},
		{"unreadable hash", `{"c2pa.hash.data":{"exclusions":"all"}}`, "cannot be read"},
		{"no active manifest", ``, "no data hash"},
	} {
		doc := `{` + valid + `}`
		if tc.store != "" {
			doc = `{"active_manifest":"m","manifests":{"m":{"assertion_store":` + tc.store + `}},` + valid + `}`
		}
		rep, err := parseReport(doc)
		if err != nil {
			t.Fatal(err)
		}
		if rep.SignatureValid || !strings.Contains(rep.Uncovered, tc.want) {
			t.Errorf("%s: %+v, want %q", tc.name, rep, tc.want)
		}
	}

	// c2patool must also say it hashed the article and it matched.
	unchecked := `{"active_manifest":"m","manifests":{"m":{"assertion_store":{"c2pa.hash.data":{}}}},"validation_state":"Valid","validation_results":{"activeManifest":{"success":[{"code":"claimSignature.validated"}]}}}`
	if rep, _ := parseReport(unchecked); rep.SignatureValid || !strings.Contains(rep.Uncovered, "did not confirm") {
		t.Errorf("unchecked data hash: %+v", rep)
	}
}

// draft's own signing goes through the same check, so it never hands back a
// credential that leaves part of the article unsigned, and says why.
func TestSignRefusesACredentialThatDoesNotCoverTheArticle(t *testing.T) {
	fakeTool(t)
	t.Setenv("FAKE_VERIFY_JSON", fixture(t, "exclusion-whole-article.json"))
	_, err := Sign(context.Background(), writeArticle(t), []byte(`{"assertions":[]}`), Signer{CertPath: "c.pem", KeyPath: "k.pem"})
	if err == nil || !strings.Contains(err.Error(), "excludes 1 byte range") {
		t.Errorf("Sign = %v", err)
	}
}

// Verify asks for the detailed report: the plain one omits the data hash,
// and every credential would then fail as having none.
func TestVerifyReadsTheDataHash(t *testing.T) {
	fakeTool(t)
	t.Setenv("FAKE_VERIFY_JSON", fixture(t, "exclusion-whole-article.json"))
	t.Setenv("FAKE_REQUIRE_DETAILED", "1")
	rep, err := Verify(context.Background(), writeArticle(t), []byte("cred"))
	if err != nil || rep.SignatureValid || rep.Uncovered == "" {
		t.Errorf("Verify = %+v, %v", rep, err)
	}
}
