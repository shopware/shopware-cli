package twiglinter

import (
	"strings"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
)

func RunFixerOnString(fixer TwigFixer, content string) (string, error) {
	nodes, err := html.NewParser(content)
	if err != nil {
		return "", err
	}

	err = fixer.Fix(nodes)
	if err != nil {
		return "", err
	}

	var buf strings.Builder

	for _, node := range nodes {
		buf.WriteString(node.Dump(0))
	}

	return buf.String(), nil
}

func RunCheckerOnString(fixer TwigFixer, content string) ([]validation.CheckResult, error) {
	nodes, err := html.NewParser(content)
	if err != nil {
		return nil, err
	}

	return fixer.Check(nodes), nil
}
