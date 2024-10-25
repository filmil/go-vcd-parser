package vcd

import (
	"fmt"
	"strings"
	"time"

	participle "github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// VCDFile represents a parsed Value Change Dump.
// The inline definition here, is based on the IEEE Std 1364-2001 Version C,
// page 331.
type VCDFile struct {
	Pos lexer.Position

	DeclarationCommand []*DeclarationCommandT `@@*`
	SimulationCommand  []*SimulationCommandT  `@@*`
}

type DeclarationCommandT struct {
	Pos lexer.Position

	CommentText    string     `@KwComment @AnyNonspace* @KwEndSpecial`
	Var            *VarT      `| @KwVar (@Ws? @AnyNonspace)* @Ws? @KwEndSpecial`
	Date           string     `| @KwDate @AnyNonspace* @KwEndSpecial`
	Version        string     `| @KwVersion @AnyNonspace* @KwEndSpecial`
	Attrbegin      bool       `| @KwAttrbegin @AnyNonspace* @KwEndSpecial`
	Attrend        bool       `| @KwAttrend @AnyNonspace* @KwEndSpecial`
	EndDefinitions bool       `| @KwEnddefinitions @KwEnd`
	Scope          ScopeT     `| @@`
	Timescale      TimescaleT `| @@`
	Upscope        bool       `parser:"| @KwUpscope @KwEnd"`
	//DeclarationKeyword DeclarationKeywordT `@@`
	//CommandText        *string             `(@String) @KwEnd?`
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

type VarT struct {
	Pos        lexer.Position
	tokenCount int
	varTokens  []string // Accumulated tokens that refer to the signal variable. Can be many.
	p          *participle.Parser[IdT]

	Kw      bool
	VarType string
	Size    int
	Code    string
	Id      IdT
	KwEnd   bool
}

type IdT struct {
	Name    string  `parser:"@Ident"`
	Indices []*IdxT `parser:"@@*"`
}

type IdxT struct {
	Index    *int `parser:"(\"[\" @Int \"]\""`
	MsbIndex *int `| "[" @Int `
	LsbIndex *int `":" @Int "]")`
}

type VarTypeT struct {
	Pos lexer.Position

	Event     bool `"event"`
	Integer   bool `| "integer"`
	Parameter bool `| "parameter"`
	Real      bool `| "real"`
	Reg       bool `| "reg"`
	Supply0   bool `| "supply0"`
	Supply1   bool `| "supply1"`
	Time      bool `| "time"`
	Tri       bool `| "tri"`
	Triand    bool `| "triand"`
	Trior     bool `| "trior"`
	Trireg    bool `| "trireg"`
	Tri0      bool `| "tri0"`
	Tri1      bool `| "tri1"`
	Wand      bool `| "wand"`
	Wire      bool `| "wire"`
	Wor       bool `| "wor"`

	// Extensions?
	Logic  bool `| "logic"`
	String bool `| "string"`
}

// VarKindCode is the type code for a variable.
type VarKindCode int

const (
	VarKindEvent VarKindCode = iota
	VarKindInteger
	VarKindParameter
	VarKindReal
	VarKindReg
	VarKindSupply0
	VarKindSupply1
	VarKindTime
	VarKindTri
	VarKindTriand
	VarKindTrior
	VarKindTrireg
	VarKindTri0
	VarKindTri1
	VarKindWand
	VarKindWire
	VarKindWor

	// Extensions?
	VarKindLogic
	VarKindString
	VarKindUnknown
)

var stringToVarKind = map[string]VarKindCode{
	"event":     VarKindEvent,
	"integer":   VarKindInteger,
	"parameter": VarKindParameter,
	"real":      VarKindReal,
	"reg":       VarKindReg,
	"supply0":   VarKindSupply0,
	"supply1":   VarKindSupply1,
	"time":      VarKindTime,
	"tri":       VarKindTri,
	"triand":    VarKindTriand,
	"trior":     VarKindTrior,
	"trireg":    VarKindTrireg,
	"tri0":      VarKindTri0,
	"tri1":      VarKindTri1,
	"wand":      VarKindWand,
	"wire":      VarKindWire,
	"wor":       VarKindWor,
	// Extensions?
	"logic":  VarKindLogic,
	"string": VarKindString,
}

func (self VarT) GetVarKind() VarKindCode {
	v, ok := stringToVarKind[self.VarType]
	if !ok {
		return VarKindUnknown
	}
	return v
}

type TimescaleT struct {
	Pos lexer.Position

	Kw     bool     `@KwTimescale`
	Number int64    `@Int`
	Unit   TimeUnit `@@`
	Kw2    bool     `@KwEnd`
}

// AsSeconds returns the number of seconds (possibly fractional, possibly very small)
func (self TimescaleT) AsSeconds() float64 {
	return float64(self.Number) * self.Unit.Multiplier()
}

// AsNanoseconds returns the number of nanoseconds.
func (self TimescaleT) AsNanoseconds() float64 {
	return self.AsSeconds() * 1e9
}

type SimDuration struct {
	// durFemto is a Duration, but time.Nanosecond is used as time.FemtoSecond.
	durFemto time.Duration
}

type TimeUnit struct {
	Second      bool `"s"`
	MilliSecond bool `| "ms"`
	MicroSecond bool `| "us"`
	NanoSecond  bool `| "ns"`
	PicoSecond  bool `| "ps"`
	FemtoSecond bool `| "fs"`
}

func (self TimeUnit) Multiplier() float64 {
	switch {
	case self.Second:
		return 1.0
	case self.MilliSecond:
		return 1e-3
	case self.MicroSecond:
		return 1e-6
	case self.NanoSecond:
		return 1e-9
	case self.PicoSecond:
		return 1e-12
	case self.FemtoSecond:
		return 1e-15
	}
	return 0.0
}

type ScopeT struct {
	Scope     bool       `@KwScope`
	ScopeKind ScopeKindT `@@`
	Id        string     `@Identifier @KwEnd`
}

type ScopeKindCode int

const (
	ScopeKindBegin ScopeKindCode = iota
	ScopeKindFork
	ScopeKindModule
	ScopeKindFunction
	ScopeKindTask
	ScopeKindVHDLArchitecture
	ScopeKindVHDLRecord
	ScopeKindUnknown
)

type ScopeKindT struct {
	Begin    bool `"begin"`
	Fork     bool `| "fork"`
	Function bool `| "function"`
	Module   bool `| "module"`
	Task     bool `| "task"`

	// Extensions?
	VHDLArchitecture bool `| "vhdl_architecture"`
	VHDLRecord       bool `| "vhdl_record"`
}

func (self ScopeKindT) Kind() ScopeKindCode {
	switch {
	case self.Begin:
		return ScopeKindBegin
	case self.Fork:
		return ScopeKindFork
	case self.Function:
		return ScopeKindFunction
	case self.Module:
		return ScopeKindModule
	case self.Task:
		return ScopeKindTask
	case self.VHDLArchitecture:
		return ScopeKindVHDLArchitecture
	case self.VHDLRecord:
		return ScopeKindVHDLRecord
	}
	return ScopeKindUnknown
}

type SimulationCommandT struct {
	Pos lexer.Position

	Dumpall        DumpallT        `@@`
	Dumpoff        DumpoffT        `| @@`
	Dumpon         DumponT         `| @@`
	Dumpvars       DumpvarsT       `| @@`
	SimulationTime SimulationTimeT `| @@`
	ValueChange    ValueChangeT    `| @@`
	Attrbegin      bool            `| @KwAttrbegin @AnyNonspace* @KwEndSpecial`
	Attrend        bool            `| @KwAttrend @AnyNonspace* @KwEndSpecial`
}

type DumpallT struct {
	Kw          bool            `@KwDumpall`
	ValueChange []*ValueChangeT `@@*`
	KwEnd       bool            `@KwEnd`
}

type DumpoffT struct {
	Kw          bool            `@KwDumpoff`
	ValueChange []*ValueChangeT `@@*`
	KwEnd       bool            `@KwEnd`
}

type DumponT struct {
	Kw          bool            `@KwDumpon`
	ValueChange []*ValueChangeT `@@*`
	KwEnd       bool            `@KwEnd`
}

type DumpvarsT struct {
	Kw          bool            `parser:"@KwDumpvars"`
	ValueChange []*ValueChangeT `parser:"@@*"`
	KwEnd       bool            `parser:"@KwEnd"`
}

type SimulationKeywordT struct {
	Pos lexer.Position

	DumpOff  bool `@KwDumpoff`
	DumpOn   bool `| @KwDumpon`
	DumpVars bool `| @KwDumpvars`
}

type SimulationTimeT struct {
	Pos lexer.Position

	DecimalNumber string `@Timestamp`
}

type ValueChangeT struct {
	Pos lexer.Position

	ScalarValueChange *ScalarValueChangeT `@@`
	VectorValueChange *VectorValueChangeT `| @@`
}

type ScalarValueChangeT struct {
	Pos lexer.Position

	Value          ValueT `@@`
	IdentifierCode string `| @IdCode`
}

type ValueT struct {
	Pos lexer.Position

	Value string `@("0" | "1" | "x" | "X"| "z" | "Z")`
}

type VectorValueChangeT struct {
	Pos lexer.Position

	VectorValueChange1 *VectorValueChange1T `@@`
	VectorValueChange3 *VectorValueChange3T `| @@`
}

type VectorValueChange1T struct {
	Pos lexer.Position

	BinaryNumber   string `@Binstring`
	IdentifierCode string `@IdCode`
}

type VectorValueChange3T struct {
	Pos lexer.Position

	RealNumber     string `@RealString`
	IdentifierCode string `@IdCode`
}

func NewParser() *participle.Parser[VCDFile] {
	// Needs a lexer definition.
	return participle.MustBuild[VCDFile](
		participle.Lexer(NewLexer()),
		// For " variable[foo], variable[foo:bar]"
		participle.UseLookahead(5),
	)
}
