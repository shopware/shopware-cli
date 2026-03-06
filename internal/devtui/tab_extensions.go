package devtui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/shop"
)

type extensionInputStep int

const (
	inputStepNone extensionInputStep = iota
	inputStepToken
	inputStepLoadingPackages
	inputStepPackageList
)

type ExtensionsModel struct {
	table          table.Model
	loading        bool
	err            error
	statusMsg      string
	config         *shop.Config
	extensions     adminSdk.ExtensionList
	configured     bool
	executor       executor.Executor
	projectRoot    string
	inputStep      extensionInputStep
	tokenInput     textinput.Model
	filterInput    textinput.Model
	packages       []packageEntry
	filteredPkgs   []packageEntry
	packageTable   table.Model
	spinner        spinner.Model
	width          int
	height         int
}

type packageEntry struct {
	name        string
	description string
}

type packagesLoadedMsg struct {
	packages []packageEntry
	err      error
}

type extensionsLoadedMsg struct {
	extensions adminSdk.ExtensionList
	err        error
}

type extensionActionDoneMsg struct {
	err error
}

type composerRequireDoneMsg struct {
	err error
}

func NewExtensionsModel(config *shop.Config, exec executor.Executor, projectRoot string) ExtensionsModel {
	columns := []table.Column{
		{Title: "Name", Width: 30},
		{Title: "Label", Width: 30},
		{Title: "Version", Width: 12},
		{Title: "Type", Width: 10},
		{Title: "Status", Width: 35},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	t.SetStyles(s)

	tokenTi := textinput.New()
	tokenTi.Placeholder = "your-token-here"
	tokenTi.Prompt = "Token: "
	tokenTi.CharLimit = 200

	filterTi := textinput.New()
	filterTi.Placeholder = "type to filter..."
	filterTi.Prompt = "Filter: "
	filterTi.CharLimit = 100

	pkgColumns := []table.Column{
		{Title: "Package", Width: 40},
		{Title: "Description", Width: 50},
	}
	pkgTable := table.New(
		table.WithColumns(pkgColumns),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	pkgStyles := table.DefaultStyles()
	pkgStyles.Header = pkgStyles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	pkgTable.SetStyles(pkgStyles)

	sp := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205"))),
	)

	return ExtensionsModel{
		table:        t,
		loading:      true,
		config:       config,
		configured:   config.IsAdminAPIConfigured(),
		executor:     exec,
		projectRoot:  projectRoot,
		tokenInput:   tokenTi,
		filterInput:  filterTi,
		packageTable: pkgTable,
		spinner:      sp,
	}
}

func (m ExtensionsModel) Init() tea.Cmd {
	if !m.configured {
		return nil
	}
	return m.loadExtensions()
}

func (m ExtensionsModel) Update(msg tea.Msg) (ExtensionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case extensionsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.extensions = msg.extensions
		m.statusMsg = ""
		m.updateTableRows()
		return m, nil

	case extensionActionDoneMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Error: " + msg.err.Error())
			return m, nil
		}
		m.statusMsg = statusStyle.Render("Action completed, reloading...")
		m.loading = true
		return m, m.loadExtensions()

	case spinner.TickMsg:
		if m.inputStep == inputStepLoadingPackages {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case packagesLoadedMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Failed to load packages: " + msg.err.Error())
			m.inputStep = inputStepNone
			return m, nil
		}
		m.packages = msg.packages
		m.filteredPkgs = msg.packages
		m.inputStep = inputStepPackageList
		m.filterInput.SetValue("")
		m.filterInput.Focus()
		m.updatePackageTableRows()
		return m, textinput.Blink

	case composerRequireDoneMsg:
		if msg.err != nil {
			m.statusMsg = errorStyle.Render("Composer require failed: " + msg.err.Error())
			m.loading = false
			return m, nil
		}
		m.statusMsg = statusStyle.Render("Package installed, refreshing extensions...")
		return m, m.refreshAndReload()

	case tea.PasteMsg:
		switch m.inputStep {
		case inputStepToken:
			var cmd tea.Cmd
			m.tokenInput, cmd = m.tokenInput.Update(msg)
			return m, cmd
		case inputStepPackageList:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.applyPackageFilter()
			return m, cmd
		}

	case tea.KeyPressMsg:
		if m.inputStep == inputStepLoadingPackages {
			if msg.String() == "esc" {
				m.inputStep = inputStepNone
			}
			return m, nil
		}

		if m.inputStep == inputStepToken {
			switch msg.String() {
			case "enter":
				token := strings.TrimSpace(m.tokenInput.Value())
				if token != "" {
					if err := m.savePackagesToken(token); err != nil {
						m.statusMsg = errorStyle.Render(err.Error())
						m.inputStep = inputStepNone
						return m, nil
					}
					m.inputStep = inputStepLoadingPackages
					return m, tea.Batch(m.spinner.Tick, m.loadPackages())
				}
				return m, nil
			case "esc":
				m.inputStep = inputStepNone
				m.tokenInput.SetValue("")
				return m, nil
			}
			var cmd tea.Cmd
			m.tokenInput, cmd = m.tokenInput.Update(msg)
			return m, cmd
		}

		if m.inputStep == inputStepPackageList {
			switch msg.String() {
			case "enter":
				row := m.packageTable.SelectedRow()
				if row != nil {
					m.inputStep = inputStepNone
					m.loading = true
					m.statusMsg = statusStyle.Render("Running composer require " + row[0] + "...")
					return m, m.composerRequire(row[0])
				}
				return m, nil
			case "esc":
				m.inputStep = inputStepNone
				m.filterInput.SetValue("")
				return m, nil
			case "up", "down", "pgup", "pgdown":
				var cmd tea.Cmd
				m.packageTable, cmd = m.packageTable.Update(msg)
				return m, cmd
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				m.applyPackageFilter()
				return m, cmd
			}
		}

		if !m.configured || m.loading {
			return m, nil
		}

		if msg.String() == "r" {
			m.err = nil
			m.statusMsg = statusStyle.Render("Reloading extensions...")
			m.loading = true
			return m, m.loadExtensions()
		}

		if msg.String() == "ctrl+n" {
			// Check if packages.shopware.com token exists in auth.json
			authFile := filepath.Join(m.projectRoot, "auth.json")
			auth, _ := packagist.ReadComposerAuth(authFile)
			if auth.BearerAuth["packages.shopware.com"] == "" {
				m.inputStep = inputStepToken
				m.tokenInput.SetValue("")
				m.tokenInput.Focus()
				return m, textinput.Blink
			}
			m.inputStep = inputStepLoadingPackages
			return m, tea.Batch(m.spinner.Tick, m.loadPackages())
		}

		if m.err != nil {
			return m, nil
		}

		ext := m.selectedExtension()
		if ext == nil {
			break
		}

		switch msg.String() {
		case "a":
			m.statusMsg = statusStyle.Render("Activating " + ext.Name + "...")
			return m, m.extensionAction(ext, "activate")
		case "d":
			m.statusMsg = statusStyle.Render("Deactivating " + ext.Name + "...")
			return m, m.extensionAction(ext, "deactivate")
		case "i":
			m.statusMsg = statusStyle.Render("Installing " + ext.Name + "...")
			return m, m.extensionAction(ext, "install")
		case "u":
			m.statusMsg = statusStyle.Render("Uninstalling " + ext.Name + "...")
			return m, m.extensionAction(ext, "uninstall")
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m ExtensionsModel) renderModal(content string) string {
	modalWidth := m.width * 8 / 10
	modalHeight := m.height * 8 / 10
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalHeight < 10 {
		modalHeight = 10
	}

	modal := overlayStyle.
		Width(modalWidth).
		Height(modalHeight).
		Render(content)

	if m.width > 0 && m.height > 0 {
		modal = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}
	return modal
}

func (m ExtensionsModel) View() string {
	if m.inputStep == inputStepToken {
		var b strings.Builder
		b.WriteString(statusStyle.Render("Download Extension"))
		b.WriteString("\n\n")
		b.WriteString("Enter your packages.shopware.com token:\n\n")
		b.WriteString(m.tokenInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter: save | esc: cancel"))
		return m.renderModal(b.String())
	}

	if m.inputStep == inputStepLoadingPackages {
		var b strings.Builder
		b.WriteString(statusStyle.Render("Download Extension"))
		b.WriteString("\n\n")
		b.WriteString(m.spinner.View() + " Loading available packages...")
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc: cancel"))
		return m.renderModal(b.String())
	}

	if m.inputStep == inputStepPackageList {
		var b strings.Builder
		b.WriteString(statusStyle.Render("Download Extension"))
		b.WriteString("\n\n")
		b.WriteString(m.filterInput.View())
		b.WriteString("\n\n")
		if len(m.filteredPkgs) == 0 {
			b.WriteString(helpStyle.Render("No packages found.") + "\n\n")
		} else {
			b.WriteString(m.packageTable.View())
			b.WriteString("\n")
		}
		b.WriteString(helpStyle.Render("enter: install | ↑/↓: navigate | esc: cancel"))
		return m.renderModal(b.String())
	}

	if !m.configured {
		return "\n" + helpStyle.Render("Admin API not configured. Add admin_api credentials to .shopware-project.yml") + "\n"
	}

	if m.loading {
		var b strings.Builder
		b.WriteString("\n" + helpStyle.Render("Loading extensions...") + "\n")
		if m.statusMsg != "" {
			b.WriteString(m.statusMsg + "\n")
		}
		return b.String()
	}

	if m.err != nil {
		return "\n" + errorStyle.Render("Failed to load extensions: "+m.err.Error()) + "\n\n" + helpStyle.Render("r: retry") + "\n"
	}

	var b strings.Builder
	b.WriteString("\n")

	if len(m.extensions) == 0 {
		b.WriteString(helpStyle.Render("No extensions installed.") + "\n")
	} else {
		b.WriteString(m.table.View())
		b.WriteString("\n")
	}

	if m.statusMsg != "" {
		b.WriteString(m.statusMsg + "\n")
	}

	b.WriteString(helpStyle.Render("a: activate | d: deactivate | i: install | u: uninstall | r: reload | ctrl+n: download extension | ↑/↓: navigate"))

	return b.String()
}

func (m *ExtensionsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.table.SetWidth(width)
	// Reserve 3 lines for status and help
	m.table.SetHeight(height - 4)
	// Modal is 80% of terminal, subtract border/padding and title/filter/help lines
	modalInnerWidth := width*8/10 - 8
	modalInnerHeight := height*8/10 - 10
	if modalInnerWidth < 40 {
		modalInnerWidth = 40
	}
	if modalInnerHeight < 5 {
		modalInnerHeight = 5
	}
	m.packageTable.SetWidth(modalInnerWidth)
	m.packageTable.SetHeight(modalInnerHeight)
}

func (m *ExtensionsModel) updateTableRows() {
	rows := make([]table.Row, 0, len(m.extensions))
	for _, ext := range m.extensions {
		rows = append(rows, table.Row{
			ext.Name,
			ext.Label,
			ext.Version,
			ext.Type,
			ext.Status(),
		})
	}
	m.table.SetRows(rows)
}

func (m *ExtensionsModel) selectedExtension() *adminSdk.ExtensionDetail {
	row := m.table.SelectedRow()
	if row == nil {
		return nil
	}
	name := row[0]
	return m.extensions.GetByName(name)
}

func (m *ExtensionsModel) loadExtensions() tea.Cmd {
	config := m.config
	return func() tea.Msg {
		ctx := context.Background()
		client, err := shop.NewShopClient(ctx, config)
		if err != nil {
			return extensionsLoadedMsg{err: fmt.Errorf("cannot create API client: %w", err)}
		}

		apiCtx := adminSdk.NewApiContext(ctx)
		extensions, _, err := client.ExtensionManager.ListAvailableExtensions(apiCtx)
		if err != nil {
			return extensionsLoadedMsg{err: fmt.Errorf("cannot list extensions: %w", err)}
		}

		return extensionsLoadedMsg{extensions: extensions}
	}
}

func (m *ExtensionsModel) loadPackages() tea.Cmd {
	projectRoot := m.projectRoot
	return func() tea.Msg {
		authFile := filepath.Join(projectRoot, "auth.json")
		auth, err := packagist.ReadComposerAuth(authFile)
		if err != nil {
			return packagesLoadedMsg{err: err}
		}

		token := auth.BearerAuth["packages.shopware.com"]
		if token == "" {
			return packagesLoadedMsg{err: fmt.Errorf("no packages.shopware.com token found")}
		}

		resp, err := packagist.GetPackages(context.Background(), token)
		if err != nil {
			return packagesLoadedMsg{err: err}
		}

		var entries []packageEntry
		for name, versions := range resp.Packages {
			var desc string
			for _, v := range versions {
				if v.Description != "" {
					desc = v.Description
					break
				}
			}
			entries = append(entries, packageEntry{name: name, description: desc})
		}

		// Sort alphabetically
		slices.SortFunc(entries, func(a, b packageEntry) int {
			return strings.Compare(a.name, b.name)
		})

		return packagesLoadedMsg{packages: entries}
	}
}

func (m *ExtensionsModel) updatePackageTableRows() {
	rows := make([]table.Row, 0, len(m.filteredPkgs))
	for _, pkg := range m.filteredPkgs {
		desc := pkg.description
		if len(desc) > 47 {
			desc = desc[:47] + "..."
		}
		rows = append(rows, table.Row{pkg.name, desc})
	}
	m.packageTable.SetRows(rows)
}

func (m *ExtensionsModel) applyPackageFilter() {
	filter := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	if filter == "" {
		m.filteredPkgs = m.packages
	} else {
		m.filteredPkgs = nil
		for _, pkg := range m.packages {
			if strings.Contains(strings.ToLower(pkg.name), filter) || strings.Contains(strings.ToLower(pkg.description), filter) {
				m.filteredPkgs = append(m.filteredPkgs, pkg)
			}
		}
	}
	m.updatePackageTableRows()
}

func (m *ExtensionsModel) savePackagesToken(token string) error {
	// Save token to auth.json
	authFile := filepath.Join(m.projectRoot, "auth.json")
	auth, _ := packagist.ReadComposerAuth(authFile)
	auth.BearerAuth["packages.shopware.com"] = token
	if err := auth.Save(); err != nil {
		return fmt.Errorf("failed to save auth.json: %w", err)
	}

	// Add repository to composer.json if missing
	composerFile := filepath.Join(m.projectRoot, "composer.json")
	composerJson, err := packagist.ReadComposerJson(composerFile)
	if err != nil {
		return fmt.Errorf("failed to read composer.json: %w", err)
	}

	if !composerJson.Repositories.HasRepository("https://packages.shopware.com") {
		composerJson.Repositories = append(composerJson.Repositories, packagist.ComposerJsonRepository{
			Type: "composer",
			URL:  "https://packages.shopware.com",
		})
		if err := composerJson.Save(); err != nil {
			return fmt.Errorf("failed to save composer.json: %w", err)
		}
	}

	return nil
}

func (m *ExtensionsModel) composerRequire(pkg string) tea.Cmd {
	e := m.executor
	projectRoot := m.projectRoot
	return func() tea.Msg {
		cmd := e.ComposerCommand(context.Background(), "require", pkg)
		cmd.Dir = projectRoot

		output, err := cmd.CombinedOutput()
		if err != nil {
			// Include last few lines of output for context
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			if len(lines) > 5 {
				lines = lines[len(lines)-5:]
			}
			return composerRequireDoneMsg{err: fmt.Errorf("%w\n%s", err, strings.Join(lines, "\n"))}
		}

		return composerRequireDoneMsg{}
	}
}

func (m *ExtensionsModel) refreshAndReload() tea.Cmd {
	config := m.config
	return func() tea.Msg {
		ctx := context.Background()
		client, err := shop.NewShopClient(ctx, config)
		if err != nil {
			// Still reload even if refresh fails
			return extensionsLoadedMsg{err: fmt.Errorf("cannot create API client for refresh: %w", err)}
		}

		apiCtx := adminSdk.NewApiContext(ctx)
		_, _ = client.ExtensionManager.Refresh(apiCtx)

		extensions, _, err := client.ExtensionManager.ListAvailableExtensions(apiCtx)
		if err != nil {
			return extensionsLoadedMsg{err: fmt.Errorf("cannot list extensions: %w", err)}
		}

		return extensionsLoadedMsg{extensions: extensions}
	}
}

func (m *ExtensionsModel) extensionAction(ext *adminSdk.ExtensionDetail, action string) tea.Cmd {
	config := m.config
	extType := ext.Type
	extName := ext.Name
	return func() tea.Msg {
		ctx := context.Background()
		client, err := shop.NewShopClient(ctx, config)
		if err != nil {
			return extensionActionDoneMsg{err: err}
		}

		apiCtx := adminSdk.NewApiContext(ctx)

		switch action {
		case "activate":
			_, err = client.ExtensionManager.ActivateExtension(apiCtx, extType, extName)
		case "deactivate":
			_, err = client.ExtensionManager.DeactivateExtension(apiCtx, extType, extName)
		case "install":
			_, err = client.ExtensionManager.InstallExtension(apiCtx, extType, extName)
		case "uninstall":
			_, err = client.ExtensionManager.UninstallExtension(apiCtx, extType, extName)
		}

		return extensionActionDoneMsg{err: err}
	}
}
