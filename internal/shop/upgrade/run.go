package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shopware/shopware-cli/internal/executor"
)

// StepID identifies one step of the upgrade execution (panel 5's checklist).
type StepID int

const (
	StepRewriteComposer StepID = iota
	StepComposerUpdate
	StepRecipesInstall
	StepDeploymentHelper
	StepWriteReport
	// StepFinished is the synthetic final event: StateOK when the upgrade
	// succeeded, StateFail (with Err) when it was aborted and rolled back.
	StepFinished
)

// RunSteps lists the visible steps in execution order.
var RunSteps = []StepID{
	StepRewriteComposer,
	StepComposerUpdate,
	StepRecipesInstall,
	StepDeploymentHelper,
	StepWriteReport,
}

// Label returns the checklist text for a step.
func (s StepID) Label() string {
	switch s {
	case StepRewriteComposer:
		return "Rewrite composer.json"
	case StepComposerUpdate:
		return "composer update --with-all-dependencies"
	case StepRecipesInstall:
		return "composer recipes:install --force --reset"
	case StepDeploymentHelper:
		return "vendor/bin/shopware-deployment-helper run"
	case StepWriteReport:
		return "Write upgrade report"
	case StepFinished:
		return "Finished"
	}
	return "Unknown step"
}

// StepEvent is emitted by the Runner while the upgrade executes. Line events
// carry streamed subprocess output (State is StateRunning); state events mark
// step transitions.
type StepEvent struct {
	Step  StepID
	State CheckState
	Line  string
	Err   error
}

// RunOptions configure one upgrade execution.
type RunOptions struct {
	// Target is the Shopware version to upgrade to.
	Target string
	// ResolvedVersions maps packages to the version the preflight resolution
	// picked; extensions are pinned to these in composer.json.
	ResolvedVersions map[string]string
	// Report is the pre-assembled report data; the run fills in the Composer
	// changes it makes and writes the report file at the end.
	Report ReportData
}

// backedUpFiles are snapshotted before any change and restored when the
// upgrade fails or is cancelled.
var backedUpFiles = []string{"composer.json", "composer.lock"}

// Run executes the upgrade asynchronously and streams progress events. The
// channel closes after a final StepFinished event. Cancelling ctx aborts the
// current step and rolls back composer.json and composer.lock. All
// file-changing operations happen here — the earlier phases are read-only.
func (u *ProjectUpgrader) Run(ctx context.Context, opts RunOptions) <-chan StepEvent {
	events := make(chan StepEvent, 64)

	go func() {
		defer close(events)

		logFile, logErr := u.openLog()
		if logErr != nil {
			events <- StepEvent{Step: StepFinished, State: StateFail, Err: logErr}
			return
		}
		defer func() { _ = logFile.Close() }()

		emit := func(ev StepEvent) {
			if ev.Line != "" {
				_, _ = fmt.Fprintln(logFile, ev.Line)
			} else if ev.State != StateRunning {
				_, _ = fmt.Fprintf(logFile, "== %s: %s\n", ev.Step.Label(), stateName(ev.State, ev.Err))
			}
			events <- ev
		}

		if err := u.backup(); err != nil {
			emit(StepEvent{Step: StepFinished, State: StateFail, Err: fmt.Errorf("backup project files: %w", err)})
			return
		}

		if err := u.runSteps(ctx, opts, emit); err != nil {
			u.restore(emit)
			u.writeFailureReport(opts.Report, err, emit)
			emit(StepEvent{Step: StepFinished, State: StateFail, Err: err})
			return
		}

		emit(StepEvent{Step: StepFinished, State: StateOK})
	}()

	return events
}

