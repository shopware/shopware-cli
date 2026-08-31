# testhelper

Shared fixture builders for tests. This package removes the boilerplate of
building Shopware project and extension trees on disk — temp directories,
composer.json / composer.lock rendering, and app manifest.xml rendering — so
tests state *what* the fixture is, not how to write it.

Everything here is behavior-first: builders render deterministic output, omit
zero-valued fields, and are covered by this package's own tests. When a fixture
can't be expressed (see [When to keep raw strings](#when-to-keep-raw-strings)),
fall back to a raw string through `WriteFile` — that escape hatch is part of
the design, not a workaround.

## Files and directories

### `WriteFile(t, path, content)`

The atom: mkdir-p the parent, write the file, fail the test on error.
Replaces the `os.MkdirAll` + `os.WriteFile` + error-check triple.

```go
testhelper.WriteFile(t, filepath.Join(dir, ".env"), "DATABASE_URL=mysql://app:secret@db/shop\n")
```

### `Project` — a Shopware project root

Chainable builder around a fresh temp dir for project-shaped fixtures
(root composer.json + lock + vendor + custom/plugins):

```go
p := testhelper.NewProject(t).
    File("composer.json", testhelper.ComposerJSON{
        Name:    "shopware/production",
        Require: map[string]string{"shopware/core": "6.6.10.3"},
    }.String()).
    File("composer.lock", testhelper.ComposerLock(
        testhelper.LockPackage{Name: "shopware/core", Version: "v6.6.10.3"},
    )).
    VendorPackage("swag/demo", testhelper.PluginComposer("swag/demo", "2.0.0", `Swag\Demo\Demo`)).
    CustomPlugin("LocalPlugin", testhelper.PluginComposer("acme/local-plugin", "1.0.0", `Acme\LocalPlugin\LocalPlugin`)).
    Dir("var/cache") // empty directory, no file needed

root := p.Root
```

`File` and `Dir` take slash-separated paths relative to `Root`. Scenario
helpers (a package's `setupProject(t)`) should stay package-local and compose
these calls — this package provides the atoms, not the scenarios.

### Standalone extension directories

For a single plugin/bundle/app rooted in its own temp dir (not inside a
project):

```go
// The composer content matters to the test: keep it visible at the call site.
dir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{
    Name: "shopware/invalid",
    Type: "invalid", // the test asserts this is rejected
})

// The manifest content matters:
appPath := testhelper.AppDir(t, manifest) // manifest is a testhelper.AppManifest

// Any valid extension will do — the test doesn't assert its metadata:
pluginDir := testhelper.NewPlugin(t, "FroshTools") // test/frosh-tools, class FroshTools\FroshTools, v1.0.0
appDir := testhelper.NewApp(t, "MyExampleApp")
```

Rule of thumb: if the test asserts names, versions, classes, or labels — or
the fixture is deliberately broken — use `ExtensionDir`/`AppDir` with an
explicit builder so the reader sees the arrange step. Use `NewPlugin`/`NewApp`
only when the fixture is incidental.

## composer.json

`ComposerJSON` renders a manifest; zero-valued fields are omitted, so partial
and degenerate manifests (`{}`, version-only) stay expressible:

```go
testhelper.ComposerJSON{Name: "swag/demo", Type: "shopware-bundle"}.String()
```

Supported fields: `Name`, `Type`, `Version`, `License`, `Description`,
`Authors` (rendered as `[{"name": ...}]`), `Require`, `RequireDev`,
`PluginClass` (`extra.shopware-plugin-class`), `Label` (`extra.label`),
`Psr4` (`autoload.psr-4`), and `Extra map[string]any` for anything else under
`extra` (e.g. `shopware-bundle-name`, `manufacturerLink`).

`PluginComposer(name, version, class)` is the canonical platform plugin:
`type: shopware-platform-plugin`, `require: {"shopware/core": "~6.6.0"}`, with
the en-GB label and PSR-4 prefix derived from the bootstrap class. Override
fields on the returned value when the defaults don't fit:

```go
c := testhelper.PluginComposer("frosh/tools", "2.0.0", `Frosh\Tools\FroshTools`)
c.Require = map[string]string{"shopware/core": "~6.7.0"}
```

An empty version renders no `version` key (useful for fallback-version tests).

## composer.lock

`ComposerLock(packages...)` renders `{"packages": [...], "packages-dev": []}`:

```go
testhelper.ComposerLock() // empty lock
testhelper.ComposerLock(
    testhelper.LockPackage{Name: "shopware/core", Version: "v6.6.10.3",
        Require: map[string]string{"php": ">=8.2"}},
    testhelper.LockPackage{Name: "swag/demo", Version: "2.0.0", Type: "shopware-platform-plugin"},
)
```

`LockPackage` carries `Name`, `Version`, `Type`, and a per-package `Require`.
Locks needing more (per-package `license` arrays, populated `packages-dev`)
stay raw — see below.

## App manifest.xml

`NewAppManifest(name)` returns a complete, valid manifest with placeholder
metadata (label/description in en + de-DE, author, copyright, version 1.0.0,
MIT). Zero-valued fields are omitted, so validator tests express "manifest
missing exactly one element" by clearing that field:

```go
m := testhelper.NewAppManifest("MyExampleApp")
m.License = ""          // missing-license variant
m.Compatibility = "~6.5.0"
m.Icon = "app.png"
m.SetupSecret = "foo"   // adds a <setup><secret> block
testhelper.WriteFile(t, filepath.Join(dir, "manifest.xml"), m.String())
// or: dir := testhelper.AppDir(t, m)
```

`Label` and `Description` are `map[lang]text` with `""` as the default
language, rendered default-first.

## When to keep raw strings

Do **not** force a fixture through a builder when:

- **The test asserts byte-exact file content** (e.g. content compared after a
  git archive round-trip, or a renderer asserted to leave a file untouched).
  A builder's formatting is not part of its contract.
- **The fixture is the test data**: broken JSON, `"{}"`, partial documents
  exercising parse-error paths.
- **The shape is intentionally inexpressible**: manifests with arbitrary
  unknown XML elements asserted to survive rewriting, locks with per-package
  license arrays, composer files with `repositories` blocks.
- **The file needs special permissions** (executables, `0600` credentials) —
  `WriteFile` always writes `0o644`.

In those cases write the raw string via `testhelper.WriteFile` (or plain
`os.WriteFile` for special permissions) and, where it isn't obvious, leave a
short comment saying why the builder doesn't apply.

## Adding to this package

Add a field or helper only when a second (ideally third) call site needs it —
single-use shapes stay raw at their call site. Every addition needs coverage
in this package's tests, and rendering must stay deterministic (sort map keys
before emitting). Test doubles for interfaces (executors, validation checks)
do not belong here yet; they live package-local.
