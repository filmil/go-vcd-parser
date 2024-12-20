package vcd

import (
	"fmt"

	"github.com/alecthomas/participle/v2/lexer"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// See: https://github.com/google/re2/wiki/Syntax
const (
	BinstringPattern  = `[bB]([10xXzZuU])+`
	FloatPattern      = `[+-]?([0-9]*\.?[0-9]+|[0-9]+\.?[0-9]*)([eE][+-]?[0-9]+)?` // Generated by Gemini.
	RealStringPattern = `[r|R]` + FloatPattern
	IntPattern        = `[+-]?[0-9]+`
	StringPattern     = `\w+`
	TimestampPattern  = `#\d+`
	WhitespacePattern = `\s+`
	IdentifierPattern = `[a-zA-Z_][a-zA-Z0-9_]*`
	StatePattern      = `s` + IdentifierPattern
)

// IntoRule converts a SimpleRule into a (complex) Rule.
func IntoRule(rules []lexer.SimpleRule) []lexer.Rule {
	var ret []lexer.Rule
	for _, r := range rules {
		newRule := lexer.Rule{
			Name:    r.Name,
			Pattern: r.Pattern,
			Action:  nil,
		}
		ret = append(ret, newRule)
	}
	return ret
}

func GenKeywordTokens() []lexer.SimpleRule {
	var caser = cases.Title(language.AmericanEnglish)
	var keywords = []string{
		// These are implemented via special tokenizer states.
		//"comment",
		//"date",
		//"var",
		//"version",
		//"enddefinitions",
		"scope",
		"timescale",
		"upscope",
		"dumpall",
		"dumpon",
		"dumpoff",
		"dumpvars",
		//"end",
	}

	var ret []lexer.SimpleRule
	for _, kw := range keywords {
		ret = append(ret, lexer.SimpleRule{
			Name: fmt.Sprintf("Kw%v", caser.String(kw)),
			// Don't forget to escape your `$`
			Pattern: fmt.Sprintf(`\$%v`, kw),
		})
	}
	return ret
}

var stringlessRules = []lexer.SimpleRule{
	{
		Name:    "Timestamp",
		Pattern: TimestampPattern,
	},
	{
		Name:    "Binstring",
		Pattern: BinstringPattern,
	},
	{
		Name:    "RealString",
		Pattern: RealStringPattern,
	},
	{
		Name:    "StateString",
		Pattern: StatePattern,
	},
	{
		Name:    "IdCode",
		Pattern: AnyWordPattern,
	},
	{
		Name:    "Int",
		Pattern: IntPattern,
	},
	{
		Name:    "Float",
		Pattern: FloatPattern,
	},
	{
		Name:    "ws",
		Pattern: WhitespacePattern,
	},
}

var rules = []lexer.SimpleRule{
	{
		Name:    "Timestamp",
		Pattern: TimestampPattern,
	},
	{
		Name:    "Binstring",
		Pattern: BinstringPattern,
	},
	{
		Name:    "Ident",
		Pattern: IdentifierPattern,
	},
	{
		Name:    "RealString",
		Pattern: RealStringPattern,
	},
	{
		Name:    "Int",
		Pattern: IntPattern,
	},
	{
		Name:    "Float",
		Pattern: FloatPattern,
	},
	{
		Name:    "IdCode",
		Pattern: AnyWordPattern,
	},
	{
		Name:    "String",
		Pattern: StringPattern,
	},
	{
		Name:    "ws",
		Pattern: WhitespacePattern,
	},
}

// SimpleRules returns keywords plus basic token types.
func SimpleRules(additions []lexer.Rule) []lexer.Rule {
	ret := append(IntoRule(GenKeywordTokens()), additions...)
	ret = append(ret, IntoRule(rules)...)
	return ret
}

// SimpleStringlessRules is the same as above, except contains no "stringy" tokens, except
// "any nonwhitespace"
func SimpleStringlessRules(additions []lexer.Rule) []lexer.Rule {
	ret := append(IntoRule(GenKeywordTokens()), additions...)
	ret = append(ret, IntoRule(stringlessRules)...)
	return ret
}

const (
	// Any string that consists of non-whitespace.
	AnyWordPattern = `[^\r\n\t\f\v ]+`
	PunctPattern   = "([\\(\\)!-/:-@[-`{-~])+\\S*"
)

// anyWordsEndingWithKwEnd is a set of lexical rules that accept a sequence of
// any words, separated by whitespace, until `$end` is encountered.
var anyWordsEndingWithKwEnd = []lexer.Rule{
	// Allows matching $end in $end, but not in $enddefinition.
	{Name: "KwEndSpecial", Pattern: `\$end(\s+|$)`, Action: lexer.Pop()},
	{Name: "AnyNonspace", Pattern: AnyWordPattern, Action: nil},
	{Name: "ws", Pattern: `\s+`, Action: nil},
}

var anyWordsEndingWithKwEndWithWs = []lexer.Rule{
	// Allows matching $end in $end, but not in $enddefinition.
	{Name: "KwEndSpecial", Pattern: `\$end(\s+|$)`, Action: lexer.Pop()},
	{Name: "AnyNonspace", Pattern: AnyWordPattern, Action: nil},
	{Name: "Ws", Pattern: `\s+`, Action: nil},
}

// NewLexer returns a built lexer that produces a valid stream of VCD tokens.
//
// The lexer is lightly stateful to allow date and comment keywords and such.
func NewLexer() *lexer.StatefulDefinition {
	rules := lexer.Rules{
		"Root": SimpleRules([]lexer.Rule{
			{Name: "KwDate", Pattern: `\$date`, Action: lexer.Push("DateTokens")},
			{Name: "KwComment", Pattern: `\$comment`, Action: lexer.Push("CommentTokens")},
			{Name: "KwVersion", Pattern: `\$version`, Action: lexer.Push("VersionTokens")},
			{Name: "KwVar", Pattern: `\$var`, Action: lexer.Push("VarTokens")},
			{Name: "KwAttrbegin", Pattern: `\$attrbegin`, Action: lexer.Push("AttrBeginTokens")},
			{Name: "KwAttrend", Pattern: `\$attrend`, Action: lexer.Push("AttrEndTokens")},
			// Longer keywords must be before shorter ones, else mixups will occur.
			{Name: "KwEnddefinitions", Pattern: `\$enddefinitions`, Action: lexer.Push("AfterEnddefinitions")},
			{Name: "KwEnd", Pattern: `\$end`, Action: nil},
		}),
		"DateTokens": anyWordsEndingWithKwEnd,
		// Is this unnecessary?
		"CommentTokens":   {lexer.Include("DateTokens")},
		"VersionTokens":   {lexer.Include("DateTokens")},
		"AttrBeginTokens": {lexer.Include("DateTokens")},
		"AttrEndTokens":   {lexer.Include("DateTokens")},
		"VarTokens":       anyWordsEndingWithKwEndWithWs,

		"AfterEnddefinitions": SimpleStringlessRules([]lexer.Rule{
			{Name: "KwEnd", Pattern: `\$end`, Action: nil},
			{Name: "KwComment", Pattern: `\$comment`, Action: lexer.Push("CommentTokens")},
		}),
	}
	//fmt.Printf("Rules: %+v", rules["AfterEnddefinitions"])
	return lexer.MustStateful(rules)
}

func NewIdLexer() *lexer.StatefulDefinition {
	return lexer.MustStateful(lexer.Rules{
		"Root": SimpleStringlessRules([]lexer.Rule{
			{Name: "KwComment", Pattern: `\$comment`, Action: lexer.Push("CommentTokens")},
		}),
		"CommentTokens": anyWordsEndingWithKwEnd,
	})
}
