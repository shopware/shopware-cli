package project

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/environment"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
)

const (
	envActionAdd    = "add"
	envActionEdit   = "edit"
	envActionTest   = "test"
	envActionRemove = "remove"
	envActionExit   = "exit"
)

// errEnvBack reports that the user left a prompt with ctrl+c. The menu takes
// it as "back", a subcommand as a cancelled run.
var errEnvBack = errors.New("prompt aborted")

var errEnvCancelled = tracking.NewCancelled("environment management cancelled")

// envWizard guides through the environment workflows, one environment at a
// time. The project config is read again before every step, because each step
// may have rewritten it.
type envWizard struct {
	cmd        *cobra.Command
	configPath string
	out        io.Writer

	// nextAction and nextName preselect the menu entry and the environment
	// when a step suggests its follow-up, e.g. testing after saving.
	nextAction string
	nextName   string
	// inMenu is set while the menu runs. A passed test then waits for the
	// user before the menu is drawn again, so the result is not overlooked.
	inMenu bool
	// notReady holds the environments whose last test in this run failed.
	notReady map[string]bool
}

func newEnvWizard(cmd *cobra.Command, configPath string) *envWizard {
	return &envWizard{cmd: cmd, configPath: configPath, out: cmd.OutOrStdout(), notReady: map[string]bool{}}
}

// direct maps the outcome of a single workflow to the result of a subcommand.
func (w *envWizard) direct(err error) error {
	if errors.Is(err, errEnvBack) {
		return errEnvCancelled
	}
	if err != nil {
		return err
	}
	if len(w.notReady) > 0 {
		return errEnvNotReady
	}
	return nil
}

func (w *envWizard) run(groups ...*huh.Group) error {
	ctx := w.cmd.Context()
	err := huh.NewForm(groups...).
		WithTheme(tui.ShopwareTheme()).
		WithInput(w.cmd.InOrStdin()).
		WithOutput(w.cmd.ErrOrStderr()).
		RunWithContext(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return errEnvBack
	}
	return err
}

// displayPath shortens the config path to one relative to the working
// directory when the file lies below it.
func (w *envWizard) displayPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return w.configPath
	}
	if rel, err := filepath.Rel(cwd, w.configPath); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	return w.configPath
}

func (w *envWizard) println(lines ...string) {
	for _, line := range lines {
		_, _ = fmt.Fprintln(w.out, line)
	}
}

func (w *envWizard) overviews() ([]environment.Overview, error) {
	project, err := readEnvProject(w.cmd.Context(), w.configPath)
	if err != nil {
		return nil, err
	}
	return environment.Overviews(project.cfg, project.store, time.Now()), nil
}

func (w *envWizard) menu() error {
	w.inMenu = true
	for {
		overviews, err := w.overviews()
		if err != nil {
			return err
		}

		w.println(tui.BoldText.Render("Environments"), envSummary(overviews), "")

		options := []huh.Option[string]{menuOption("Add environment", "Enter a name and configure the connection.", envActionAdd)}
		action := envActionAdd
		if len(overviews) > 0 {
			action = envActionTest
			options = append(options,
				menuOption("Edit environment", "Change the environment configuration.", envActionEdit),
				menuOption("Test environment", "Check connection, configuration, and capabilities.", envActionTest),
				menuOption("Remove environment", "Remove this environment from the project.", envActionRemove),
			)
		}
		options = append(options, menuOption("Exit", "", envActionExit))

		name := w.nextName
		if w.nextAction != "" {
			action = w.nextAction
		}
		w.nextAction, w.nextName = "", ""

		err = w.run(huh.NewGroup(huh.NewSelect[string]().Title("What do you want to do?").Options(options...).Value(&action)))
		if errors.Is(err, errEnvBack) {
			return nil
		}
		if err != nil {
			return err
		}

		switch action {
		case envActionAdd:
			err = w.add(environment.Settings{}, false, false)
		case envActionEdit:
			err = w.edit("", "")
		case envActionTest:
			err = w.test(name)
		case envActionRemove:
			err = w.remove("")
		case envActionExit:
			return nil
		}
		if err != nil && !errors.Is(err, errEnvBack) {
			return err
		}
		w.println()
	}
}

func menuOption(label, description, value string) huh.Option[string] {
	if description == "" {
		return huh.NewOption(label, value)
	}
	return huh.NewOption(fmt.Sprintf("%-20s %s", label, tui.DimText.Render(description)), value)
}

