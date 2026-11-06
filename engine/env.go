// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import "strings"

// credentialPatterns name the variables a provider is not given unless they
// are its own credentials. "*_X" matches a suffix, "X_*" a prefix, anything
// else the whole name.
//
// This is a denylist, and deliberately not an allowlist: a provider reads
// proxies, locales, certificates and its own settings from the environment,
// and a list of what it may see breaks one of them and surfaces as a
// confusing provider error. What it must not see is narrower and nameable: the
// tokens for every other service in the user's shell, which a provider that
// ran a tool, or was talked into echoing its environment, could hand to
// whoever wrote the source. It is a second line, not the boundary: a secret
// under a name nothing here matches still gets through.
var credentialPatterns = []string{
	"*_TOKEN", "*_API_KEY", "*_SECRET", "*_SECRET_KEY", "*_ACCESS_KEY",
	"*_PASSWORD", "*_CREDENTIALS",
	"AWS_*", "GH_*",
	// The agent socket signs as the user without revealing a key, so it is
	// worth more than most keys.
	"SSH_AUTH_SOCK",
}

// sessionEnv is the environment handed to a provider subprocess: environ
// without draft's own configuration and without any credential except those
// matching keep, the provider's own.
//
// DRAFT_* names steer draft, and letting them reach an agent that reads its
// own environment lets one tool's settings silently redirect another's.
// Names are compared in upper case because Windows treats them that way.
func sessionEnv(environ, keep []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		name = strings.ToUpper(name)
		if strings.HasPrefix(name, "DRAFT_") {
			continue
		}
		if matchesAny(name, credentialPatterns) && !matchesAny(name, keep) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if matchesEnv(name, p) {
			return true
		}
	}
	return false
}

func matchesEnv(name, pattern string) bool {
	if suffix, ok := strings.CutPrefix(pattern, "*"); ok {
		return strings.HasSuffix(name, suffix)
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(name, prefix)
	}
	return name == pattern
}
