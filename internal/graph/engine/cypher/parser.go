package cypher

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	maxExpressionDepth = 256
	maxOrderKeys       = 8
	maxListItems       = 100_000
)

type Direction uint8

const (
	Any Direction = iota
	Outbound
	Inbound
)

type Query struct {
	Unwind    *UnwindClause
	Matches   []MatchClause
	Where     Expr
	With      *Projection
	PostWhere Expr
	Return    *Projection
	Union     *Query
	UnionAll  bool
}

type UnwindClause struct {
	Expression Expr
	Alias      string
}

type MatchClause struct {
	Optional bool
	Patterns []Pattern
}

type Pattern struct {
	Nodes         []NodePattern
	Relationships []RelationshipPattern
}

type NodePattern struct {
	Variable   string
	Labels     []string
	Properties map[string]Expr
}

type RelationshipPattern struct {
	Variable       string
	Types          []string
	Properties     map[string]Expr
	Direction      Direction
	MinHops        int
	MaxHops        int // zero means unbounded only when VariableLength is true.
	VariableLength bool
}

type Expr interface{ cypherExpr() }

type Literal struct{ Value any }
type Variable struct{ Name string }
type Property struct{ Variable, Name string }
type LabelTest struct {
	Variable string
	Labels   []string
}
type ParameterExpr struct{ Name string }
type ListExpr struct{ Items []Expr }
type CallExpr struct {
	Name     string
	Args     []Expr
	Distinct bool
}
type UnaryExpr struct {
	Op    string
	Value Expr
}
type BinaryExpr struct {
	Op          string
	Left, Right Expr
}
type IsNullExpr struct {
	Value Expr
	Not   bool
}
type CaseExpr struct {
	Branches []CaseBranch
	Else     Expr
}
type CaseBranch struct{ When, Then Expr }
type ExistsPattern struct{ Pattern Pattern }

func (Literal) cypherExpr()       {}
func (Variable) cypherExpr()      {}
func (Property) cypherExpr()      {}
func (LabelTest) cypherExpr()     {}
func (ParameterExpr) cypherExpr() {}
func (ListExpr) cypherExpr()      {}
func (CallExpr) cypherExpr()      {}
func (UnaryExpr) cypherExpr()     {}
func (BinaryExpr) cypherExpr()    {}
func (IsNullExpr) cypherExpr()    {}
func (CaseExpr) cypherExpr()      {}
func (ExistsPattern) cypherExpr() {}

type Projection struct {
	Distinct bool
	Star     bool
	Items    []ProjectionItem
	OrderBy  []OrderItem
	Skip     int
	Limit    int
}

type ProjectionItem struct {
	Expression Expr
	Alias      string
}

type OrderItem struct {
	Expression Expr
	Descending bool
}

type parser struct {
	tokens []Token
	pos    int
	depth  int
}

// Parse validates and parses the complete read-only query. It never accepts a
// trailing token, which is important both for diagnostics and SQL-injection resistance.
func Parse(text string) (*Query, error) {
	tokens, err := Lex(text)
	if err != nil {
		return nil, err
	}
	if err := RejectWrites(tokens); err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	q, err := p.parseSingleQuery()
	if err != nil {
		return nil, err
	}
	if p.accept(Union) {
		q.UnionAll = p.accept(All)
		q.Union, err = p.parseSingleQuery()
		if err != nil {
			return nil, err
		}
	}
	for q.Union != nil && p.accept(Union) {
		tail := q
		for tail.Union != nil {
			tail = tail.Union
		}
		tail.UnionAll = p.accept(All)
		tail.Union, err = p.parseSingleQuery()
		if err != nil {
			return nil, err
		}
	}
	if err := p.expect(EOF, "end of query"); err != nil {
		return nil, err
	}
	if err := validateQueryLimits(q); err != nil {
		return nil, err
	}
	return q, nil
}

