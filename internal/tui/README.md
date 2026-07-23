# internal/tui — the shopware-cli terminal UI library

Shared building blocks for every terminal UI in the CLI: plain-output helpers
for commands, Bubble Tea components for the interactive TUIs (`project dev`,
`project upgrade`), an application shell that hosts them, and overlay kits for
the common modal patterns.

```
internal/tui             flat component library (this package)
internal/tui/app         application shell: chrome, overlay stack, key/command routing
internal/tui/picker      overlay kit: filterable single-select list modal
internal/tui/prompt      overlay kit: multi-button confirm modal
internal/tui/textprompt  overlay kit: single-line text input modal
```

## Conventions

- **Components** follow `New(Options) → Render()` (or `View(width)` when they
  size themselves). One component per file, options structs over long
  parameter lists.
- **Interactive components** (`FilterList`, `Task`, `CredentialStep`) are
  value models: `Update` returns the new value plus a `tea.Cmd`, matching
  Bubble Tea idiom.
- **Colors** are package-level adaptive variables (`BrandColor`,
  `SuccessColor`, `WarnColor`, `ErrorColor`, `TextColor`, `MutedColor`,
  `BorderColor`, …) plus the semantic `Variant` enum with `VariantColor(v)`.
  There is deliberately no theme abstraction.
- **Overlays** (in the kits) close by returning `nil` from `Update` and
  deliver their outcome with `app.Emit(ResultMsg)`; the hosting content
  handles the result message.

The renders below are real component output (ANSI colors stripped).

---

## Lists & selection

### FilterList — type-to-filter windowed list

The shared mechanics behind every picker: filter input, cursor, windowing.
Enter/esc stay with the caller, which reads the choice via `Selected()`.

```go
list := tui.NewFilterList(tui.FilterListOptions{
    Items: []tui.FilterItem{
        {Label: "Main Storefront", Detail: "https://shop.example"},
        {Label: "EU Outlet", Detail: "https://eu.example"},
    },
    Header: "Sales channel",
})
list, cmd = list.Update(msg)      // up/down + filter typing
item, index, ok := list.Selected()
view := list.View(44)
```

```
> Type to filter

Sales channel
Main Storefront         https://shop.example   ← selected row is highlighted
EU Outlet                 https://eu.example
US Outlet                 https://us.example
```

### RenderSelectList — static cursor list

For short fixed choice lists inside wizard steps (no filtering).

```go
tui.RenderSelectList("Default Language", "Select the primary language", opts, cursor)
```

```
Default Language
Select the primary language

● English (UK) (en-GB)
  Deutsch (de-DE)
```

### FilterSelect / FilterMultiSelect — standalone pickers

Self-contained `tea.Program`s for CLI commands outside a hosted TUI (single
or multi select with checkboxes). Inside a hosted TUI use the `picker` kit
instead.

```go
value, err := tui.FilterSelect(ctx, "Select a shop", "Pick the target shop", items)
values, err := tui.FilterMultiSelect(ctx, "Select extensions", "", items)
```

---

## Status & progress

### StateDot / CheckRow — semantic status bullets

```go
tui.StateDot(tui.DotOK)
tui.NewCheckRow(tui.CheckRowOptions{
    State: tui.DotOK, Label: "Git working tree clean", Value: "yes", LabelWidth: 30,
}).Render()
```

```
○ pending   ◐ running   ● ok   ● warn   ● error

● Git working tree clean        yes
● PHP version                   8.1.2
```

### StepList / StepDone·StepActive·StepPending — step checklists

```go
tui.NewStepList(tui.StepListOptions{Steps: []tui.StepItem{
    {State: tui.StepStateDone, Label: "Rewrite composer.json"},
    {State: tui.StepStateActive, Label: "composer update", Indicator: spinner.View()},
    {State: tui.StepStatePending, Label: "Install recipes"},
}}).Render()
```

```
✓ Rewrite composer.json
⠋ composer update
· Install recipes
```

### StatusStrip — one-line status row

```go
tui.NewStatusStrip(tui.StatusStripOptions{
    Variant: tui.VariantError, Label: "BLOCKED",
    Message: "Fix the failing checks below, then press Recheck.",
}).Render()
```

```
BLOCKED   Fix the failing checks below, then press Recheck.
```

### Badges

```go
tui.TextBadge("Step 1/3")                      //  Step 1/3   (inverted block)
tui.StatusBadge("running", tui.SuccessColor)   // ● RUNNING
```

### Command-output status lines (non-TUI)

