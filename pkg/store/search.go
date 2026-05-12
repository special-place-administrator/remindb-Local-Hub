package store

import (
	"context"
	"database/sql"
	"strings"
)

type RankedNode struct {
	Node *Node
	Rank float64
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSearchFTS, rewriteQuery(query), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) SearchRanked(ctx context.Context, query string, limit int) ([]*RankedNode, error) {
	rows, err := s.db.QueryContext(ctx, qSearchRanked, rewriteQuery(query), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*RankedNode
	for rows.Next() {
		var n Node
		var parentID sql.NullString
		var rank float64

		err := rows.Scan(
			&n.ID, &parentID, &n.SourceFile, &n.NodeType, &n.Depth,
			&n.Label, &n.Content, &n.Format, &n.TokenCount, &n.ContentHash,
			&n.Temperature, &n.AccessCount, &n.LastAccessed,
			&n.CreatedAt, &n.UpdatedAt, &rank,
		)
		if err != nil {
			return nil, err
		}

		n.ParentID = parentID.String
		out = append(out, &RankedNode{Node: &n, Rank: rank})
	}
	return out, rows.Err()
}

// Convert a natural-language query into FTS5 OR syntax. Terms containing
// characters outside [A-Za-z0-9_] (hyphens, dots, slashes, non-ASCII
// letters) are wrapped in quotes and matched as FTS5 phrases so the
// bareword parser does not treat them as column references. Explicit FTS5
// operators remain operators, while adjacent ordinary terms are OR-joined.
func rewriteQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return q
	}

	tokens := splitFTSQuery(q)
	if len(tokens) == 0 {
		return q
	}

	rewritten := make([]string, 0, len(tokens)*2)
	prevWasOperator := false
	for i, token := range tokens {
		isOperator := isFTSOperator(token)
		if i > 0 && !prevWasOperator && !isOperator {
			rewritten = append(rewritten, "OR")
		}
		rewritten = append(rewritten, rewriteFTSToken(token))
		prevWasOperator = isOperator
	}

	return strings.Join(rewritten, " ")
}

func splitFTSQuery(q string) []string {
	var tokens []string
	for i := 0; i < len(q); {
		for i < len(q) && isSpace(q[i]) {
			i++
		}
		if i >= len(q) {
			break
		}

		start := i
		switch q[i] {
		case '"':
			i++
			for i < len(q) {
				if q[i] == '"' {
					i++
					break
				}
				i++
			}
		default:
			depth := 0
			for i < len(q) {
				switch q[i] {
				case '(':
					depth++
				case ')':
					if depth > 0 {
						depth--
					}
				default:
					if depth == 0 && isSpace(q[i]) {
						goto done
					}
				}
				i++
			}
		}
	done:
		tokens = append(tokens, q[start:i])
	}
	return tokens
}

func rewriteFTSToken(s string) string {
	if strings.HasPrefix(s, `"`) {
		return s
	}
	if strings.HasPrefix(s, "NEAR(") && strings.HasSuffix(s, ")") {
		return rewriteNearExpression(s)
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		return rewriteParenthesizedExpression(s)
	}
	if rewritten, ok := rewriteColumnExpression(s); ok {
		return rewritten
	}
	if strings.HasSuffix(s, "*") && len(s) > 1 {
		stem := strings.TrimSuffix(s, "*")
		if needsFtsQuoting(stem) {
			return quoteFTSToken(stem) + "*"
		}
		return s
	}
	if needsFtsQuoting(s) {
		return quoteFTSToken(s)
	}
	return s
}

func isFTSOperator(s string) bool {
	return s == "OR" || s == "AND" || s == "NOT"
}

func rewriteParenthesizedExpression(s string) string {
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return s
	}
	return "(" + rewriteFTSExpressionTerms(inner) + ")"
}

func rewriteNearExpression(s string) string {
	inner := strings.TrimSpace(s[len("NEAR(") : len(s)-1])
	if inner == "" {
		return s
	}

	terms, distance, hasDistance := splitTopLevelComma(inner)
	rewritten := "NEAR(" + rewriteFTSExpressionTerms(terms)
	if hasDistance {
		rewritten += ", " + distance
	}
	return rewritten + ")"
}

func rewriteColumnExpression(s string) (string, bool) {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 || idx == len(s)-1 {
		return "", false
	}

	field := s[:idx]
	if !isFTSColumn(field) {
		return "", false
	}

	return field + ":" + rewriteFTSToken(s[idx+1:]), true
}

func rewriteFTSExpressionTerms(s string) string {
	tokens := splitFTSQuery(s)
	rewritten := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if isFTSOperator(token) {
			rewritten = append(rewritten, token)
			continue
		}
		rewritten = append(rewritten, rewriteFTSToken(token))
	}
	return strings.Join(rewritten, " ")
}

func splitTopLevelComma(s string) (before, after string, ok bool) {
	depth := 0
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				depth++
			}
		case ')':
			if !inQuote && depth > 0 {
				depth--
			}
		case ',':
			if !inQuote && depth == 0 {
				return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
			}
		}
	}
	return strings.TrimSpace(s), "", false
}

func isFTSColumn(s string) bool {
	return s == "label" || s == "content" || s == "node_type"
}

func quoteFTSToken(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// needsFtsQuoting reports whether s contains any character that FTS5's
// bareword parser rejects. Hyphenated tokens like "LR-2026-05-09" trip
// the column-reference parser ("no such column: 2026"); quoting forces
// FTS5 to treat the whole string as a phrase to match against the
// tokenized content.
func needsFtsQuoting(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return true
		}
	}
	return false
}
