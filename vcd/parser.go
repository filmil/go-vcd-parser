package vcd

import (
	"fmt"

	participle "github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// File represents a parsed Value Change Dump.
// The inline definition here, is based on the IEEE Std 1364-2001 Version C,
// page 331. Plus some extensions that don't seem described there, but
// happen in realistic VCD files.
type File struct {
	DeclarationCommand []*DeclarationCommandT `parser:"@@*" json:",omitempty"`
	SimulationCommand  []*SimulationCommandT  `parser:"@@*" json:",omitempty"`
}

type DeclarationCommandT struct {
	CommentText *string `parser:"@KwComment @AnyNonspace* @KwEndSpecial" json:",omitempty"`
	Var         *VarT   `parser:"| @KwVar (@Ws? @AnyNonspace)* @Ws? @KwEndSpecial" json:",omitempty"`
	Date        *string `parser:"| @KwDate @AnyNonspace* @KwEndSpecial" json:",omitempty"`
	Version     *string `parser:"| @KwVersion @AnyNonspace* @KwEndSpecial" json:",omitempty"`
	Attrbegin   *bool   `parser:"| @KwAttrbegin @AnyNonspace* @KwEndSpecial" json:",omitempty"`
	Attrend     *bool   `parser:"| @KwAttrend @AnyNonspace* @KwEndSpecial" json:",omitempty"`

	EndDefinitions *bool `parser:"| @KwEnddefinitions (@KwEnd|@KwEndSpecial)" json:",omitempty"`

	Scope     *ScopeT     `parser:"| @@" json:",omitempty"`
	Timescale *TimescaleT `parser:"| @@" json:",omitempty"`
	Upscope   *bool       `parser:"| @KwUpscope @KwEnd" json:",omitempty"`
}

type VarTypeT struct {
	Event     bool `parser:" @\"event\"" json:",omitempty"`
	Integer   bool `parser:"|  @\"integer\"" json:",omitempty"`
	Parameter bool `parser:"|  @\"parameter\"" json:",omitempty"`
	Real      bool `parser:"| @\"real\"" json:",omitempty"`
	Reg       bool `parser:"| @\"reg\"" json:",omitempty"`
	Supply0   bool `parser:"| @\"supply0\"" json:",omitempty"`
	Supply1   bool `parser:"| @\"supply1\"" json:",omitempty"`
	Time      bool `parser:"| @\"time\"" json:",omitempty"`
	Tri       bool `parser:"| @\"tri\"" json:",omitempty"`
	Triand    bool `parser:"| @\"triand\"" json:",omitempty"`
	Trior     bool `parser:"| @\"trior\"" json:",omitempty"`
	Trireg    bool `parser:"| @\"trireg\"" json:",omitempty"`
	Tri0      bool `parser:"| @\"tri0\"" json:",omitempty"`
	Tri1      bool `parser:"| @\"tri1\"" json:",omitempty"`
	Wand      bool `parser:"| @\"wand\"" json:",omitempty"`
	Wire      bool `parser:"| @\"wire\"" json:",omitempty"`
	Wor       bool `parser:"| @\"wor\"" json:",omitempty"`

	// Extensions?
	Logic  bool `parser:"| @\"logic\"" json:",omitempty"`
	String bool `parser:"| @\"string\"" json:",omitempty"`
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

func (self VarKindCode) Int() int {
	return int(self)
}

func (self VarT) GetVarKind() VarKindCode {
	v, ok := stringToVarKind[self.VarType]
	if !ok {
		return VarKindUnknown
	}
	return v
}

type TimescaleT struct {
	Kw     bool      `parser:"@KwTimescale" json:"-"`
	Number int64     `parser:"@Int" json:",omitempty"`
	Unit   *TimeUnit `parser:"@@" json:",omitempty"`
	Kw2    bool      `parser:"@KwEnd" json:"-"`
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
	Second      bool `parser:" @\"s\"" json:",omitempty"`
	MilliSecond bool `parser:"|  @\"ms\"" json:",omitempty"`
	MicroSecond bool `parser:"|  @\"us\"" json:",omitempty"`
	NanoSecond  bool `parser:"|  @\"ns\"" json:",omitempty"`
	PicoSecond  bool `parser:"|  @\"ps\"" json:",omitempty"`
	FemtoSecond bool `parser:"|  @\"fs\"" json:",omitempty"`
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
	Scope     bool       `parser:"@KwScope" json:",omitempty"`
	ScopeKind ScopeKindT `parser:"@@" json:",omitempty"`
	Id        string     `parser:"@Ident" json:",omitempty"`
	KwEnd     bool       `parser:"@KwEnd" json:"-"`
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
	Begin    bool `parser:"@\"begin\"" json:",omitempty"`
	Fork     bool `parser:"| \"fork\"" json:",omitempty"`
	Function bool `parser:"| \"function\"" json:",omitempty"`
	Module   bool `parser:"| @\"module\"" json:",omitempty"`
	Task     bool `parser:"| \"task\"" json:",omitempty"`

	// Extensions?
	VHDLArchitecture bool `parser:"| \"vhdl_architecture\"" json:",omitempty"`
	VHDLRecord       bool `parser:"| \"vhdl_record\"" json:",omitempty"`
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
	Dumpall        *DumpallT        `parser:"@@" json:",omitempty"`
	Dumpoff        *DumpoffT        `parser:"| @@" json:",omitempty"`
	Dumpon         *DumponT         `parser:"| @@" json:",omitempty"`
	Dumpvars       *DumpvarsT       `parser:"| @@" json:",omitempty"`
	SimulationTime *SimulationTimeT `parser:"| @@" json:",omitempty"`
	ValueChange    *ValueChangeT    `parser:"| @@" json:",omitempty"`
	Attrbegin      *bool            `parser:"| @KwAttrbegin @AnyNonspace* @KwEndSpecial" json:",omitempty"`
	Attrend        *bool            `parser:"| @KwAttrend @AnyNonspace* @KwEndSpecial" json:",omitempty"`
}

type DumpallT struct {
	Kw          bool            `parser:"@KwDumpall" json:",omitempty"`
	ValueChange []*ValueChangeT `parser:"@@*" json:",omitempty"`
	KwEnd       bool            `parser:"@KwEnd" json:",omitempty"`
}

type DumpoffT struct {
	Kw          bool            `parser:"@KwDumpoff" json:",omitempty"`
	ValueChange []*ValueChangeT `parser:"@@*" json:",omitempty"`
	KwEnd       bool            `parser:"@KwEnd" json:",omitempty"`
}

type DumponT struct {
	Kw          bool            `parser:"@KwDumpon" json:",omitempty"`
	ValueChange []*ValueChangeT `parser:"@@*" json:",omitempty"`
	KwEnd       bool            `parser:"@KwEnd" json:",omitempty"`
}

type DumpvarsT struct {
	Kw          bool            `parser:"@KwDumpvars" json:",omitempty"`
	ValueChange []*ValueChangeT `parser:"@@*" json:",omitempty"`
	KwEnd       bool            `parser:"@KwEnd" json:",omitempty"`
}

type SimulationKeywordT struct {
	DumpOff  bool `parser:"@KwDumpoff" json:",omitempty"`
	DumpOn   bool `parser:"| @KwDumpon" json:",omitempty"`
	DumpVars bool `parser:"| @KwDumpvars" json:",omitempty"`
}

type SimulationTimeT struct {
	DecimalNumber string `parser:"@Timestamp" json:",omitempty"`
}

func (self SimulationTimeT) Value() uint64 {
	var ret uint64
	s := self.DecimalNumber
	l := len(s)
	s = s[1:l]

	_, err := fmt.Sscanf(s, "%d", &ret)
	if err != nil {
		panic(fmt.Sprintf("Value is not parseable as uint64: %q, %+v",
			s, self.DecimalNumber))
	}
	return ret
}

type ValueChangeT struct {
	ScalarValueChange *ScalarValueChangeT `parser:"@@" json:",omitempty"`
	VectorValueChange *VectorValueChangeT `parser:"| @@" json:",omitempty"`
}

func (self ValueChangeT) GetIdCode() string {
	switch {
	case self.ScalarValueChange != nil:
		return self.ScalarValueChange.GetIdCode()
	case self.VectorValueChange != nil:
		return self.VectorValueChange.GetCode()
	}
	panic(fmt.Sprintf("unreachable: %+v", self))
}

func (self ValueChangeT) GetValue() string {
	switch {
	case self.ScalarValueChange != nil:
		return self.ScalarValueChange.GetValue()
	case self.VectorValueChange != nil:
		return self.VectorValueChange.GetValue()
	}
	panic(fmt.Sprintf("unreachable: %+v", self))
}

type ScalarValueChangeT struct {
	Pos    lexer.Position
	Value  ValueT `parser:"@@" json:",omitempty"`
	IdCode string `parser:"@IdCode" json:",omitempty"`
	// Garble is used to work around the tokenizer being unable to
	// make a distinction between [`x*@`] and [`x`, `*@`]. The
	// correct way to handle this is to write a custom lexer, but
	// since this seems to be the only place where it matters, this is
	// somewhat easier and kind of achieves the same goal.
	Garble string `parser:"| @IdCode" json:",omitempty"`
}

func (self ScalarValueChangeT) GetIdCode() string {
	if self.Garble != "" {
		return string(self.Garble[1:])
	}
	return self.IdCode
}

func (self ScalarValueChangeT) GetValue() string {
	g := self.Garble
	if g != "" {
		if len(g) < 2 {
			panic(fmt.Sprintf("garble was weird: %+v", self))
		}
		return string(g[0])
	}
	return self.Value.Value
}

type ValueT struct {
	Value string `parser:"@(\"0\" | \"1\" | \"x\" | \"X\"| \"z\" | \"Z\")" json:",omitempty"`
}

type VectorValueChangeT struct {
	VectorValueChange1 *VectorValueChange1T `parser:"@@" json:",omitempty"`
	VectorValueChange2 *VectorValueChange2T `parser:"| @@" json:",omitempty"`
	VectorValueChange3 *VectorValueChange3T `parser:"| @@" json:",omitempty"`
}

func (self VectorValueChangeT) GetCode() string {
	var ret string
	switch {
	case self.VectorValueChange1 != nil:
		return self.VectorValueChange1.GetCode()
	case self.VectorValueChange2 != nil:
		return self.VectorValueChange2.GetCode()
	case self.VectorValueChange3 != nil:
		return self.VectorValueChange3.GetCode()
	}
	return ret
}

func (self VectorValueChangeT) GetValue() string {
	var ret string
	switch {
	case self.VectorValueChange1 != nil:
		return self.VectorValueChange1.GetValue()
	case self.VectorValueChange2 != nil:
		return self.VectorValueChange2.GetValue()
	case self.VectorValueChange3 != nil:
		return self.VectorValueChange3.GetValue()
	}
	return ret
}

type VectorValueChange1T struct {
	Value  string `parser:"@Binstring" json:",omitempty"`
	IdCode string `parser:"@IdCode" json:",omitempty"`
}

func (self VectorValueChange1T) GetCode() string {
	return self.IdCode
}

func (self VectorValueChange1T) GetValue() string {
	return self.Value[1:]
}

type VectorValueChange2T struct {
	Value  string `parser:" @StateString  " json:",omitempty"`
	IdCode string `parser:" @IdCode  " json:",omitempty"`
}

func (self VectorValueChange2T) GetCode() string {
	return self.IdCode
}

func (self VectorValueChange2T) GetValue() string {
	return self.Value[1:]
}

type VectorValueChange3T struct {
	Value  string `parser:"@RealString" json:",omitempty"`
	IdCode string `parser:"@IdCode" json:",omitempty"`
}

func (self VectorValueChange3T) GetCode() string {
	return self.IdCode
}

func (self VectorValueChange3T) GetValue() string {
	return self.Value[1:]
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