// envSummary renders "24 environments · 21 READY · 3 NOT READY".
func envSummary(overviews []environment.Overview) string {
	if len(overviews) == 0 {
		return "No environments yet."
	}

	ready := 0
	for _, overview := range overviews {
		if overview.Ready {
			ready++
		}
	}

	noun := "environments"
	if len(overviews) == 1 {
		noun = "environment"
	}
	parts := []string{fmt.Sprintf("%d %s", len(overviews), noun)}
	if ready > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", ready, tui.GreenText.Render("READY")))
	}
	if notReady := len(overviews) - ready; notReady > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", notReady, tui.YellowText.Render("NOT READY")))
	}
	return strings.Join(parts, " · ")
}

// pick asks for the environment an action applies to. A project with a single
// environment needs no question.
func (w *envWizard) pick(action, preselected string) (string, error) {
	overviews, err := w.overviews()
	if err != nil {
		return "", err
	}
	switch len(overviews) {
	case 0:
		return "", errors.New("cannot pick an environment: the project config has none, add one first")
	case 1:
		return overviews[0].Name, nil
	}

	nameWidth := 0
	for _, overview := range overviews {
		nameWidth = max(nameWidth, len(overview.Name))
	}

	selected := overviews[0].Name
	options := make([]huh.Option[string], 0, len(overviews))
	for _, overview := range overviews {
		status := tui.YellowText.Render("NOT READY")
		if overview.Ready {
			status = tui.GreenText.Render("READY") + "    "
		}
		label := fmt.Sprintf("%-*s  %-6s  %s  %s", nameWidth, overview.Name, overview.Type, status, tui.DimText.Render(overview.LastTest))
		options = append(options, huh.NewOption(label, overview.Name))
		if overview.Name == preselected {
			selected = preselected
		}
	}

	err = w.run(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Pick the environment to " + action).
			Description(fmt.Sprintf("%d environments · most recently tested first", len(overviews))).
			Options(options...).
			Height(min(len(options), 8) + 3).
			Value(&selected),
	))
	if err != nil {
		return "", err
	}
	return selected, nil
}

// add asks for every setting missing from input, saves the environment and
// offers to test it. testDecided skips that question.
func (w *envWizard) add(input environment.Settings, testDecided, test bool) error {
	project, err := readEnvProject(w.cmd.Context(), w.configPath)
	if err != nil {
		return err
	}

	askOptional := !input.HasOptional()
	setOptional := false
	port := ""
	var groups []*huh.Group

	if input.Name == "" {
		groups = append(groups, huh.NewGroup(huh.NewInput().
			Title("Environment name").
			Description("Letters, numbers and dashes. Has to be unique.").
			Validate(project.validateNewName("")).
			Value(&input.Name)))
	}
	if input.URL == "" {
		groups = append(groups, huh.NewGroup(huh.NewInput().
			Title("URL").
			Description("Shop URL of the environment, e.g. https://preprod.example.com").
			Validate(tui.ValidateHTTPURL).
			Value(&input.URL)))
	}
	if input.Host == "" {
		groups = append(groups, huh.NewGroup(huh.NewInput().
			Title("Host").
			Description("SSH host name. Enter alone takes the host of the URL.").
			PlaceholderFunc(func() string { return environment.HostOfURL(input.URL) }, &input.URL).
			Validate(func(value string) error {
				if value == "" && environment.HostOfURL(input.URL) != "" {
					return nil
				}
				return environment.ValidateHost(value)
			}).
			Value(&input.Host)))
	}
	if input.Directory == "" {
		groups = append(groups, huh.NewGroup(huh.NewInput().
			Title("Directory").
			Description("Absolute path of the Shopware project on the remote host. For deployments, the path of the active release symlink.").
			Validate(environment.ValidateDirectory).
			Value(&input.Directory)))
	}
	if askOptional {
		groups = append(groups,
			huh.NewGroup(huh.NewSelect[bool]().
				Title("Optional settings").
				Description("Without them, ssh connects as a plain \"ssh <host>\" does:\n"+
					"  User           the user of your ~/.ssh/config, else your local user name\n"+
					"  Port           the port of your ~/.ssh/config, else 22\n"+
					"  Identity file  the key of your ~/.ssh/config, else the ssh agent and your default keys\n"+
					"  PHP binary     \"php\" on the PATH of the remote host").
				Options(huh.NewOption("Use defaults", false), huh.NewOption("Set them now", true)).
				Value(&setOptional)),
			huh.NewGroup(
				huh.NewInput().Title("User").Placeholder("from ~/.ssh/config, else your local user name").Validate(tui.ValidateSingleLine).Value(&input.User),
				huh.NewInput().Title("Port").Placeholder("from ~/.ssh/config, else 22").Validate(tui.OptionalValidator(tui.ValidatePort)).Value(&port),
				huh.NewInput().Title("Identity file").Placeholder("from ~/.ssh/config, else the ssh agent and your default keys").Validate(tui.ValidateSingleLine).Value(&input.IdentityFile),
				huh.NewInput().Title("PHP binary").Placeholder("php on the remote PATH").Validate(tui.ValidateSingleLine).Value(&input.PHPBinary),
			).WithHideFunc(func() bool { return !setOptional }),
		)
	}

	if len(groups) > 0 {
		if err := w.run(groups...); err != nil {
			return err
		}
	}
	if input.Host == "" {
		input.Host = environment.HostOfURL(input.URL)
	}
	if err := environment.ValidateHost(input.Host); err != nil {
		return fmt.Errorf("cannot add environment %s: host: %w", input.Name, err)
	}
	if askOptional {
		if setOptional {
			input.Port, _ = strconv.Atoi(port)
		} else {
			// Values typed before going back to "Use defaults" do not count.
			input.User, input.Port, input.IdentityFile, input.PHPBinary = "", 0, "", ""
		}
	}

	if err := shop.AddEnvironment(w.configPath, input.Name, input.Config()); err != nil {
		return err
	}

	w.println(
		tui.SuccessLine(fmt.Sprintf("Saved %s to %s", input.Name, w.displayPath())),
		fmt.Sprintf("  %-10s %s", "Target", environment.Target(input.Config())),
		fmt.Sprintf("  %-10s %s  %s", "Status", tui.YellowText.Render("NOT READY"), tui.DimText.Render("not tested yet")),
		"",
	)

	return w.offerTest(input.Name, "not tested yet", testDecided, test)
}

