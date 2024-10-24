package vcd

import (
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

	CommentText    string `@KwComment @AnyNonspace* @KwEndSpecial`
    Var VarT `| @@`
	Date           string `| @KwDate @AnyNonspace* @KwEndSpecial`
	Version        string `| @KwVersion @AnyNonspace* @KwEndSpecial`
	EndDefinitions bool   `| @KwEnddefinitions @KwEnd`
	Scope          ScopeT `| @@`
    Timescale TimescaleT `| @@`
    Upscope bool `| @KwUpscope @KwEnd`
	//DeclarationKeyword DeclarationKeywordT `@@`
	//CommandText        *string             `(@String) @KwEnd?`
}

type VarT struct {
    Pos lexer.Position

    Kw bool `@KwVar`
    VarType VarTypeT `@@`
    Size int `@Int`
    Code string `@IdCode`
    Id IdT `@@`
    KwEnd bool `@KwEnd`
}

type IdT struct {
    Name string `@Identifier`
    Index *int `( "[" @Int "]"`
    MsbIndex *int `| "[" @Int `
    LsbIndex *int `  ":" @Int "]" )?`
}

type VarTypeT struct {
    Pos lexer.Position

    Event bool `"event"`
    Integer bool `| "integer"`
    Parameter bool `| "parameter"`
    Real bool `| "real"`
    Reg bool `| "reg"`
    Supply0 bool `| "supply0"`
    Supply1 bool `| "supply1"`
    Time bool `| "time"`
    Tri bool `| "tri"`
    Triand bool `| "triand"`
    Trior bool `| "trior"`
    Trireg bool `| "trireg"`
    Tri0 bool `| "tri0"`
    Tri1 bool `| "tri1"`
    Wand bool `| "wand"`
    Wire bool `| "wire"`
    Wor bool `| "wor"`
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
    VarKindUnknown
)

func (self VarTypeT) GetVarKind() VarKindCode {
    switch {
        case self.Event: return VarKindEvent
    case self.Integer: return VarKindInteger
    case self.Parameter: return  VarKindParameter
    case self.Real: return VarKindReal
    case self.Reg: return VarKindReg
    case self.Supply0: return VarKindSupply0
    case self.Supply1: return VarKindSupply1
    case self.Time: return VarKindTime
    case self.Tri: return VarKindTri
    case self.Triand: return VarKindTriand
    case self.Trior: return VarKindTrior
    case self.Trireg: return VarKindTrireg
    case self.Tri0: return VarKindTri0
    case self.Tri1: return VarKindTri1
    case self.Wand: return VarKindWand
    case self.Wire: return VarKindWire
    case self.Wor: return VarKindWor
    }
    return VarKindUnknown
}

type TimescaleT struct {
    Pos lexer.Position

    Kw bool `@KwTimescale`
    Number int64 `@Int`
    Unit TimeUnit `@@`
    Kw2 bool `@KwEnd`
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
    Second bool `"s"`
    MilliSecond bool `| "ms"`
    MicroSecond bool `| "us"`
    NanoSecond bool `| "ns"`
    PicoSecond bool `| "ps"`
    FemtoSecond bool `| "fs"`
}

func (self TimeUnit) Multiplier() float64 {
    switch {
    case self.Second:  return 1.0
    case self.MilliSecond: return 1e-3
    case self.MicroSecond:return  1e-6
    case self.NanoSecond: return 1e-9
    case self.PicoSecond: return 1e-12
    case self.FemtoSecond: return 1e-15
    }
    return 0.0
}

type ScopeT struct {
    Scope bool `@KwScope`
	ScopeKind ScopeKindT `@@`
    Id string `@Identifier @KwEnd`
}

type ScopeKindCode int

const (
	ScopeKindBegin ScopeKindCode = iota
	ScopeKindFork
	ScopeKindModule
	ScopeKindFunction
	ScopeKindTask
	ScopeKindUnknown
)

type ScopeKindT struct {
	Begin    bool `"begin"`
	Fork     bool `| "fork"`
	Function bool `| "function"`
	Module   bool `| "module"`
	Task     bool `| "task"`
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
	}
	return ScopeKindUnknown
}

type SimulationCommandT struct {
	Pos lexer.Position

	SimulationCommand1 SimulationCommand1T `@@`
	SimulationTime     SimulationTimeT     `| @@`
	ValueChange        ValueChangeT        `| @@`
}

type SimulationCommand1T struct {
	Pos lexer.Position

	SimulationKeyword SimulationKeywordT `@@`
	ValueChange       []*ValueChangeT    `@@*`
	End               bool               `@KwEnd`
}

type SimulationKeywordT struct {
	Pos lexer.Position

	DumpAll  bool `@KwDumpall`
	DumpOff  bool `| @KwDumpoff`
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
	IdentifierCode string `| @String`
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
	IdentifierCode string `@String`
}

type VectorValueChange3T struct {
	Pos lexer.Position

	RealNumber     string `@RealString`
	IdentifierCode string `@String`
}

func NewParser() *participle.Parser[VCDFile] {
	// Needs a lexer definition.
	return participle.MustBuild[VCDFile](
        participle.Lexer(NewLexer()),
        // For " variable[foo], variable[foo:bar]"
        participle.UseLookahead(2),
    )
}
