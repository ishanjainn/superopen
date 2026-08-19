package cypher

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind identifies one lexical element in the read-only Cypher dialect.
type Kind uint16

const (
	Invalid Kind = iota
	EOF
	Identifier
	String
	Number
	LParen
	RParen
	LBracket
	RBracket
	LBrace
	RBrace
	Dash
	GT
	LT
	Colon
	Dot
	Star
	Comma
	Equal
	RegexEqual
	NotEqual
	GTE
	LTE
	Pipe
	DotDot
	Parameter
	Match
	Where
	Return
	Order
	By
	Limit
	And
	Or
	As
	Distinct
	Count
	Contains
	Starts
	With
	Not
	Asc
	Desc
	Ends
	In
	Is
	Null
	Xor
	Skip
	Union
	Unwind
	Sum
	Avg
	Min
	Max
	Collect
	ToLower
	ToUpper
	ToString
	Case
	When
	Then
	Else
	End
	Create
	Delete
	Detach
	Set
	Remove
	Merge
	Optional
	Yield
	Call
	All
	True
	False
	Exists
	Mandatory
	Foreach
	On
	Add
	Constraint
	Do
	Drop
	For
	From
	Graph
	Of
	Require
	Scalar
	Unique
)

type Token struct {
	Kind Kind
	Text string
	Pos  int
}

const maxTokenBytes = 4095

var keywords = map[string]Kind{
	"MATCH": Match, "WHERE": Where, "RETURN": Return, "ORDER": Order,
	"BY": By, "LIMIT": Limit, "AND": And, "OR": Or, "AS": As,
	"DISTINCT": Distinct, "COUNT": Count, "CONTAINS": Contains,
	"STARTS": Starts, "WITH": With, "NOT": Not, "ASC": Asc, "DESC": Desc,
	"ENDS": Ends, "IN": In, "IS": Is, "NULL": Null, "XOR": Xor,
	"SKIP": Skip, "UNION": Union, "UNWIND": Unwind, "SUM": Sum,
	"AVG": Avg, "MIN": Min, "MAX": Max, "COLLECT": Collect,
	"TOLOWER": ToLower, "TOUPPER": ToUpper, "TOSTRING": ToString,
	"CASE": Case, "WHEN": When, "THEN": Then, "ELSE": Else, "END": End,
	"CREATE": Create, "DELETE": Delete, "DETACH": Detach, "SET": Set,
	"REMOVE": Remove, "MERGE": Merge, "OPTIONAL": Optional, "YIELD": Yield,
	"CALL": Call, "ALL": All, "TRUE": True, "FALSE": False,
	"EXISTS": Exists, "MANDATORY": Mandatory, "FOREACH": Foreach, "ON": On,
	"ADD": Add, "CONSTRAINT": Constraint, "DO": Do, "DROP": Drop,
	"FOR": For, "FROM": From, "GRAPH": Graph, "OF": Of, "REQUIRE": Require,
	"SCALAR": Scalar, "UNIQUE": Unique,
}

// Lex tokenizes query. Positions are byte offsets, matching Superopen diagnostics.
func Lex(query string) ([]Token, error) {
	if strings.IndexByte(query, 0) >= 0 {
		return nil, fmt.Errorf("cypher: NUL byte in query")
	}
	tokens := make([]Token, 0, 32)
	for pos := 0; pos < len(query); {
		if next, ok, err := skipSpaceOrComment(query, pos); ok || err != nil {
			if err != nil {
				return nil, err
			}
			pos = next
			continue
		}
		start := pos
		if text, next, ok, err := scanQuoted(query, pos); ok || err != nil {
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, Token{Kind: String, Text: text, Pos: start})
			pos = next
			continue
		}
		if query[pos] == '$' {
			pos++
			end := scanIdentifier(query, pos)
			if end == pos {
				return nil, fmt.Errorf("cypher: expected parameter name at byte %d", start)
			}
			tokens = append(tokens, Token{Kind: Parameter, Text: query[pos:end], Pos: start})
			pos = end
			continue
		}
		if end := scanIdentifier(query, pos); end > pos {
			text := query[pos:end]
			kind := Identifier
			if kw, ok := keywords[strings.ToUpper(text)]; ok {
				kind = kw
			}
			tokens = append(tokens, Token{Kind: kind, Text: text, Pos: start})
			pos = end
			continue
		}
		if end := scanNumber(query, pos); end > pos {
			tokens = append(tokens, Token{Kind: Number, Text: query[pos:end], Pos: start})
			pos = end
			continue
		}
		if pos+1 < len(query) {
			if kind, ok := twoCharacterKind(query[pos : pos+2]); ok {
				tokens = append(tokens, Token{Kind: kind, Text: query[pos : pos+2], Pos: start})
				pos += 2
				continue
			}
		}
		if kind, ok := oneCharacterKind(query[pos]); ok {
			tokens = append(tokens, Token{Kind: kind, Text: query[pos : pos+1], Pos: start})
			pos++
			continue
		}
		_, width := utf8.DecodeRuneInString(query[pos:])
		return nil, fmt.Errorf("cypher: unexpected character %q at byte %d", query[pos:pos+width], pos)
	}
	tokens = append(tokens, Token{Kind: EOF, Pos: len(query)})
	return tokens, nil
}

