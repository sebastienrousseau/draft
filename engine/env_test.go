// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/sebastienrousseau/draft/config"
)

// envNames returns the variable names in env, so a test can ask what a
// provider was given without caring about the values.
func envNames(env []string) []string {
	names := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		names = append(names, name)
	}
	return names
}

// A provider signs in with its own credentials and nobody else's. Every other
// token in the user's shell is something an agent that ran a tool, or was
// talked into echoing its environment, could hand to whoever wrote the source.
func TestProviderGetsOnlyItsOwnCredentials(t *testing.T) {
	everyone := map[string]string{
		"ANTHROPIC_API_KEY":       "a",
		"CLAUDE_CODE_OAUTH_TOKEN": "a",
		"OPENAI_API_KEY":          "o",
		"CODEX_API_KEY":           "o",
		"GH_TOKEN":                "g",
		"GITHUB_TOKEN":            "g",
		"NPM_TOKEN":               "n",
		"STRIPE_SECRET_KEY":       "s",
		"SSH_AUTH_SOCK":           "/tmp/agent.sock",
		"VAULT_PASSWORD":          "v",
		"AWS_SECRET_ACCESS_KEY":   "aws",
		"HOME":                    "/home/u",
	}
	cases := []struct {
		provider   string
		keep, drop []string
	}{
		{"claude",
			[]string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "HOME"},
			[]string{"OPENAI_API_KEY", "CODEX_API_KEY", "GH_TOKEN", "GITHUB_TOKEN", "NPM_TOKEN", "STRIPE_SECRET_KEY", "SSH_AUTH_SOCK", "VAULT_PASSWORD"}},
		{"codex",
			[]string{"OPENAI_API_KEY", "CODEX_API_KEY", "HOME"},
			[]string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "GH_TOKEN", "GITHUB_TOKEN", "NPM_TOKEN", "STRIPE_SECRET_KEY", "SSH_AUTH_SOCK", "VAULT_PASSWORD", "AWS_SECRET_ACCESS_KEY"}},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			for k, v := range everyone {
				t.Setenv(k, v)
			}
			var captured *exec.Cmd
			orig := execCommand
			execCommand = captureExec(&captured)
			defer func() { execCommand = orig }()

			s, _ := NewSession(tc.provider, config.Config{})
			if _, err := s.Generate(context.Background(), Request{Prompt: "hi"}); err != nil {
				t.Fatal(err)
			}
			got := envNames(captured.Env)
			for _, name := range tc.keep {
				if !slices.Contains(got, name) {
					t.Errorf("%s lost %s, which it needs", tc.provider, name)
				}
			}
			for _, name := range tc.drop {
				if slices.Contains(got, name) {
					t.Errorf("%s was handed %s", tc.provider, name)
				}
			}
		})
	}
}

func TestSessionEnv(t *testing.T) {
	in := []string{
		"PATH=/bin", "HOME=/home/u", "DRAFTY=keep", "HTTPS_PROXY=http://proxy",
		"DRAFT_MODEL=x", "draft_engine=y",
		"NPM_TOKEN=1", "gh_token=2", "GH_HOST=ghe.example", "AWS_PROFILE=p",
		"OPENAI_API_KEY=3", "MY_SECRET=4", "DB_PASSWORD=5", "SSH_AUTH_SOCK=/s",
		"GOOGLE_APPLICATION_CREDENTIALS=/c.json", "AWS_SECRET_ACCESS_KEY=6",
		"ANTHROPIC_API_KEY=7", "COPILOT_PROVIDER_MAX_PROMPT_TOKENS=8",
		"=C:=C:\\",
	}
	got := sessionEnv(in, []string{"ANTHROPIC_*", "GH_TOKEN"})
	want := []string{
		"PATH=/bin", "HOME=/home/u", "DRAFTY=keep", "HTTPS_PROXY=http://proxy",
		"gh_token=2", "ANTHROPIC_API_KEY=7", "COPILOT_PROVIDER_MAX_PROMPT_TOKENS=8",
		"=C:=C:\\",
	}
	if !slices.Equal(got, want) {
		t.Errorf("sessionEnv() =\n%v\nwant\n%v", got, want)
	}
}

// A provider without Credentials cannot sign in from the environment at all,
// which looks like a broken login rather than a decision; make it one. Names
// are upper-cased before matching, so a lower-case pattern keeps nothing.
func TestEveryProviderNamesItsCredentials(t *testing.T) {
	for _, p := range defaultProviders() {
		if len(p.Credentials) == 0 {
			t.Errorf("%s names no credentials", p.Name)
		}
		for _, c := range p.Credentials {
			if c != strings.ToUpper(c) {
				t.Errorf("%s: credential pattern %q is not upper case", p.Name, c)
			}
		}
	}
}