func (u *ProjectUpgrader) runSteps(ctx context.Context, opts RunOptions, emit func(StepEvent)) error {
	// The deployment helper runs Composer internally, so the audit opt-out
	// env applies to every step, not just the direct Composer invocations.
	exec := u.executor
	if env := u.composerEnv(nil); len(env) > 0 {
		exec = exec.WithEnv(env)
	}

	steps := []struct {
		id       StepID
		nonFatal bool // a failure warns instead of aborting the upgrade
		run      func(context.Context, func(string)) error
	}{
		{StepRewriteComposer, false, func(_ context.Context, line func(string)) error {
			changes, err := u.RewriteComposerJSON(opts.Target, opts.ResolvedVersions)
			if err != nil {
				return err
			}
			opts.Report.PlannedChanges = changes
			for _, c := range changes {
				line(c)
			}
			return nil
		}},
		{StepComposerUpdate, false, func(ctx context.Context, line func(string)) error {
			return streamProcess(ctx, exec.ComposerCommand(ctx, "update", "--with-all-dependencies", "--no-interaction", "--no-progress"), line)
		}},
		// symfony:recipes:install --force --reset reinstalls every Flex
		// recipe for the new package versions, the same way the Shopware
		// web-installer refreshes recipe-managed files. It is best effort:
		// a failure downgrades to a warning instead of rolling back.
		{StepRecipesInstall, true, func(ctx context.Context, line func(string)) error {
			return streamProcess(ctx, exec.ComposerCommand(ctx,
				"symfony:recipes:install", "--force", "--reset", "--yes", "--no-interaction", "--no-ansi", "-v"), line)
		}},
		{StepDeploymentHelper, false, func(ctx context.Context, line func(string)) error {
			return streamProcess(ctx, exec.PHPCommand(ctx, "vendor/bin/shopware-deployment-helper", "run"), line)
		}},
		{StepWriteReport, false, func(_ context.Context, line func(string)) error {
			path, err := u.WriteReport(opts.Report)
			if err != nil {
				return err
			}
			line("Report written to " + path)
			return nil
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

// streamProcess runs a process and forwards its combined output line by line.
// On cancellation it also runs the process's executor-specific Stop: for
// Docker environments the context kill only reaches the local docker CLI,
// while Stop signals the actual process inside the container — otherwise
// Composer keeps running there and races the rollback that follows.
func streamProcess(ctx context.Context, p *executor.Process, line func(string)) error {
	w := &lineWriter{emit: line}
	err := p.RunWithOutput(w)
	w.Flush()

	if ctx.Err() != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	}
	return err
}

// writeFailureReport records the outcome of a rolled-back upgrade so the
// report the wizard links to actually exists. Best effort: a write failure
// only logs a line.
func (u *ProjectUpgrader) writeFailureReport(data ReportData, runErr error, emit func(StepEvent)) {
	data.Failed = true
	data.Error = runErr.Error()
	path, err := u.WriteReport(data)
	if err != nil {
		emit(StepEvent{Step: StepFinished, State: StateRunning, Line: "Could not write the failure report: " + err.Error()})
		return
	}
	emit(StepEvent{Step: StepFinished, State: StateRunning, Line: "Failure report written to " + path})
}

func (u *ProjectUpgrader) openLog() (*os.File, error) {
	if err := os.MkdirAll(u.UpgradeDir(), 0o755); err != nil {
		return nil, err
	}
	return os.Create(u.LogPath())
}

// LogPath returns the location of the streamed upgrade log.
func (u *ProjectUpgrader) LogPath() string {
	return filepath.Join(u.UpgradeDir(), "upgrade.log")
}

func (u *ProjectUpgrader) backupDir() string {
	return filepath.Join(u.UpgradeDir(), "backup")
}

func (u *ProjectUpgrader) backup() error {
	if err := os.MkdirAll(u.backupDir(), 0o755); err != nil {
		return err
	}

	for _, name := range backedUpFiles {
		content, err := os.ReadFile(filepath.Join(u.projectRoot, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.WriteFile(filepath.Join(u.backupDir(), name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// restore puts composer.json and composer.lock back into their pre-upgrade
// state after a failed or cancelled run.
func (u *ProjectUpgrader) restore(emit func(StepEvent)) {
	for _, name := range backedUpFiles {
		backup := filepath.Join(u.backupDir(), name)
		content, err := os.ReadFile(backup)
		if err != nil {
			continue
		}

		current, readErr := os.ReadFile(filepath.Join(u.projectRoot, name))
		if readErr == nil && bytes.Equal(current, content) {
			continue
		}

		if err := os.WriteFile(filepath.Join(u.projectRoot, name), content, 0o644); err != nil {
			emit(StepEvent{Step: StepFinished, State: StateRunning, Line: "Could not restore " + name + ": " + err.Error()})
			continue
		}
		emit(StepEvent{Step: StepFinished, State: StateRunning, Line: "Restored " + name + " from backup."})
	}
	emit(StepEvent{Step: StepFinished, State: StateRunning, Line: "Run composer install to bring vendor/ back in sync."})
}

func stateName(s CheckState, err error) string {
	switch s {
	case StateOK:
		return "done"
	case StateWarn:
		if err != nil {
			return "warning: " + err.Error()
		}
		return "warning"
	case StateFail:
		if err != nil {
			return "failed: " + err.Error()
		}
		return "failed"
	case StateRunning:
		return "running"
	case StatePending:
		return "pending"
	}
	return "unknown"
}

// lineWriter converts a byte stream into per-line emit calls.
type lineWriter struct {
	emit func(string)
	buf  []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(bytes.TrimRight(w.buf[:idx], "\r"))
		w.buf = w.buf[idx+1:]
		w.emit(line)
	}
	return len(p), nil
}

// Flush emits any trailing output that did not end in a newline.
func (w *lineWriter) Flush() {
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}