func skipSpaceOrComment(s string, pos int) (int, bool, error) {
	r, width := utf8.DecodeRuneInString(s[pos:])
	if unicode.IsSpace(r) {
		return pos + width, true, nil
	}
	if pos+1 >= len(s) {
		return pos, false, nil
	}
	pair := s[pos : pos+2]
	if pair == "//" || pair == "--" {
		if end := strings.IndexByte(s[pos+2:], '\n'); end >= 0 {
			return pos + 2 + end + 1, true, nil
		}
		return len(s), true, nil
	}
	if pair == "/*" {
		end := strings.Index(s[pos+2:], "*/")
		if end < 0 {
			return pos, true, fmt.Errorf("cypher: unterminated comment at byte %d", pos)
		}
		return pos + 2 + end + 2, true, nil
	}
	return pos, false, nil
}

func scanQuoted(s string, pos int) (string, int, bool, error) {
	if s[pos] != '\'' && s[pos] != '"' && s[pos] != '`' {
		return "", pos, false, nil
	}
	quote := s[pos]
	var out strings.Builder
	for i := pos + 1; i < len(s); {
		if s[i] == quote {
			return out.String(), i + 1, true, nil
		}
		if s[i] == '\\' {
			if i+1 >= len(s) {
				return "", pos, true, fmt.Errorf("cypher: unterminated escape at byte %d", i)
			}
			i++
			if out.Len() < maxTokenBytes {
				switch s[i] {
				case 'n':
					out.WriteByte('\n')
				case 'r':
					out.WriteByte('\r')
				case 't':
					out.WriteByte('\t')
				default:
					out.WriteByte(s[i])
				}
			}
			i++
		} else {
			if out.Len() < maxTokenBytes {
				out.WriteByte(s[i])
			}
			i++
		}
	}
	return "", pos, true, fmt.Errorf("cypher: unterminated string at byte %d", pos)
}

func scanIdentifier(s string, pos int) int {
	if pos >= len(s) {
		return pos
	}
	r, width := utf8.DecodeRuneInString(s[pos:])
	if r != '_' && !unicode.IsLetter(r) {
		return pos
	}
	i := pos + width
	for i < len(s) {
		r, width = utf8.DecodeRuneInString(s[i:])
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		i += width
	}
	return i
}

func scanNumber(s string, pos int) int {
	i := pos
	if s[i] == '.' {
		if i+1 >= len(s) || s[i+1] < '0' || s[i+1] > '9' {
			return pos
		}
		i++
	}
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if digits == 0 {
		return pos
	}
	if i < len(s) && s[i] == '.' && (i+1 >= len(s) || s[i+1] != '.') {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	return i
}

func twoCharacterKind(text string) (Kind, bool) {
	switch text {
	case "!=", "<>":
		return NotEqual, true
	case "=~":
		return RegexEqual, true
	case ">=":
		return GTE, true
	case "<=":
		return LTE, true
	case "..":
		return DotDot, true
	default:
		return Invalid, false
	}
}

func oneCharacterKind(char byte) (Kind, bool) {
	kinds := map[byte]Kind{'(': LParen, ')': RParen, '[': LBracket, ']': RBracket,
		'{': LBrace, '}': RBrace, '-': Dash, '>': GT, '<': LT, ':': Colon,
		'.': Dot, '*': Star, ',': Comma, '=': Equal, '|': Pipe}
	kind, ok := kinds[char]
	return kind, ok
}
