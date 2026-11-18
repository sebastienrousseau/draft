// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package tui

import "github.com/sebastienrousseau/draft/internal/runes"

// display makes text from outside draft (streamed tokens, log and warning
// lines, a failed job's error) safe to draw: a control sequence in it would
// act on the user's terminal under the view instead of being shown. The view
// adds its own styling afterwards, so only the payload is cleaned.
func display(s string) string { return runes.TerminalSafe(s) }
