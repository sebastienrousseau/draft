// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"
)

// completionFlags is the flag list offered by every shell's completion,
// derived from flagHelp so the two genuinely cannot drift. The old hand-kept
// copy carried the same promise in a comment and had fallen four flags behind.
var completionFlags = flagNames(flagHelp)

// flagNames strips the value placeholder from each help entry ("--out <dir>"
// becomes "--out") and drops the combined "-h, --help" form, which no shell
// wants offered as one token.
func flagNames(help [][2]string) []string {
	out := make([]string, 0, len(help))
	for _, f := range help {
		for _, part := range strings.Split(f[0], ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), " ")
			if strings.HasPrefix(name, "--") {
				out = append(out, name)
			}
		}
	}
	return out
}

// completionEngines are the values --engine accepts beyond provider names.
var completionEngines = []string{"auto", "ollama"}

// writeCompletion prints a completion script for shell to w. It returns false
// for an unknown shell.
func writeCompletion(w io.Writer, shell string, providers []string) bool {
	flags := strings.Join(completionFlags, " ")
	engines := strings.Join(append(append([]string{}, completionEngines...), providers...), " ")

	switch shell {
	case "bash":
		fmt.Fprintf(w, `# draft bash completion — add to ~/.bashrc:
#   source <(draft --completion bash)
_draft_complete() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  case "$prev" in
    --engine) COMPREPLY=( $(compgen -W %[2]q -- "$cur") ); return ;;
    --review|--frontmatter|--combine) COMPREPLY=( $(compgen -f -- "$cur") ); return ;;
    --completion) COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") ); return ;;
  esac
  if [[ "$cur" == -* ]]; then
    COMPREPLY=( $(compgen -W %[1]q -- "$cur") )
  else
    COMPREPLY=( $(compgen -f -- "$cur") )
  fi
}
complete -F _draft_complete draft
`, flags, engines)

	case "zsh":
		fmt.Fprintf(w, `#compdef draft
# draft zsh completion — add to ~/.zshrc:
#   source <(draft --completion zsh)
_draft() {
  local -a flags engines
  flags=(%[1]s)
  engines=(%[2]s)
  case "${words[CURRENT-1]}" in
    --engine) compadd -- $engines; return ;;
    --review|--frontmatter|--combine) _files; return ;;
    --completion) compadd -- bash zsh fish; return ;;
  esac
  if [[ "${words[CURRENT]}" == -* ]]; then
    compadd -- $flags
  else
    _files
  fi
}
compdef _draft draft
`, flags, engines)

	case "fish":
		fmt.Fprintln(w, "# draft fish completion — save to ~/.config/fish/completions/draft.fish:")
		fmt.Fprintln(w, "#   draft --completion fish > ~/.config/fish/completions/draft.fish")
		for _, f := range completionFlags {
			fmt.Fprintf(w, "complete -c draft -l %s\n", strings.TrimPrefix(f, "--"))
		}
		fmt.Fprintf(w, "complete -c draft -l engine -x -a %q\n", engines)
		fmt.Fprintln(w, `complete -c draft -l completion -x -a "bash zsh fish"`)
		for _, f := range []string{"review", "frontmatter", "combine"} {
			fmt.Fprintf(w, "complete -c draft -l %s -r -F\n", f)
		}

	default:
		return false
	}
	return true
}
