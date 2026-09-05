package vcd

import (
	"io"

	"github.com/alecthomas/participle/v2/lexer"
)

// NodeBuilder turns the streaming parser's events into the grammar's node
// types. It is shared by ParseFile and by the incremental JSON writer, so
// that a streamed document and a parsed one are built the same way.
//
// A dump block is the one construct that spans several events, so the
// builder buffers its value changes until DumpEnd.
type NodeBuilder struct {
	changes []*ValueChangeT
	inDump  bool
	pos     lexer.Position
}

// SetPos records where the next scalar value change came from, so that the
// built node carries the same position the grammar-driven parser records.
func (b *NodeBuilder) SetPos(p lexer.Position) { b.pos = p }

// Timestamp returns the node for a `#123` simulation time. raw is kept
// verbatim, since the grammar stores the original token.
func (b *NodeBuilder) Timestamp(_ uint64, raw []byte) *SimulationCommandT {
	return &SimulationCommandT{
		SimulationTime: &SimulationTimeT{DecimalNumber: string(raw)},
	}
}

// ValueChange returns the node for one value change, or nil when the change
// belongs to an open dump block and is being buffered.
func (b *NodeBuilder) ValueChange(kind ValueKind, value, idcode []byte) *SimulationCommandT {
	vc := newValueChange(kind, value, idcode)
	if vc.ScalarValueChange != nil {
		vc.ScalarValueChange.Pos = b.pos
	}
	if b.inDump {
		b.changes = append(b.changes, vc)
		return nil
	}
	return &SimulationCommandT{ValueChange: vc}
}

// DumpBegin opens a dump block.
func (b *NodeBuilder) DumpBegin(DumpKind) error {
	b.inDump, b.changes = true, nil
	return nil
}

// DumpEnd closes a dump block and returns the node for it.
func (b *NodeBuilder) DumpEnd(k DumpKind) *SimulationCommandT {
	b.inDump = false
	c := b.changes
	b.changes = nil
	s := &SimulationCommandT{}
	switch k {
	case DumpAll:
		s.Dumpall = &DumpallT{Kw: true, ValueChange: c, KwEnd: true}
	case DumpOff:
		s.Dumpoff = &DumpoffT{Kw: true, ValueChange: c, KwEnd: true}
	case DumpOn:
		s.Dumpon = &DumponT{Kw: true, ValueChange: c, KwEnd: true}
	case DumpVars:
		s.Dumpvars = &DumpvarsT{Kw: true, ValueChange: c, KwEnd: true}
	}
	return s
}

// Directive returns the node for a `$name ... $end` block in the value
// change section, or nil when the grammar has no place for it.
func (b *NodeBuilder) Directive(name string, _ []byte) *SimulationCommandT {
	// The grammar has slots for $attrbegin and $attrend here, though its
	// value-change lexer state never reaches them. A $comment in the body
	// has no slot at all, and is dropped.
	switch name {
	case "$attrbegin":
		return &SimulationCommandT{Attrbegin: boolPtr(true)}
	case "$attrend":
		return &SimulationCommandT{Attrend: boolPtr(true)}
	}
	return nil
}

// newValueChange builds the node participle would have produced for this
// value change.
func newValueChange(kind ValueKind, value, idcode []byte) *ValueChangeT {
	switch kind {
	case ValueBinary:
		return &ValueChangeT{VectorValueChange: &VectorValueChangeT{
			VectorValueChange1: &VectorValueChange1T{
				Value: string(value), IdCode: string(idcode)}}}
	case ValueString:
		return &ValueChangeT{VectorValueChange: &VectorValueChangeT{
			VectorValueChange2: &VectorValueChange2T{
				Value: string(value), IdCode: string(idcode)}}}
	case ValueReal:
		return &ValueChangeT{VectorValueChange: &VectorValueChangeT{
			VectorValueChange3: &VectorValueChange3T{
				Value: string(value), IdCode: string(idcode)}}}
	}
	// A scalar value is glued to its code, so the grammar's ValueT
	// alternative never fires and everything lands in Garble. Rebuilding
	// the original word reproduces that exactly.
	return &ValueChangeT{ScalarValueChange: &ScalarValueChangeT{
		Garble: string(value) + string(idcode)}}
}

func boolPtr(b bool) *bool { return &b }

// astHandler collects the whole File in memory. It is what ParseFile uses.
type astHandler struct {
	file File
	b    NodeBuilder
}

func (a *astHandler) Declaration(d *DeclarationCommandT) error {
	a.file.DeclarationCommand = append(a.file.DeclarationCommand, d)
	return nil
}

func (a *astHandler) sim(s *SimulationCommandT) error {
	if s != nil {
		a.file.SimulationCommand = append(a.file.SimulationCommand, s)
	}
	return nil
}

func (a *astHandler) Timestamp(ts uint64, raw []byte) error {
	return a.sim(a.b.Timestamp(ts, raw))
}

func (a *astHandler) ValueChange(kind ValueKind, value, idcode []byte) error {
	return a.sim(a.b.ValueChange(kind, value, idcode))
}

func (a *astHandler) SetPos(p lexer.Position) { a.b.SetPos(p) }

func (a *astHandler) DumpBegin(k DumpKind) error { return a.b.DumpBegin(k) }
func (a *astHandler) DumpEnd(k DumpKind) error   { return a.sim(a.b.DumpEnd(k)) }

func (a *astHandler) Directive(name string, text []byte) error {
	return a.sim(a.b.Directive(name, text))
}

// ParseFile reads a whole VCD file into memory, the way
// NewParser[File]().Parse does, but without ever holding the input text or
// its token stream. Prefer Parse with your own Handler for large files:
// the returned File still grows with the number of value changes.
//
func ParseFile(filename string, r io.Reader) (*File, error) {
	var a astHandler
	if err := Parse(filename, r, &a); err != nil {
		return nil, err
	}
	return &a.file, nil
}

// NormalizeForCompare rewrites a File into a canonical form, so that two
// parses of the same input can be compared with reflect.DeepEqual. It
// folds the two representation details that carry no meaning:
//
//   - ScalarValueChangeT.Pos, which the grammar-driven parser fills in and
//     the streaming parser does not track;
//   - the two ways a scalar value change reaches the grammar. `0 !` parses
//     as the two-token `ValueT IdCode` alternative and `0!` as the
//     one-token Garble alternative, purely because of a space. Both mean
//     the same change, so both become Garble.
//
// It edits f in place and returns it.
func NormalizeForCompare(f *File) *File {
	if f == nil {
		return nil
	}
	for _, s := range f.SimulationCommand {
		if s == nil {
			continue
		}
		normalizeChange(s.ValueChange)
		if d := s.Dumpall; d != nil {
			normalizeChanges(d.ValueChange)
		}
		if d := s.Dumpoff; d != nil {
			normalizeChanges(d.ValueChange)
		}
		if d := s.Dumpon; d != nil {
			normalizeChanges(d.ValueChange)
		}
		if d := s.Dumpvars; d != nil {
			normalizeChanges(d.ValueChange)
		}
	}
	return f
}

func normalizeChanges(vcs []*ValueChangeT) {
	for _, vc := range vcs {
		normalizeChange(vc)
	}
}

func normalizeChange(vc *ValueChangeT) {
	if vc == nil || vc.ScalarValueChange == nil {
		return
	}
	s := vc.ScalarValueChange
	s.Pos = lexer.Position{}
	if s.Garble == "" {
		s.Garble = s.Value.Value + s.IdCode
		s.Value.Value, s.IdCode = "", ""
	}
}
