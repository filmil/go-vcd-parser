package vcd

import (
	participle "github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// File represents a parsed Value Change Dump.
// The inline definition here, is based on the IEEE Std 1364-2001 Version C,
// page 331. Plus some extensions that don't seem described there, but
// happen in realistic VCD files.
type File struct {
	Pos lexer.Position

	DeclarationCommand []*DeclarationCommandT `parser:"@@*"`
	SimulationCommand  []*SimulationCommandT  `parser:"@@*"`
}

type DeclarationCommandT struct {
	Pos lexer.Position

	CommentText    string     `parser:"@KwComment @AnyNonspace* @KwEndSpecial"`
	Var            *VarT      `parser:"| @KwVar (@Ws? @AnyNonspace)* @Ws? @KwEndSpecial"`
	Date           string     `parser:"| @KwDate @AnyNonspace* @KwEndSpecial"`
	Version        string     `parser:"| @KwVersion @AnyNonspace* @KwEndSpecial"`
	Attrbegin      bool       `parser:"| @KwAttrbegin @AnyNonspace* @KwEndSpecial"`
	Attrend        bool       `parser:"| @KwAttrend @AnyNonspace* @KwEndSpecial"`
	EndDefinitions bool       `parser:"| @KwEnddefinitions (@KwEnd|@KwEndSpecial)"`
	Scope          ScopeT     `parser:"| @@"`
	Timescale      TimescaleT `parser:"| @@"`
	Upscope        bool       `parser:"| @KwUpscope @KwEnd"`
}

type VarTypeT struct {
	Pos lexer.Position

	Event     bool `parser:"\"event\""`
	Integer   bool `parser:"| \"integer\""`
	Parameter bool `parser:"| \"parameter\""`
	Real      bool `parser:"| \"real\""`
	Reg       bool `parser:"| \"reg\""`
	Supply0   bool `parser:"| \"supply0\""`
	Supply1   bool `parser:"| \"supply1\""`
	Time      bool `parser:"| \"time\""`
	Tri       bool `parser:"| \"tri\""`
	Triand    bool `parser:"| \"triand\""`
	Trior     bool `parser:"| \"trior\""`
	Trireg    bool `parser:"| \"trireg\""`
	Tri0      bool `parser:"| \"tri0\""`
	Tri1      bool `parser:"| \"tri1\""`
	Wand      bool `parser:"| \"wand\""`
	Wire      bool `parser:"| \"wire\""`
	Wor       bool `parser:"| \"wor\""`

	// Extensions?
	Logic  bool `parser:"| \"logic\""`
	String bool `parser:"| \"string\""`
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

	Kw     bool     `parser:"@KwTimescale"`
	Number int64    `parser:"@Int"`
	Unit   TimeUnit `parser:"@@"`
	Kw2    bool     `parser:"@KwEnd"`
}

// AsSeconds returns the number of seconds (possibly fractional, possibly very small)
func (self TimescaleT) AsSeconds() float64 {
	return float64(self.Number) * self.Unit.Multiplier()
}

// AsNanoseconds returns the number of nanoseconds.
func (self TimescaleT) AsNanoseconds() float64 {
	return self.AsSeconds() * 1e9
}

type TimeUnit struct {
	Pos lexer.Position

	Second      bool `parser:"\"s\""`
	MilliSecond bool `parser:"| \"ms\""`
	MicroSecond bool `parser:"| \"us\""`
	NanoSecond  bool `parser:"| \"ns\""`
	PicoSecond  bool `parser:"| \"ps\""`
	FemtoSecond bool `parser:"| \"fs\""`
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
	Pos lexer.Position

	Scope     bool       `parser:"@KwScope"`
	ScopeKind ScopeKindT `parser:"@@"`
	Id        string     `parser:"@Ident @KwEnd"`
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
	Pos lexer.Position

	Begin    bool `parser:"\"begin\""`
	Fork     bool `parser:"| \"fork\""`
	Function bool `parser:"| \"function\""`
	Module   bool `parser:"| \"module\""`
	Task     bool `parser:"| \"task\""`

	// Extensions?
	VHDLArchitecture bool `parser:"| \"vhdl_architecture\""`
	VHDLRecord       bool `parser:"| \"vhdl_record\""`
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

	Dumpall        DumpallT        `parser:"@@"`
	Dumpoff        DumpoffT        `parser:"| @@"`
	Dumpon         DumponT         `parser:"| @@"`
	Dumpvars       DumpvarsT       `parser:"| @@"`
	SimulationTime SimulationTimeT `parser:"| @@"`
	ValueChange    ValueChangeT    `parser:"| @@"`
	Attrbegin      bool            `parser:"| @KwAttrbegin @AnyNonspace* @KwEndSpecial"`
	Attrend        bool            `parser:"| @KwAttrend @AnyNonspace* @KwEndSpecial"`
}

type DumpallT struct {
	Pos lexer.Position

	Kw          bool            `parser:"@KwDumpall"`
	ValueChange []*ValueChangeT `parser:"@@*"`
	KwEnd       bool            `parser:"@KwEnd"`
}

type DumpoffT struct {
	Pos lexer.Position

	Kw          bool            `parser:"@KwDumpoff"`
	ValueChange []*ValueChangeT `parser:"@@*"`
	KwEnd       bool            `parser:"@KwEnd"`
}

type DumponT struct {
	Pos lexer.Position

	Kw          bool            `parser:"@KwDumpon"`
	ValueChange []*ValueChangeT `parser:"@@*"`
	KwEnd       bool            `parser:"@KwEnd"`
}

type DumpvarsT struct {
	Pos lexer.Position

	Kw          bool            `parser:"@KwDumpvars"`
	ValueChange []*ValueChangeT `parser:"@@*"`
	KwEnd       bool            `parser:"@KwEnd"`
}

type SimulationKeywordT struct {
	Pos    lexer.Position
	Tokens []lexer.Token

	DumpOff  bool `parser:"@KwDumpoff"`
	DumpOn   bool `parser:"| @KwDumpon"`
	DumpVars bool `parser:"| @KwDumpvars"`
}

type SimulationTimeT struct {
	Pos    lexer.Position
	Tokens []lexer.Token

	DecimalNumber string `parser:"@Timestamp"`
}

type ValueChangeT struct {
	Pos    lexer.Position
	Tokens []lexer.Token

	ScalarValueChange *ScalarValueChangeT `parser:"@@"`
	VectorValueChange *VectorValueChangeT `parser:"| @@"`
}

type ScalarValueChangeT struct {
	Pos    lexer.Position
	Tokens []lexer.Token

	Value  ValueT `parser:"@@"`
	IdCode string `parser:"@IdCode"`
	Garble string `parser:"| @IdCode"`
}

func (self ScalarValueChangeT) GetIdCode() string {
	if self.Garble != "" {
		return string(self.Garble[1:])
	}
	return self.IdCode
}

func (self ScalarValueChangeT) GetValue() string {
	if self.Garble != "" {
		return string(self.Garble[1])
	}
	return self.Value.Value
}

type ValueT struct {
	Pos    lexer.Position
	Tokens []lexer.Token

	Value string `parser:"@(\"0\" | \"1\" | \"x\" | \"X\"| \"z\" | \"Z\")"`
}

type VectorValueChangeT struct {
	Pos    lexer.Position
	Tokens []lexer.Token

	VectorValueChange1 *VectorValueChange1T `parser:"@@"`
	VectorValueChange3 *VectorValueChange3T `parser:"| @@"`
}

type VectorValueChange1T struct {
	Pos    lexer.Position
	Tokens []lexer.Token

	BinaryNumber string `parser:"@Binstring"`
	IdCode       string `parser:"@IdCode"`
}

type VectorValueChange3T struct {
	Pos    lexer.Position
	Tokens []lexer.Token

	RealNumber     string `parser:"@RealString"`
	IdentifierCode string `parser:"@IdCode"`
}

// MaxIterations is the max number of items to be captured by the parser.
// While the default is 1 million, VCD files can be GIGANTIC, so we just
// let it rip.
const MaxIterations = int(int64(^uint64(0) >> 1))

func commonNewParser[T any](l *lexer.StatefulDefinition) *participle.Parser[T] {
	participle.MaxIterations = MaxIterations
	return participle.MustBuild[T](
		participle.Lexer(l),
		// For " variable[foo], variable[foo:bar]"
		participle.UseLookahead(5),
	)
}

func NewParser[T any]() *participle.Parser[T] {
	return commonNewParser[T](NewLexer())
}

func NewIdParser[T any]() *participle.Parser[T] {
	return commonNewParser[T](NewIdLexer())
}