func validateQueryLimits(query *Query) error {
	for branch := query; branch != nil; branch = branch.Union {
		nodeVariables := map[string]bool{}
		edgeVariables := map[string]bool{}
		for _, match := range branch.Matches {
			for _, pattern := range match.Patterns {
				for _, node := range pattern.Nodes {
					if node.Variable != "" {
						nodeVariables[node.Variable] = true
					}
				}
				for _, relationship := range pattern.Relationships {
					if relationship.Variable != "" {
						edgeVariables[relationship.Variable] = true
					}
				}
			}
		}
		if len(nodeVariables) > 16 {
			return fmt.Errorf("cypher: query exceeds 16 node variables")
		}
		if len(edgeVariables) > 8 {
			return fmt.Errorf("cypher: query exceeds 8 relationship variables")
		}
	}
	return nil
}

func (p *parser) parseSingleQuery() (*Query, error) {
	q := &Query{}
	if p.accept(Unwind) {
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if err := p.expect(As, "AS after UNWIND expression"); err != nil {
			return nil, err
		}
		alias, err := p.ident("UNWIND alias")
		if err != nil {
			return nil, err
		}
		q.Unwind = &UnwindClause{Expression: expr, Alias: alias}
	}
	for p.at(Match) || p.at(Optional) {
		optional := p.accept(Optional)
		if optional {
			if err := p.expect(Match, "MATCH after OPTIONAL"); err != nil {
				return nil, err
			}
		} else {
			p.pos++
		}
		clause := MatchClause{Optional: optional}
		for {
			pattern, err := p.parsePattern()
			if err != nil {
				return nil, err
			}
			clause.Patterns = append(clause.Patterns, pattern)
			if !p.accept(Comma) {
				break
			}
		}
		q.Matches = append(q.Matches, clause)
	}
	if len(q.Matches) == 0 && q.Unwind == nil {
		return nil, p.errorf("expected MATCH, OPTIONAL MATCH, or UNWIND")
	}
	var err error
	if p.accept(Where) {
		q.Where, err = p.parseExpr(0)
		if err != nil {
			return nil, err
		}
	}
	if p.accept(With) {
		q.With, err = p.parseProjection()
		if err != nil {
			return nil, err
		}
		if p.accept(Where) {
			q.PostWhere, err = p.parseExpr(0)
			if err != nil {
				return nil, err
			}
		}
	}
	if p.accept(Return) {
		q.Return, err = p.parseProjection()
		if err != nil {
			return nil, err
		}
	}
	return q, nil
}

func (p *parser) parsePattern() (Pattern, error) {
	first, err := p.parseNode()
	if err != nil {
		return Pattern{}, err
	}
	pattern := Pattern{Nodes: []NodePattern{first}}
	for p.at(Dash) || p.at(LT) {
		rel, err := p.parseRelationship()
		if err != nil {
			return Pattern{}, err
		}
		node, err := p.parseNode()
		if err != nil {
			return Pattern{}, err
		}
		pattern.Relationships = append(pattern.Relationships, rel)
		pattern.Nodes = append(pattern.Nodes, node)
	}
	return pattern, nil
}

func (p *parser) parseNode() (NodePattern, error) {
	if err := p.expect(LParen, "'(' to start node pattern"); err != nil {
		return NodePattern{}, err
	}
	node := NodePattern{}
	if p.at(Identifier) {
		node.Variable = p.take().Text
	}
	for p.accept(Colon) {
		label, err := p.ident("node label")
		if err != nil {
			return NodePattern{}, err
		}
		node.Labels = append(node.Labels, label)
		for p.accept(Pipe) {
			p.accept(Colon)
			label, err = p.ident("node label after '|'")
			if err != nil {
				return NodePattern{}, err
			}
			node.Labels = append(node.Labels, label)
		}
	}
	if p.at(LBrace) {
		props, err := p.parseProperties()
		if err != nil {
			return NodePattern{}, err
		}
		node.Properties = props
	}
	if err := p.expect(RParen, "')' after node pattern"); err != nil {
		return NodePattern{}, err
	}
	return node, nil
}

