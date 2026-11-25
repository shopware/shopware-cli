package dev

import (
	"errors"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

func TestComposerAuditSummary(t *testing.T) {
	assert.Equal(t, "no security advisories", composerAuditSummary(shop.ComposerAudit{}))

	audit := shop.ComposerAudit{Advisories: map[string][]shop.ComposerAdvisory{
		"a/a": {{Title: "one"}, {Title: "two"}},
		"b/b": {{Title: "three"}},
	}}
	assert.Equal(t, "3 security advisories in 2 packages", composerAuditSummary(audit))
}

func TestRunComposerAuditWithoutExecutor(t *testing.T) {
	_, err := runComposerAudit(t.Context(), nil)

	assert.Error(t, err)
}

func TestOverviewComposerAudit(t *testing.T) {
	m := NewOverviewModel(t.Context(), "docker", "http://localhost:8000", "", "", "/tmp/project", nil, nil)
	m.loading = false

	assert.Contains(t, ansi.Strip(m.View(80, 0)), "checking")

	m, _ = m.Update(composerAuditLoadedMsg{err: errors.New("composer not found")})
	view := ansi.Strip(m.View(80, 0))
	assert.Contains(t, view, "unknown")
	assert.Contains(t, view, "composer not found")
	assert.NotContains(t, view, "Security advisories")

	m, _ = m.Update(composerAuditLoadedMsg{audit: shop.ComposerAudit{Advisories: map[string][]shop.ComposerAdvisory{
		"symfony/http-kernel": {{Title: "Request smuggling", CVE: "CVE-2026-1", Link: "https://example.com/advisory"}},
	}}})
	view = ansi.Strip(m.View(80, 0))
	assert.Contains(t, view, "1 security advisory in 1 package")
	assert.Contains(t, view, "Security advisories")
	assert.Contains(t, view, "symfony/http-kernel")
	assert.Contains(t, view, "CVE-2026-1: Request smuggling")
}

func TestComposerAuditResultReachesOverviewDuringTask(t *testing.T) {
	m := newTestModel(t)
	m.phase = phaseTask
	m.task = tui.NewTask("Building...")

	updated, _ := m.Update(composerAuditLoadedMsg{err: errors.New("composer not found")})

	overview := updated.(Model).overview
	assert.False(t, overview.auditLoading)
	assert.Error(t, overview.auditErr)
}