For plain `fmt.Println` command output:

```go
fmt.Println(tui.SectionHeadingStyle.Render("Project"))
fmt.Printf("%s PHP version: 8.3\n", tui.CheckOK)
fmt.Println(tui.SuccessLine("Development environment started in 2.4s"))
```

```
Project            (bold, underlined)
✓ PHP version: 8.3
⚠ Node missing
✓ Development environment started in 2.4s
✗ Development environment is down
```

### NewBrandSpinner — the shared spinner

```go
sp := tui.NewBrandSpinner() // spinner.Dot in BrandColor
```

### Task — run a command, stream its output

Embeddable "run job, show spinner + log tail" machinery: channel plumbing,
capped scrollback, done state. Route `TaskLineMsg`, `TaskDoneMsg`, and
`spinner.TickMsg` through `Update`.

```go
m.task = tui.NewTask("Building Administration...")
cmd := m.task.Start(func() (*exec.Cmd, error) { return buildCmd(), nil })
// in Update:
m.task, cmd = m.task.Update(msg)
// in View:
title := m.task.StatusTitle()   // "⠋ Building Administration..." while running
lines := m.task.Lines()         // capped scrollback
done, err := m.task.Done(), m.task.Err()
```

### RunSpinnerWithLogs — blocking spinner for plain commands

Runs a single `*exec.Cmd` behind a spinner with a toggleable live log panel
(ctrl+l) and prints the last log lines on failure. For CLI commands, not
hosted TUIs.

```go
err := tui.RunSpinnerWithLogs(ctx, "Installing dependencies...", cmd)
```

---

## Input

### Checkbox

```go
tui.Checkbox(checked, focused, "Show password")
```

```
[x] Show password    (muted; brand + bold when focused)
```

### ButtonRow / ConfirmButtons

Buttons wrap onto extra rows when `MaxWidth` is set and space runs out.

```go
tui.NewButtonRow(tui.ButtonRowOptions{Labels: labels, Active: 0, MaxWidth: 40}).Render()
tui.ConfirmButtons("Initialize now", "No, skip", confirmYes)
```

```
  Stop containers & quit      Quit, keep running      Cancel
  ^^^^^^^^^^^^^^^^^^^^^^ active button is inverted (brand background)
```

### CredentialStep — username/password/show-password fieldset

Embed in a wizard; `HandleKey` owns focus navigation, enter flow, and
validation — the wizard decides what a submit means.

```go
type myWizard struct {
    tui.CredentialStep
    step int
}

creds := tui.NewCredentialStep(tui.CredentialStepOptions{
    Username: "admin", Password: "shopware",
    ValidatePassword: validateAdminPassword,
})

cmd, submitted := w.HandleKey(msg)
if submitted { /* advance to the next step */ }
w.Render(&b)
hint := w.FooterHint("Install") // per-focus shortcut bar
```

```
Choose a username
admin

Choose a password  at least 8 characters
********

[ ] Show password
```

### Key helpers

```go
tui.KeyString(msg)                       // lower-cased key string ("ctrl+c" even with Caps Lock)
tui.KeyEnter, tui.KeyEsc, tui.KeyTab, …  // key string constants
tui.MoveCursor(cursor, key, count)       // up/down (k/j) with clamping
tui.ConfirmNav(confirmYes, key)          // ←/h = yes, →/l = no, tab toggles
```

---

## Layout & chrome

### Modal — centered modal box

```go
modal := tui.NewModal(tui.ModalOptions{MaxWidth: 70, AreaWidth: w, AreaHeight: h})
out := modal.Render(content)     // modal.ContentWidth() sizes inner content
```

```
     ╭────────────────────────────╮
     │                            │
     │  Are you sure?             │
     │                            │
     │  This cannot be undone.    │
     │                            │
     ╰────────────────────────────╯
```

### TwoColumn — read-only left / user-action right split

```go
tui.NewTwoColumn(tui.TwoColumnOptions{
    Width: 44, LeftWidth: 24, Left: report, Right: actions,
}).Render()
```

```
1. Check project readin… │ User action
                         │
● Git clean        yes   │ > ◉ 6.7.4.1  rec…
● Composer         ok    │   ○ 6.6.10.6
```

### WizardFrame — titled panel with status and footer rows

```go
tui.NewWizardFrame(tui.WizardFrameOptions{
    Width: 44, Height: 9,
    Title: "Upgrade check", TitleRight: "Target 6.7.4.1",
    Status: statusStrip, Body: body, Footer: shortcuts,
}).Render()
```