func (p *parser) parseRelationship() (RelationshipPattern, error) {
	rel := RelationshipPattern{MinHops: 1, MaxHops: 1}
	left := p.accept(LT)
	if err := p.expect(Dash, "'-' in relationship pattern"); err != nil {
		return rel, err
	}
	if p.accept(LBracket) {
		if p.at(Identifier) {
			rel.Variable = p.take().Text
		}
		if p.accept(Colon) {
			for {
				typeName, err := p.ident("relationship type")
				if err != nil {
					return rel, err
				}
				rel.Types = append(rel.Types, typeName)
				if !p.accept(Pipe) {
					break
				}
				p.accept(Colon)
			}
		}
		if p.accept(Star) {
			rel.VariableLength = true
			rel.MinHops, rel.MaxHops = 1, 0
			if p.at(Number) {
				rel.MinHops, _ = strconv.Atoi(p.take().Text)
				rel.MaxHops = rel.MinHops
			}
			if p.accept(DotDot) {
				rel.MaxHops = 0
				if p.at(Number) {
					rel.MaxHops, _ = strconv.Atoi(p.take().Text)
				}
			}
			if rel.MinHops < 0 || rel.MaxHops > 0 && rel.MaxHops < rel.MinHops {
				return rel, p.errorf("invalid relationship hop range")
			}
		}
		if p.at(LBrace) {
			props, err := p.parseProperties()
			if err != nil {
				return rel, err
			}
			rel.Properties = props
		}
		if err := p.expect(RBracket, "']' after relationship pattern"); err != nil {
			return rel, err
		}
	}
	if err := p.expect(Dash, "closing '-' in relationship pattern"); err != nil {
		return rel, err
	}
	right := p.accept(GT)
	if left && right {
		return rel, p.errorf("relationship cannot point both directions")
	}
	switch {
	case left:
		rel.Direction = Inbound
	case right:
		rel.Direction = Outbound
	default:
		rel.Direction = Any
	}
	return rel, nil
}

func (p *parser) parseProperties() (map[string]Expr, error) {
	p.pos++ // {
	props := map[string]Expr{}
	if p.accept(RBrace) {
		return props, nil
	}
	for {
		name, err := p.ident("property name")
		if err != nil {
			return nil, err
		}
		if _, duplicate := props[name]; duplicate {
			return nil, p.errorf("duplicate property %q", name)
		}
		if err := p.expect(Colon, "':' after property name"); err != nil {
			return nil, err
		}
		value, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		props[name] = value
		if !p.accept(Comma) {
			break
		}
	}
	if err := p.expect(RBrace, "'}' after properties"); err != nil {
		return nil, err
	}
	return props, nil
}

