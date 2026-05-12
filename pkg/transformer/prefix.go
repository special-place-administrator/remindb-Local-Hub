package transformer

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/radimsem/remindb/pkg/parser"
)

// Strip compileRoot (or the longest common dir if empty) so hashed paths stay stable across call shapes.
func compressPrefix(nodes []*parser.ContextNode, compileRoot string) {
	if len(nodes) == 0 {
		return
	}

	prefix := compileRoot
	if prefix != "" {
		sep := pathSeparator(prefix)
		prefix = cleanPath(prefix)
		if !strings.HasSuffix(prefix, sep) {
			prefix += sep
		}
	} else {
		prefix = commonDirPrefix(nodes)
	}

	if prefix == "" {
		return
	}

	for _, n := range nodes {
		n.SourceFile = strings.TrimPrefix(n.SourceFile, prefix)
	}
}

func commonDirPrefix(nodes []*parser.ContextNode) string {
	sep := pathSeparator(nodes[0].SourceFile)
	parts := splitPath(dirPath(nodes[0].SourceFile), sep)

	for _, n := range nodes[1:] {
		if pathSeparator(n.SourceFile) != sep {
			return ""
		}
		np := splitPath(dirPath(n.SourceFile), sep)
		parts = commonParts(parts, np)

		if len(parts) == 0 {
			return ""
		}
	}

	result := strings.Join(parts, sep)
	if result == "" || result == "." || result == "/" || result == `\` {
		return ""
	}

	return result + sep
}

func splitPath(p, sep string) []string {
	p = cleanPath(p)
	if p == "." {
		return nil
	}

	return strings.Split(p, sep)
}

func pathSeparator(p string) string {
	if strings.Contains(p, "/") && !strings.Contains(p, `\`) {
		return "/"
	}
	return string(filepath.Separator)
}

func cleanPath(p string) string {
	if pathSeparator(p) == "/" {
		return path.Clean(p)
	}
	return filepath.Clean(p)
}

func dirPath(p string) string {
	if pathSeparator(p) == "/" {
		return path.Dir(p)
	}
	return filepath.Dir(p)
}

func commonParts(a, b []string) []string {
	i := 0
	for i < min(len(a), len(b)) && a[i] == b[i] {
		i++
	}
	return a[:i]
}