```
╭──────────────────────────────────────────╮
│ Upgrade check             Target 6.7.4.1 │
├──────────────────────────────────────────┤
│ RUNNING   Checking…                      │
├──────────────────────────────────────────┤
│ body content                             │
├──────────────────────────────────────────┤
│  enter  Continue                         │
╰──────────────────────────────────────────╯
```

### BrandingHeader / PhaseFooter — shared app chrome

```go
tui.BrandingHeader(width)                                  // right-aligned branding line
tui.PhaseFooter(tui.ShortcutBadge("l", "Toggle logs"), "Exit")
```

```
 ● Shopware CLI dev · Documentation · GitHub

 l  Toggle logs  │   ctrl+c  Exit
```

### ShortcutBar / ShortcutBadge — footer shortcut hints

`ShortcutBarFit` drops trailing entries that don't fit the width.

```go
tui.ShortcutBar(
    tui.Shortcut{Key: "↑/↓", Label: "Choose"},
    tui.Shortcut{Key: "enter", Label: "Confirm"},
    tui.Shortcut{Key: "esc", Label: "Cancel"},
)
```

```
 ↑/↓  Choose  │   enter  Confirm  │   esc  Cancel
```

### RenderPhaseCard — centered card with the Shopware mascot

Full-screen phase card (starting/stopping/wizard screens). There is also
`RenderPhaseCardCowsay(speech, content)` with a speech bubble.

```go
tui.RenderPhaseCard(spinner.View() + " Starting Docker containers...")
```

```
╭──────────────────────────────────╮
│      (Shopware mascot art)       │
│──────────────────────────────────│
│                                  │
│  ⠋ Starting Docker containers... │
│                                  │
╰──────────────────────────────────╯
```

### Scrollbar — vertical scrollbar column

```go
tui.NewScrollbar(tui.ScrollbarOptions{Total: 30, Visible: 5, Offset: 10, Height: 7}).Render()
```

```
↑
┆
█
┆
┆
┆
↓
```

### RenderTable / PrintTable — bordered tables for command output

```go
tui.PrintTable([]string{"Name", "Version"}, rows)
```

```
┌───────────────┬─────────┐
│ Name          │ Version │
├───────────────┼─────────┤
│ frosh/tools   │ 2.1.0   │
│ shopware/core │ 6.7.4.1 │
└───────────────┴─────────┘
```

### Labels & small helpers

```go
tui.KVRow("Username", "admin")     //   Username              admin
tui.SectionDivider(30)             // ──────────────────────────────
tui.StyledLink(url, label, style)  // OSC-8 terminal hyperlink
tui.FormatLabel("Name", "detail")  // Name (detail)
```

---

## Text utilities

All ANSI-aware (styled strings measure correctly):

```go
tui.Truncate(s, width)            // truncate with ellipsis
tui.PadRight(s, width)            // pad to a column width
tui.JoinColumns(left, right, gap) // side-by-side blocks
tui.SpreadRow(width, left, right) // left + right anchored to the edges
```

Log scrollback (shared by every streamed-output screen):

```go
lines = tui.AppendTail(lines, 500, newLine) // capped append
visible := tui.TailLines(lines, height)     // last n lines
```

Subprocess streaming:

```go
err := tui.StreamCmdOutput(cmd, ch, true)                  // lines → channel, closes on exit
cmd := tui.ReadLineCmd(ch, toMsg, doneMsg)                 // re-arm after every line
```

---

## The application shell (`internal/tui/app`)

A minimal Bubble Tea host: it owns the frame (header / main / footer),
window title, an overlay stack, size propagation, and key bindings mapped to
named commands. Content stays a plain screen.

```go
type wizard struct{ host app.Host }

func (w *wizard) Init() tea.Cmd                             { … }
func (w *wizard) Update(msg tea.Msg) (app.Content, tea.Cmd) { … }
func (w *wizard) View(ctx app.Context) string               { … } // render ctx.Width × ctx.MainHeight

shell := app.New(app.Options{
    Content:         w,
    Header:          func(ctx app.Context) string { return tui.BrandingHeader(ctx.Width) },
    Footer:          func(ctx app.Context) string { return tui.PhaseFooter("", "Exit") },
    WindowTitleFunc: func(app.Context) string { return "my-app" },
})
w.host = shell
shell.Run()
```

Key points:

- **Overlays** capture input while open; non-input messages still reach the
  Content (background work never stalls behind a modal). Push with
  `host.PushOverlay(o)`; an overlay closes by returning `nil` and emitting a
  result: `return nil, app.Emit(myResultMsg{…})`.
