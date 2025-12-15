package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type LoaderFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(LoaderFixer{})
}

func (l LoaderFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errs []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-loader" {
			errs = append(errs, validation.CheckResult{
				Message:    "sw-loader is removed, use mt-loader instead.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-loader",
				Line:       node.Line,
			})
		}
	})
	return errs
}

func (l LoaderFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (l LoaderFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-loader" {
			node.Tag = "mt-loader"
		}
	})
	return nil
}
