package transformer

import (
	"github.com/special-place-administrator/remindb-Local-Hub/internal/tokens"
	"github.com/special-place-administrator/remindb-Local-Hub/pkg/parser"
)

func setTokenCount(n *parser.ContextNode) {
	n.TokenCount = tokens.Estimate(n.Content)
}