func (p *parser) parseExpr(minPrecedence int) (Expr, error) {
	if p.depth >= maxExpressionDepth {
		return nil, p.errorf("expression nesting exceeds %d", maxExpressionDepth)
	}
	p.depth++
	defer func() { p.depth-- }()
	var left Expr
	var err error
	if p.accept(Not) {
		// NOT binds tighter than AND/OR but applies to comparisons and label
		// tests: NOT n.name = "x" means NOT (n.name = "x").
		value, parseErr := p.parseExpr(4)
		left, err = UnaryExpr{Op: "NOT", Value: value}, parseErr
	} else {
		left, err = p.parsePrimary()
	}
	if err != nil {
		return nil, err
	}
	for {
		op, precedence, width := p.binaryOperator()
		if precedence < minPrecedence {
			break
		}
		p.pos += width
		if op == "IS NULL" || op == "IS NOT NULL" {
			left = IsNullExpr{Value: left, Not: op == "IS NOT NULL"}
			continue
		}
		right, err := p.parseExpr(precedence + 1)
		if err != nil {
			return nil, err
		}
		left = BinaryExpr{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) binaryOperator() (string, int, int) {
	switch p.peek().Kind {
	case Or:
		return "OR", 1, 1
	case Xor:
		return "XOR", 2, 1
	case And:
		return "AND", 3, 1
	case Equal:
		return "=", 4, 1
	case NotEqual:
		return "<>", 4, 1
	case RegexEqual:
		return "=~", 4, 1
	case GT:
		return ">", 4, 1
	case LT:
		return "<", 4, 1
	case GTE:
		return ">=", 4, 1
	case LTE:
		return "<=", 4, 1
	case Contains:
		return "CONTAINS", 4, 1
	case In:
		return "IN", 4, 1
	case Starts:
		if p.peekN(1).Kind == With {
			return "STARTS WITH", 4, 2
		}
	case Ends:
		if p.peekN(1).Kind == With {
			return "ENDS WITH", 4, 2
		}
	case Not:
		if p.peekN(1).Kind == In {
			return "NOT IN", 4, 2
		}
	case Is:
		if p.peekN(1).Kind == Null {
			return "IS NULL", 4, 2
		}
		if p.peekN(1).Kind == Not && p.peekN(2).Kind == Null {
			return "IS NOT NULL", 4, 3
		}
	}
	return "", -1, 0
}

func (p *parser) parsePrimary() (Expr, error) {
	token := p.take()
	switch token.Kind {
	case String:
		return Literal{Value: token.Text}, nil
	case Number:
		if strings.Contains(token.Text, ".") {
			value, err := strconv.ParseFloat(token.Text, 64)
			if err != nil {
				return nil, p.errorf("invalid number %q", token.Text)
			}
			return Literal{Value: value}, nil
		}
		value, err := strconv.ParseInt(token.Text, 10, 64)
		if err != nil {
			return nil, p.errorf("invalid number %q", token.Text)
		}
		return Literal{Value: value}, nil
	case True:
		return Literal{Value: true}, nil
	case False:
		return Literal{Value: false}, nil
	case Null:
		return Literal{Value: nil}, nil
	case Parameter:
		return ParameterExpr{Name: token.Text}, nil
	case LParen:
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if err := p.expect(RParen, "')' after expression"); err != nil {
			return nil, err
		}
		return expr, nil
	case LBracket:
		items := []Expr{}
		if !p.accept(RBracket) {
			for {
				if len(items) == maxListItems {
					return nil, p.errorf("list exceeds %d items", maxListItems)
				}
				item, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				items = append(items, item)
				if !p.accept(Comma) {
					break
				}
			}
			if err := p.expect(RBracket, "']' after list"); err != nil {
				return nil, err
			}
		}
		return ListExpr{Items: items}, nil
	case Case:
		return p.parseCase()
	case Exists:
		if err := p.expect(LBrace, "'{' after EXISTS"); err != nil {
			return nil, err
		}
		pattern, err := p.parsePattern()
		if err != nil {
			return nil, err
		}
		if err := p.expect(RBrace, "'}' after EXISTS pattern"); err != nil {
			return nil, err
		}
		return ExistsPattern{Pattern: pattern}, nil
	case Identifier, Count, Sum, Avg, Min, Max, Collect, ToLower, ToUpper, ToString:
		name := token.Text
		if p.accept(LParen) {
			if !supportedFunction(name) {
				return nil, p.errorfAt(token, "unsupported function %q", name)
			}
			call := CallExpr{Name: name}
			call.Distinct = p.accept(Distinct)
			if p.accept(Star) {
				call.Args = []Expr{Variable{Name: "*"}}
				if err := p.expect(RParen, "')' after function arguments"); err != nil {
					return nil, err
				}
			} else if !p.accept(RParen) {
				for {
					arg, err := p.parseExpr(0)
					if err != nil {
						return nil, err
					}
					call.Args = append(call.Args, arg)
					if !p.accept(Comma) {
						break
					}
				}
				if err := p.expect(RParen, "')' after function arguments"); err != nil {
					return nil, err
				}
			}
			return call, nil
		}
		if token.Kind != Identifier {
			return nil, p.errorf("expected '(' after function %q", token.Text)
		}
		if p.accept(Dot) {
			property, err := p.ident("property name")
			if err != nil {
				return nil, err
			}
			return Property{Variable: name, Name: property}, nil
		}
		if p.accept(Colon) {
			labels := []string{}
			for {
				label, err := p.ident("label name")
				if err != nil {
					return nil, err
				}
				labels = append(labels, label)
				if !p.accept(Pipe) {
					break
				}
				p.accept(Colon)
			}
			return LabelTest{Variable: name, Labels: labels}, nil
		}
		return Variable{Name: name}, nil
	default:
		return nil, p.errorfAt(token, "expected expression")
	}
}

func supportedFunction(name string) bool {
	switch strings.ToLower(name) {
	case "count", "sum", "avg", "min", "max", "collect",
		"tolower", "toupper", "tostring", "tointeger", "tofloat",
		"size", "length", "reverse", "substring", "left", "right", "replace", "coalesce",
		"labels", "type", "id", "keys", "properties":
		return true
	default:
		return false
	}
}

func (p *parser) parseCase() (Expr, error) {
	result := CaseExpr{}
	for p.accept(When) {
		condition, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if err := p.expect(Then, "THEN in CASE expression"); err != nil {
			return nil, err
		}
		value, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		result.Branches = append(result.Branches, CaseBranch{When: condition, Then: value})
	}
	if len(result.Branches) == 0 {
		return nil, p.errorf("CASE requires at least one WHEN")
	}
	if p.accept(Else) {
		var err error
		result.Else, err = p.parseExpr(0)
		if err != nil {
			return nil, err
		}
	}
	if err := p.expect(End, "END after CASE expression"); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *parser) parseProjection() (*Projection, error) {
	projection := &Projection{Distinct: p.accept(Distinct)}
	if p.accept(Star) {
		projection.Star = true
	} else {
		for {
			expr, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			item := ProjectionItem{Expression: expr}
			if p.accept(As) {
				item.Alias, err = p.ident("projection alias")
				if err != nil {
					return nil, err
				}
			}
			projection.Items = append(projection.Items, item)
			if !p.accept(Comma) {
				break
			}
		}
	}
	if p.accept(Order) {
		if err := p.expect(By, "BY after ORDER"); err != nil {
			return nil, err
		}
		for {
			if len(projection.OrderBy) == maxOrderKeys {
				return nil, p.errorf("ORDER BY exceeds %d keys", maxOrderKeys)
			}
			expr, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			item := OrderItem{Expression: expr}
			if p.accept(Desc) {
				item.Descending = true
			} else {
				p.accept(Asc)
			}
			projection.OrderBy = append(projection.OrderBy, item)
			if !p.accept(Comma) {
				break
			}
		}
	}
	var err error
	if p.accept(Skip) {
		projection.Skip, err = p.nonnegativeInt("SKIP")
		if err != nil {
			return nil, err
		}
	}
	if p.accept(Limit) {
		projection.Limit, err = p.nonnegativeInt("LIMIT")
		if err != nil {
			return nil, err
		}
	}
	return projection, nil
}

func (p *parser) nonnegativeInt(clause string) (int, error) {
	if !p.at(Number) || strings.Contains(p.peek().Text, ".") {
		return 0, p.errorf("%s requires a non-negative integer", clause)
	}
	value, err := strconv.Atoi(p.take().Text)
	if err != nil || value < 0 {
		return 0, p.errorf("invalid %s value", clause)
	}
	return value, nil
}

func (p *parser) ident(description string) (string, error) {
	if !p.at(Identifier) {
		return "", p.errorf("expected %s", description)
	}
	return p.take().Text, nil
}

func (p *parser) expect(kind Kind, description string) error {
	if !p.accept(kind) {
		return p.errorf("expected %s", description)
	}
	return nil
}

func (p *parser) accept(kind Kind) bool {
	if !p.at(kind) {
		return false
	}
	p.pos++
	return true
}

func (p *parser) at(kind Kind) bool { return p.peek().Kind == kind }
func (p *parser) take() Token       { token := p.peek(); p.pos++; return token }
func (p *parser) peek() Token       { return p.peekN(0) }
func (p *parser) peekN(offset int) Token {
	if p.pos+offset >= len(p.tokens) {
		return Token{Kind: EOF, Pos: p.tokens[len(p.tokens)-1].Pos}
	}
	return p.tokens[p.pos+offset]
}

func (p *parser) errorf(format string, args ...any) error {
	return p.errorfAt(p.peek(), format, args...)
}

func (p *parser) errorfAt(token Token, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if token.Kind == EOF {
		return fmt.Errorf("cypher: %s at end of query", message)
	}
	return fmt.Errorf("cypher: %s at byte %d near %q", message, token.Pos, token.Text)
}

// RejectWrites performs an early, explicit read-only validation. The parser
// would reject these tokens as syntax too, but this diagnostic is stable and actionable.
func RejectWrites(tokens []Token) error {
	for _, token := range tokens {
		switch token.Kind {
		case Create, Delete, Detach, Set, Remove, Merge, Call, Yield, Add, Constraint, Drop:
			return fmt.Errorf("cypher: %s is not allowed; graph queries are read-only (byte %d)", strings.ToUpper(token.Text), token.Pos)
		}
	}
	return nil
}