// validateNewName accepts a valid environment name that is not taken yet.
// current is the name an edited environment already has.
func (p *envProject) validateNewName(current string) func(string) error {
	return func(value string) error {
		if current != "" && value == current {
			return nil
		}
		if err := shop.ValidateEnvironmentName(value); err != nil {
			return err
		}
		if _, taken := p.cfg.Environments[value]; taken {
			return fmt.Errorf("%q already exists, pick another name", value)
		}
		return nil
	}
}

// offerTest asks whether to test a saved environment now. Declining suggests
// the test as the next menu action.
func (w *envWizard) offerTest(name, pending string, decided, test bool) error {
	if !decided {
		test = true
		err := w.run(huh.NewGroup(huh.NewSelect[bool]().
			Title("Test it now?").
			Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).
			Value(&test)))
		if errors.Is(err, errEnvBack) {
			test = false
		} else if err != nil {
			return err
		}
	}

	if test {
		return w.testName(name)
	}

	w.println(fmt.Sprintf("%s %s is saved, but %s: %s.", tui.CheckWarn(), name, tui.YellowText.Render("NOT READY"), pending))
	w.nextAction, w.nextName = envActionTest, name
	return nil
}

// envSettingField is one question of the edit form.
type envSettingField struct {
	setting  environment.Setting
	label    string
	value    *string
	fallback string
	validate func(string) error
}