- **Quit** is the re-registerable `app.CmdQuit` command (default binding
  ctrl+c). Intercept it to e.g. cancel a running job instead of exiting:

  ```go
  shell.RegisterCommand(app.Command{ID: app.CmdQuit, Run: func(*app.App) tea.Cmd {
      return m.handleQuit()
  }})
  ```
- **FullscreenOverlay** (default true) makes the top overlay replace the whole
  view; `app.Ptr(false)` keeps the chrome and swaps only the main region.
- **SwapContent** switches screens (wizard → dashboard) and runs the new
  content's `Init`.
- **Sizing**: content receives `ctx.MainHeight` (terminal minus chrome);
  implement `app.Sizeable`/`app.SizePropagator` for automatic propagation.
- **Testing**: `app.NewHarness(opts, width, height)` drives the shell without
  a TTY — `h.Send(msgs…)`, `h.SendCmd(cmd)`, `h.View()`.

---

## Overlay kits

Opinionated modals built on the shell's overlay contract. All three follow
the same shape: `New(Options{Key, …})` → push → handle `ResultMsg` in your
Content, matching on `Key` when several instances exist.

### prompt — multi-button confirm

```go
return m, m.host.PushOverlay(prompt.New(prompt.Options{
    ID:      "stop-confirm",
    Title:   "Leaving the workspace",
    Message: "Do you also want to stop the running Docker containers?",
    Danger:  true,
    Choices: []prompt.Choice{
        {ID: "stop", Label: "Stop & quit"},
        {ID: "quit", Label: "Keep running"},
        {ID: "cancel", Label: "Cancel"},
    },
}))

// later:
case prompt.ResultMsg: // msg.Choice is the chosen ID ("" = dismissed with esc)
```

```
  ╭──────────────────────────────────────────────────────╮
  │                                                      │
  │  Leaving the workspace                               │
  │  Do you also want to stop the running Docker         │
  │  containers?                                         │
  │                                                      │
  │    Stop & quit      Keep running      Cancel         │
  │                                                      │
  │   ←/→  Select  │   enter  Confirm  │   esc  Cancel   │
  │                                                      │
  ╰──────────────────────────────────────────────────────╯
```

### picker — filterable single-select list

```go
return m, m.host.PushOverlay(picker.New(picker.Options{
    Key:   fieldPHPVersion,
    Title: "PHP Version",
    Items: []picker.Item{{Label: "8.2"}, {Label: "8.3"}, {Label: "8.4"}},
}))

// later:
case picker.ResultMsg: // msg.Value / msg.Index, msg.Cancelled on esc
```

```
  ╭──────────────────────────────────────────────────────╮
  │                                                      │
  │  PHP Version                                         │
  │                                                      │
  │  > Type to filter                                    │
  │                                                      │
  │  8.2                                                 │
  │  8.3                                                 │
  │  8.4                                                 │
  │                                                      │
  │   ↑/↓  Choose  │   enter  Confirm  │   esc  Cancel   │
  │                                                      │
  ╰──────────────────────────────────────────────────────╯
```

### textprompt — single-line text input

`Secret: true` masks the value (password-style echo).

```go
return m, m.host.PushOverlay(textprompt.New(textprompt.Options{
    Key:    fieldBlackfireServerID,
    Title:  "Blackfire Server ID",
    Help:   "Server ID for the Blackfire profiler",
    Value:  current,
    Secret: true,
}))

// later:
case textprompt.ResultMsg: // msg.Value, msg.Cancelled on esc
```

```
  ╭──────────────────────────────────────────────────────╮
  │                                                      │
  │  Blackfire Server ID                                 │
  │                                                      │
  │  Server ID for the Blackfire profiler                │
  │                                                      │
  │  > *******                                           │
  │                                                      │
  │   enter  Confirm  │   esc  Cancel                    │
  │                                                      │
  ╰──────────────────────────────────────────────────────╯
```

---

## Consumers

- `internal/devtui` — the `project dev` dashboard (tabs, watchers, install &
  migration wizards) hosted on `tui/app`.
- `internal/upgradetui` — the `project upgrade` wizard, hosted on `tui/app`.
- `cmd/**` — plain command output uses `PrintTable`, the status-line helpers,
  `FilterSelect`, and `RunSpinnerWithLogs`.

When adding a component: one file per component, `New(Options)` constructor,
behavior tests next to it, and update this README with a snippet and render.
