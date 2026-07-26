package pluginmigrate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shyim/go-composer"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

// StepID identifies one step of the migration execution.
type StepID int

const (
	StepWriteConfig StepID = iota
	StepComposerRequire
	StepRemoveDirs
	StepPluginRefresh
	// StepFinished is the synthetic final event: StateOK when the migration
	// succeeded, StateFail (with Err) when it was aborted and rolled back.
	StepFinished
)

// RunSteps lists the visible steps in execution order.
var RunSteps = []StepID{
	StepWriteConfig,
	StepComposerRequire,
	StepRemoveDirs,
	StepPluginRefresh,
}

// Label returns the checklist text for a step.
func (s StepID) Label() string {
	switch s {
	case StepWriteConfig:
		return "Update composer.json and auth.json"
	case StepComposerRequire:
		return "composer require"
	case StepRemoveDirs:
		return "Remove migrated plugin directories"
	case StepPluginRefresh:
		return "bin/console plugin:refresh"
	case StepFinished:
		return "Finished"
	}
	return "Unknown step"
}

// StepState is the lifecycle state of a step.
type StepState int

const (
	StatePending StepState = iota
	StateRunning
	StateOK
	StateWarn
	StateFail
)

// StepEvent is emitted while the migration executes. Line events carry
// streamed subprocess output (State is StateRunning); state events mark step
// transitions.
type StepEvent struct {
	Step  StepID
	State StepState
	Line  string
	Err   error
}

// backedUpFiles are snapshotted before any change and restored when the
// migration fails before directories were removed.
var backedUpFiles = []string{"composer.json", "composer.lock", "auth.json"}

// Run executes the migration asynchronously and streams progress events. The
// channel closes after a final StepFinished event. Failures before the
// destructive directory removal roll composer.json, composer.lock, and
// auth.json back.
func (m *PluginMigrator) Run(ctx context.Context, plan Plan, token string) <-chan StepEvent {
	events := make(chan StepEvent, 64)

	go func() {
		defer close(events)

		if err := m.backup(); err != nil {
			events <- StepEvent{Step: StepFinished, State: StateFail, Err: fmt.Errorf("backup project files: %w", err)}
			return
		}

		if err := m.runSteps(ctx, plan, token, func(ev StepEvent) { events <- ev }); err != nil {
			m.restore(func(ev StepEvent) { events <- ev })
			events <- StepEvent{Step: StepFinished, State: StateFail, Err: err}
			return
		}

		events <- StepEvent{Step: StepFinished, State: StateOK}
	}()

	return events
}

func (m *PluginMigrator) runSteps(ctx context.Context, plan Plan, token string, emit func(StepEvent)) error {
	steps := []struct {
		id       StepID
		nonFatal bool // a failure warns instead of aborting the migration
		run      func(context.Context, func(string)) error
	}{
		{StepWriteConfig, false, func(_ context.Context, line func(string)) error {
			return m.writeConfig(plan, token, line)
		}},
		{StepComposerRequire, false, func(ctx context.Context, line func(string)) error {
			args := append([]string{"require", "--no-interaction", "--no-progress"}, plan.RequireArgs()...)
			return streamProcess(ctx, m.executor.ComposerCommand(ctx, args...), line)
		}},
		// Directory removal cannot be rolled back, so problems here only warn
		// and the remaining directories are reported for manual cleanup.
		{StepRemoveDirs, true, func(_ context.Context, line func(string)) error {
			for _, dir := range plan.RemoveDirs() {
				if err := os.RemoveAll(dir); err != nil {
					return fmt.Errorf("remove %s manually: %w", dir, err)
				}
				line("Removed " + dir)
			}
			return nil
		}},
		{StepPluginRefresh, true, func(ctx context.Context, line func(string)) error {
			return streamProcess(ctx, m.executor.ConsoleCommand(ctx, "plugin:refresh"), line)
		}},
	}

	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}

		emit(StepEvent{Step: step.id, State: StateRunning})
		line := func(l string) {
			emit(StepEvent{Step: step.id, State: StateRunning, Line: l})
		}

		err := step.run(ctx, line)
		switch {
		case err == nil:
			emit(StepEvent{Step: step.id, State: StateOK})
		case step.nonFatal && ctx.Err() == nil:
			emit(StepEvent{Step: step.id, State: StateWarn, Err: err})
		default:
			emit(StepEvent{Step: step.id, State: StateFail, Err: err})
			return fmt.Errorf("%s: %w", step.id.Label(), err)
		}
	}

	return nil
}

// writeConfig registers the Store and path repositories in composer.json and
// stores the token in auth.json.
func (m *PluginMigrator) writeConfig(plan Plan, token string, line func(string)) error {
	composerJSON, err := composer.ReadJson(filepath.Join(m.projectRoot, "composer.json"))
	if err != nil {
		return err
	}

	if plan.AddStoreRepository && !composerJSON.Repositories.HasRepository(StoreRepositoryURL) {
		composerJSON.Repositories = append(composerJSON.Repositories, composer.Repository{
			Type: "composer",
			URL:  StoreRepositoryURL,
		})
		line("Added Composer repository " + StoreRepositoryURL)
	}

	for _, relPath := range plan.PathRepositories() {
		if composerJSON.Repositories.HasRepository(relPath) {
			continue
		}
		composerJSON.Repositories = append(composerJSON.Repositories, composer.Repository{
			Type: "path",
			URL:  relPath,
		})
		line("Added path repository " + relPath)
	}

	if err := composerJSON.Save(); err != nil {
		return err
	}

	if token != "" {
		auth, err := shop.ReadComposerAuth(filepath.Join(m.projectRoot, "auth.json"))
		if err != nil {
			return err
		}
		auth.BearerAuth["packages.shopware.com"] = token
		if err := auth.Save(); err != nil {
			return err
		}
		line("Stored the Packagist token in auth.json")
	}

	return nil
}

// streamProcess runs a process and forwards its combined output line by line.
// On cancellation it also runs the process's executor-specific Stop so
// Docker-side processes terminate before any rollback.
func streamProcess(ctx context.Context, p *executor.Process, line func(string)) error {
	w := tui.NewLineWriter(line)
	err := p.RunWithOutput(w)
	w.Flush()

	if ctx.Err() != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}
	return err
}

func (m *PluginMigrator) backupDir() string {
	return filepath.Join(m.projectRoot, ".shopware-cli", "plugin-migrate-backup")
}

func (m *PluginMigrator) backup() error {
	if err := os.MkdirAll(m.backupDir(), 0o755); err != nil {
		return err
	}

	for _, name := range backedUpFiles {
		content, err := os.ReadFile(filepath.Join(m.projectRoot, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.WriteFile(filepath.Join(m.backupDir(), name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// restore puts the backed-up files back after a failed run.
func (m *PluginMigrator) restore(emit func(StepEvent)) {
	for _, name := range backedUpFiles {
		content, err := os.ReadFile(filepath.Join(m.backupDir(), name))
		if err != nil {
			continue
		}

		current, readErr := os.ReadFile(filepath.Join(m.projectRoot, name))
		if readErr == nil && bytes.Equal(current, content) {
			continue
		}

		if err := os.WriteFile(filepath.Join(m.projectRoot, name), content, 0o644); err != nil {
			emit(StepEvent{Step: StepFinished, State: StateRunning, Line: "Could not restore " + name + ": " + err.Error()})
			continue
		}
		emit(StepEvent{Step: StepFinished, State: StateRunning, Line: "Restored " + name + " from backup."})
	}
}