// edit asks which settings of an environment to change, then for each new
// value. fix preselects the setting a failed test pointed at.
func (w *envWizard) edit(name string, fix environment.Setting) error {
	if name == "" {
		var err error
		if name, err = w.pick("edit", ""); err != nil {
			return err
		}
	}

	project, err := readEnvProject(w.cmd.Context(), w.configPath)
	if err != nil {
		return err
	}
	env, err := project.cfg.ResolveEnvironment(name)
	if err != nil {
		return err
	}

	current, editable := environment.SettingsOf(name, env)
	edited := current
	port := portText(current.Port)

	fields := []envSettingField{
		{environment.SettingName, "Name", &edited.Name, "", project.validateNewName(name)},
		{environment.SettingURL, "URL", &edited.URL, "not set", tui.OptionalValidator(tui.ValidateHTTPURL)},
		{environment.SettingHost, "Host", &edited.Host, "", environment.ValidateHost},
		{environment.SettingDirectory, "Directory", &edited.Directory, "", environment.ValidateDirectory},
		{environment.SettingUser, "User", &edited.User, "ssh default", tui.ValidateSingleLine},
		{environment.SettingPort, "Port", &port, "ssh default, 22", tui.OptionalValidator(tui.ValidatePort)},
		{environment.SettingIdentityFile, "Identity file", &edited.IdentityFile, "ssh default", tui.ValidateSingleLine},
		{environment.SettingPHPBinary, "PHP binary", &edited.PHPBinary, "php", tui.ValidateSingleLine},
	}
	fields = slices.DeleteFunc(fields, func(field envSettingField) bool { return !slices.Contains(editable, field.setting) })

	var selected []environment.Setting
	options := make([]huh.Option[environment.Setting], 0, len(fields))
	questions := make([]*huh.Group, 0, len(fields))
	for _, field := range fields {
		label := fmt.Sprintf("%-14s %s", field.label, tui.DimText.Render(cmp.Or(*field.value, field.fallback)))
		options = append(options, huh.NewOption(label, field.setting).Selected(field.setting == fix))
		if field.setting == fix {
			selected = append(selected, field.setting)
		}

		description := "Enter alone keeps it."
		switch {
		case field.setting == environment.SettingURL:
			description = "Enter alone keeps it. Empty removes it."
		case field.fallback != "":
			description = "Enter alone keeps it. Empty uses the default."
		}
		questions = append(questions, huh.NewGroup(huh.NewInput().
			Title(field.label).
			Description(description).
			Placeholder(field.fallback).
			Validate(field.validate).
			Value(field.value),
		).WithHideFunc(func() bool { return !slices.Contains(selected, field.setting) }))
	}

	groups := append([]*huh.Group{huh.NewGroup(huh.NewMultiSelect[environment.Setting]().
		Title(fmt.Sprintf("What do you want to change on %s?", name)).
		Description("One or more. Each one is asked next.").
		Options(options...).
		Validate(func(values []environment.Setting) error {
			if len(values) == 0 {
				return errors.New("tick at least one setting with space")
			}
			return nil
		}).
		Value(&selected))}, questions...)
	if err := w.run(groups...); err != nil {
		return err
	}
	edited.Port, _ = strconv.Atoi(port)

	change, changed := envSettingsChange(current, edited, selected)
	if change.Empty() {
		w.println(fmt.Sprintf("Nothing changed on %s.", name))
		return nil
	}

	name, err = w.save(project, name, change)
	if err != nil {
		return err
	}
	w.println(changed...)
	w.println(fmt.Sprintf("  %-14s %s  %s", "Status", tui.YellowText.Render("NOT READY"), tui.DimText.Render("changed, not tested yet")), "")

	return w.offerTest(name, "changed, not tested yet", false, false)
}

// save writes a change and moves the remembered status along with a rename.
// It returns the name the environment has afterwards.
func (w *envWizard) save(project *envProject, name string, change shop.EnvironmentChange) (string, error) {
	if err := shop.UpdateEnvironment(w.configPath, name, change); err != nil {
		return name, err
	}
	if change.Name != nil && *change.Name != name {
		delete(w.notReady, name)
		renamed := *change.Name
		w.println(tui.SuccessLine("Saved " + renamed))
		return renamed, project.store.Rename(name, renamed)
	}
	w.println(tui.SuccessLine("Saved " + name))
	return name, nil
}

// envSettingsChange lists the selected settings that differ, so untouched
// keys of the config file stay exactly as they are. Answers of a setting that
// was unticked again do not count.
func envSettingsChange(before, after environment.Settings, selected []environment.Setting) (shop.EnvironmentChange, []string) {
	var change shop.EnvironmentChange
	var lines []string

	text := func(setting environment.Setting, label, old string, value *string, target **string) {
		if slices.Contains(selected, setting) && old != *value {
			*target = value
			lines = append(lines, fmt.Sprintf("  %-14s %s", label, cmp.Or(*value, "default")))
		}
	}
	text(environment.SettingName, "Name", before.Name, &after.Name, &change.Name)
	text(environment.SettingURL, "URL", before.URL, &after.URL, &change.URL)
	text(environment.SettingHost, "Host", before.Host, &after.Host, &change.Host)
	text(environment.SettingDirectory, "Directory", before.Directory, &after.Directory, &change.Directory)
	text(environment.SettingUser, "User", before.User, &after.User, &change.User)
	if slices.Contains(selected, environment.SettingPort) && before.Port != after.Port {
		change.Port = &after.Port
		lines = append(lines, fmt.Sprintf("  %-14s %s", "Port", cmp.Or(portText(after.Port), "default")))
	}
	text(environment.SettingIdentityFile, "Identity file", before.IdentityFile, &after.IdentityFile, &change.IdentityFile)
	text(environment.SettingPHPBinary, "PHP binary", before.PHPBinary, &after.PHPBinary, &change.PHPBinary)

	return change, lines
}

