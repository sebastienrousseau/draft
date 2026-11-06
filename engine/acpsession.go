// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package engine

import (
	"strings"
	"sync"
)

// acpSession collects one session's streamed text. Chunks are appended by the
// reader goroutine as they arrive, so the collected text is complete by the
// time the prompt's response is delivered: the agent sends its updates before
// it answers the request.
type acpSession struct {
	mu       sync.Mutex
	buf      strings.Builder
	onChunk  func(string)
	ranTools bool
}

func (s *acpSession) add(chunk string) {
	s.mu.Lock()
	s.buf.WriteString(chunk)
	s.mu.Unlock()
	if s.onChunk != nil && chunk != "" {
		s.onChunk(chunk)
	}
}

// toolUsed records that the agent reported a tool call, and reports whether
// it is the first, so the turn is cancelled once.
func (s *acpSession) toolUsed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.ranTools
	s.ranTools = true
	return first
}

func (s *acpSession) usedTools() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ranTools
}

func (s *acpSession) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
