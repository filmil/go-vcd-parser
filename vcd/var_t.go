package vcd

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/v2/lexer"
)

type IdT struct {
	Name    string  `parser:"@Ident" json:",omitempty"`
	Indices []*IdxT `parser:"@@*" json:",omitempty"`
}

type IdxT struct {
	Index    *int `parser:"(\"[\" @Int \"]\"" json:",omitempty"`
	MsbIndex *int `parser:"| \"[\" @Int" `
	LsbIndex *int `parser:" \":\" @Int \"]\") " json:",omitempty"`
}

type VarT struct {
	Pos        lexer.Position `json:"-"`
	tokenCount int
	varTokens  []string // Accumulated tokens that refer to the signal variable. Can be many.
	p          *participle.Parser[IdT]

	Kw      bool   `json:",omitempty"`
	VarType string `json:",omitempty"`
	Size    int    `json:",omitempty"`
	Code    string `json:",omitempty"`
	Id      IdT    `json:",omitempty"`
	KwEnd   bool   `json:",omitempty"`
}

// Capture implements custom capturing of tokens into VarT.
func (self *VarT) Capture(tokens []string) error {
	if self.p == nil {
		p, err := participle.Build[IdT](participle.UseLookahead(3))
		if err != nil {
			return fmt.Errorf("could not build mini parser: %w", err)

		}
		self.p = p
	}
	// Parsing 6 tokens total.
	//
	//	1    2     3 4 5   6
	//
	// `$var logic 1 ! clk[foo][bar] $end`
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		self.tokenCount++
		switch self.tokenCount {
		case 1: // First token to be read.
			if t != "$var" {
				return fmt.Errorf("expected keyword: `$var`, got: %v", t)
			}
		case 2: // Type
			self.VarType = t
			if self.GetVarKind() == VarKindUnknown {
				return fmt.Errorf("unknown var type: `%v`", t)
			}
		case 3:
			if _, err := fmt.Sscanf(t, "%d", &self.Size); err != nil {
				return fmt.Errorf("expected length, got: %w", err)
			}
		case 4:
			self.Code = t // This can be anything. Just consume it.
		case 5: // This is where it gets tricky.
			self.tokenCount-- // Make sure we come back to handle this again, until $end.
			if t == "$end" {  // Try to extract identifier now.
				idString := strings.Join(self.varTokens, "")
				id, err := self.p.ParseString("<idString>", idString)
				if err != nil {
					return fmt.Errorf("could not parse Id: `%v`: %w", idString, err)
				}
				self.Id = *id
				self.varTokens = nil
			} else {
				// While not accummulated yet, continue adding.
				self.varTokens = append(self.varTokens, t)
			}
		}
		return nil
	}
	return nil
}