// test asks which environment to test; a name suggested by an earlier step
// comes preselected.
func (w *envWizard) test(suggested string) error {
	name, err := w.pick("test", suggested)
	if err != nil {
		return err
	}
	return w.testName(name)
}

// testName tests one environment and, when it is not ready, offers to edit
// it or to test it again.
func (w *envWizard) testName(name string) error {
	for {
		results, err := runEnvironmentTests(w.cmd, w.configPath, []string{name})
		if err != nil {
			return err
		}
		result := results[0]
		const again, edit, closeResult = "again", "edit", "close"

		if result.Ready {
			delete(w.notReady, name)
			if !w.inMenu {
				return nil
			}

			status := "READY"
			if result.Limited {
				status = "READY, limited"
			}
			next := closeResult
			err = w.run(huh.NewGroup(huh.NewSelect[string]().
				Title(fmt.Sprintf("%s is %s", name, status)).
				Options(huh.NewOption("Back to menu", closeResult), huh.NewOption("Test again", again)).
				Value(&next)))
			if errors.Is(err, errEnvBack) || (err == nil && next == closeResult) {
				return nil
			}
			if err != nil {
				return err
			}
			continue
		}
		w.notReady[name] = true

		next := edit
		err = w.run(huh.NewGroup(huh.NewSelect[string]().
			Title(name+" is NOT READY. What do you want to do?").
			Options(
				huh.NewOption("Edit "+name, edit),
				huh.NewOption("Test again", again),
				huh.NewOption("Close", closeResult),
			).
			Value(&next)))
		if errors.Is(err, errEnvBack) {
			return nil
		}
		if err != nil {
			return err
		}

		switch next {
		case edit:
			if err := w.edit(name, result.Fix); err != nil && !errors.Is(err, errEnvBack) {
				return err
			}
			return nil
		case closeResult:
			return nil
		}
	}
}

// remove confirms and removes one environment. Remote targets are confirmed
// by typing the name, because commands and deployments that point at them
// stop working.
func (w *envWizard) remove(name string) error {
	if name == "" {
		var err error
		if name, err = w.pick("remove", ""); err != nil {
			return err
		}
	}

	project, err := readEnvProject(w.cmd.Context(), w.configPath)
	if err != nil {
		return err
	}
	env, err := project.cfg.ResolveEnvironment(name)
	if err != nil {
		return err
	}

	remote := env.Type == executor.TypeSSH
	kept := "The shop and its files are not deleted."
	if remote {
		kept = "The remote shop and its files are not deleted."
	}
	w.println(
		fmt.Sprintf("%s Remove %s from %s", tui.CheckWarn(), tui.BoldText.Render(name), w.displayPath()),
		"  "+tui.YellowText.Render(kept),
		"",
		fmt.Sprintf("  %-10s %s", "Target", environment.Target(env)),
	)
	if env.SSH != nil && env.SSH.Directory != "" {
		w.println(fmt.Sprintf("  %-10s %s", "Directory", env.SSH.Directory))
	}
	w.println("", "  These stop working for "+name+":", "  · commands run with -e "+name)
	if name == "local" {
		w.println("  · commands run without -e, which use local")
	}
	if remote {
		w.println("  · deployments to " + name)
	}
	w.println()

	if remote {
		typed := ""
		err = w.run(huh.NewGroup(huh.NewInput().
			Title(fmt.Sprintf("Type %q to confirm", name)).
			Description("Enter works once the name matches.").
			Validate(func(value string) error {
				if value != name {
					return fmt.Errorf("type %q to remove the environment, or press ctrl+c to keep it", name)
				}
				return nil
			}).
			Value(&typed)))
	} else {
		confirmed := false
		err = w.run(huh.NewGroup(huh.NewSelect[bool]().
			Title(fmt.Sprintf("Remove %s?", name)).
			Options(huh.NewOption("Keep", false), huh.NewOption("Remove", true)).
			Value(&confirmed)))
		if err == nil && !confirmed {
			err = errEnvBack
		}
	}
	if errors.Is(err, errEnvBack) {
		w.println(fmt.Sprintf("Kept %s.", name))
		return nil
	}
	if err != nil {
		return err
	}

	if err := removeEnvironment(project, name); err != nil {
		return err
	}
	delete(w.notReady, name)
	w.println(tui.SuccessLine("Removed "+name), "  "+tui.DimText.Render(kept))
	return nil
}

func portText(port int) string {
	if port == 0 {
		return ""
	}
	return strconv.Itoa(port)
}
