// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package main

import (
	"go/scanner"
	"go/token"
)

// halsteadDifficulty returns (n1/2) * (N2/n2) for one function's source:
// n1 distinct operators, n2 distinct operands, N2 total operands. Operands
// are identifiers and literals; operators are every other token (keywords,
// punctuation), with a closing bracket counted as part of its opener and the
// newline semicolons the scanner inserts not counted at all.
func halsteadDifficulty(src []byte) float64 {
	operators, operands := map[string]bool{}, map[string]bool{}
	total := 0
	var s scanner.Scanner
	fset := token.NewFileSet()
	s.Init(fset.AddFile("", -1, len(src)), src, nil, 0)
	for {
		_, tok, lit := s.Scan()
		switch {
		case tok == token.EOF:
			if len(operands) == 0 {
				return 0
			}
			return float64(len(operators)) / 2 * float64(total) / float64(len(operands))
		case tok.IsLiteral():
			operands[lit] = true
			total++
		case tok == token.RPAREN || tok == token.RBRACK || tok == token.RBRACE:
		case tok == token.SEMICOLON && lit == "\n":
		default:
			operators[tok.String()] = true
		}
	}
}
