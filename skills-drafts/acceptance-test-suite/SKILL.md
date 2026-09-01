---
name: acceptance-test-suite
description: Write and run Playwright acceptance and accessibility tests in a Shopware project that consumes the Shopware Acceptance Test Suite (@shopware-ag/acceptance-test-suite) as an npm package. Use when setting up the test workspace, writing or debugging specs, creating test data, working with actors and tasks, or checking accessibility with the bundled axe integration, including when the request just mentions ATS or acceptance tests. Not for contributing to the shopware/acceptance-test-suite repository itself.
---

# Shopware Acceptance Test Suite (consumer guide)

Use this skill when a project consumes `@shopware-ag/acceptance-test-suite` (ATS) as an npm package to test its own Shopware instance. It covers workspace setup, writing specs against the public API, test data, accessibility, and running tests.

Do not use this skill for work inside the `shopware/acceptance-test-suite` repository itself. That repository ships its own contributor skill with different rules about which internal layer a change belongs in.

The official consumer guide is https://developer.shopware.com/docs/guides/development/testing/e2e-playwright/ (the repository README points there); consult it for prose walkthroughs beyond what this skill covers.

## Determine the installed version first

ATS moves quickly and its API changes between majors. Read the version from files before giving version specific advice. Never run a package manager to find out.

Check the project root **and** `tests/acceptance/`, because a separate test workspace is the common layout and is usually where the real dependency lives. Run these from the project root:

```bash
# 1. Installed truth. Works for npm, pnpm, and Yarn's node-modules linker.
for dir in . tests/acceptance; do
  f="$dir/node_modules/@shopware-ag/acceptance-test-suite/package.json"
  [ -f "$f" ] && printf '%s -> ' "$f" && grep -m1 '"version"' "$f"
done

# 2. Lock file resolution, when node_modules is absent.
#    Keep context lines: pnpm splits specifier and resolved version, and a
#    workspace nests entries under `importers:` per directory.
grep -rn -A3 'acceptance-test-suite' \
  package-lock.json pnpm-lock.yaml yarn.lock \
  tests/acceptance/package-lock.json tests/acceptance/pnpm-lock.yaml tests/acceptance/yarn.lock 2>/dev/null

# 3. Requested range only. This is an intention, not proof of what is installed.
grep -rn 'acceptance-test-suite' package.json tests/acceptance/package.json 2>/dev/null
```

Say which of the three sources the version came from. If none yields a version, state that the ATS version could not be determined, ask the user, and avoid version specific claims until it is known.

For the version of the **shop under test** (a different thing), use the `InstanceMeta` fixture. `InstanceMeta.version` is the right way to guard a test against older Shopware releases.

## Environment and constraints

These are strict and are the usual cause of a broken setup:

- **Package**: `@shopware-ag/acceptance-test-suite`, published on npm, MIT licensed.
- **ESM only**: the package sets `"type": "module"`. The test workspace must set it too.
- **Playwright peer is an exact pin**, not a range. A mismatched `@playwright/test` is the usual cause of fixture type errors. `playwright-core` is pinned as an optional peer at the same version.
- **Node**: only the majors in the package's `engines.node` field. Read that field rather than assuming.
- **Single import path**: the `exports` map exposes only the package root. Deep imports such as `@shopware-ag/acceptance-test-suite/dist/...` are unsupported.
- **axe is bundled**: `@axe-core/playwright` and `axe-html-reporter` are runtime dependencies of ATS. Do not install them separately.
- **Only `dist/` is published.** The suite's own `playwright.config.ts`, its docker compose setup, and its `src/` tree are not in the npm tarball. Nothing in the installed package starts a server or a container.

The environment variables a typical project needs (the shipped code reads a few more for SaaS and seeded-ID setups, not listed here):

| Variable | Required | Purpose |
|---|---|---|
| `APP_URL` | yes | Storefront base URL of the shop under test, with trailing slash |
| `ADMIN_URL` | no | Administration URL, defaults to `${APP_URL}admin/` |
| `ADMIN_API_URL` | no | Admin API base URL when it is not on `APP_URL`, trailing slash required |
| `SHOPWARE_ACCESS_KEY_ID`, `SHOPWARE_SECRET_ACCESS_KEY` | preferred | Admin API integration credentials |
| `SHOPWARE_ADMIN_USERNAME`, `SHOPWARE_ADMIN_PASSWORD` | alternative | Admin user login, used when no integration credentials are set |
| `LANG` | no | Locale, read as `LANG`, `LANGUAGE`, or `lang`, default `en-GB`. Set it explicitly: a non English shell locale runs the whole suite in another language and breaks text based locators |
| `MAILPIT_BASE_URL` | for mail tests | Mailpit base URL, no default |
| `ATS_SKIP_CLEANUP` | no | Set to `1` to keep test data after a run, for debugging only |

Integration credentials are preferred over an admin user: they are faster and unaffected by admin session behavior.

## Set up the test workspace

Install ATS first, then read its pinned peer version from disk. Do not guess the Playwright version.

```bash
mkdir -p tests/acceptance && cd tests/acceptance
npm init -y
npm install --save-dev @shopware-ag/acceptance-test-suite

# Read the exact version ATS pins, then install exactly that.
grep -A4 '"peerDependencies"' node_modules/@shopware-ag/acceptance-test-suite/package.json

# --save-exact matters: without it npm records a caret range and the pin drifts.
npm install --save-dev --save-exact @playwright/test@<version from the previous command>
npm install --save-dev dotenv

# Add --with-deps only on a fresh CI image: it installs OS packages as root and
# fails where sudo needs a password.
npx playwright install chromium
```

Target layout. **Run every command in this skill from `tests/acceptance/`**, except the version detection commands above, which run from the project root.

```text
tests/acceptance/
├── .env                  # never commit
├── package.json          # "type": "module"
├── playwright.config.ts
├── fixtures/
│   └── AcceptanceTest.ts
└── tests/
    ├── checkout.spec.ts
    └── accessibility.spec.ts
```

**Edit** the generated `package.json`, do not replace it: it already holds the `devDependencies` from the installs above, and overwriting it breaks every import. Add `"type": "module"` and the scripts:

```json
{
  "type": "module",
  "scripts": {
    "test": "playwright test",
    "test:a11y": "playwright test --grep @Accessibility",
    "report": "playwright show-report"
  }
}
```

`.env` holds the shop under test. Playwright does not read `.env` by itself, which is why the config below loads it. `APP_URL` needs the trailing slash: ATS composes the admin URL as `${APP_URL}admin/`, and a project's own `.env` usually stores it without one.

```bash
APP_URL="https://shop.example.test/"
LANG="en-GB"

# Preferred: an Admin API integration. Create one in Settings, System, Integrations,
# or run `bin/console integration:create AcceptanceTest --admin` on the shop:
# it prints both lines below ready to paste.
SHOPWARE_ACCESS_KEY_ID="..."
SHOPWARE_SECRET_ACCESS_KEY="..."

# Alternative, and what a freshly installed shop already has:
# SHOPWARE_ADMIN_USERNAME="admin"
# SHOPWARE_ADMIN_PASSWORD="shopware"
```

`playwright.config.ts`. No `webServer` block: this configuration tests an instance that is already running.

```ts
import { defineConfig, devices } from '@playwright/test';
import dotenv from 'dotenv';

dotenv.config();

export default defineConfig({
  testDir: './tests',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html']] : [['list'], ['html']],
  use: {
    baseURL: process.env.APP_URL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
```

Create one base test file and import from it everywhere. This is what makes project fixtures and tasks available alongside the suite's own:

```ts
// fixtures/AcceptanceTest.ts
import { test as base } from '@shopware-ag/acceptance-test-suite';
import type { FixtureTypes } from '@shopware-ag/acceptance-test-suite';

export * from '@shopware-ag/acceptance-test-suite';

export const test = base.extend<FixtureTypes>({
  // project specific fixtures go here
});
```

When the project has its own task collections, merge instead of extending:

```ts
import { mergeTests, test as ShopwareTestSuite } from '@shopware-ag/acceptance-test-suite';
import { test as projectTasks } from './ProjectTasks';

export * from '@shopware-ag/acceptance-test-suite';

export const test = mergeTests(ShopwareTestSuite, projectTasks);
```

### Typecheck the tests

Playwright transpiles without typechecking, so type errors run silently and surface as wrong behavior instead of a failure. Add a `tsconfig.json` and run `tsc` in CI alongside the tests.

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true,
    "types": ["node"]
  },
  "include": ["fixtures/**/*.ts", "tests/**/*.ts", "playwright.config.ts"]
}
```

```bash
npm install --save-dev typescript
npx tsc --noEmit
```

This is how the `attemptsTo(ValidateAccessibility(...))` mistake described under Accessibility gets caught.

## Write a spec

Everything the suite offers arrives as Playwright fixtures destructured in the test callback: actors, tasks, page objects, and services. There is no separate setup call.

```ts
// tests/checkout.spec.ts
import { test } from '../fixtures/AcceptanceTest';

test('Customer can add a product to the cart.', { tag: '@Checkout' }, async ({
  ShopCustomer,
  TestDataService,
  AddProductToCart,
  StorefrontProductDetail,
  StorefrontCheckoutCart,
}) => {
  const category = await TestDataService.createCategory();
  const product = await TestDataService.createBasicProduct();
  await TestDataService.assignProductCategory(product.id, category.id);

  await ShopCustomer.goesTo(StorefrontProductDetail.url(product));
  await ShopCustomer.attemptsTo(AddProductToCart(product, '5'));

  await ShopCustomer.goesTo(StorefrontCheckoutCart.url());
  await ShopCustomer.expects(StorefrontCheckoutCart.grandTotalPrice).toBeVisible();
});
```

Rules that keep specs working:

- **Create the data the test needs.** Never rely on demo data or on data another test created.
- **One browser surface per test.** `ShopCustomer` for storefront coverage, `ShopAdmin` for administration coverage. Do not model flows that hop from administration to storefront and back in one test.
- **Use page objects for URLs and elements**, so a routing or markup change is fixed in one place.
- **`expects` is Playwright's `expect`**, reachable through the actor for readability.

### Actors

| Actor | Surface |
|---|---|
| `ShopCustomer` | storefront |
| `ShopAdmin` | administration |

Methods: `goesTo(url, forceReload?)`, `attemptsTo(task)`, `expects(locator)`, `presses(locator, key?)`, `fillsIn(locator, input)`, `selectsRadioButton(group, label)`, `a11y_checks(locator)`.

`a11y_checks` scrolls the locator into view and asserts it is enabled, focused, and shows a visible focus indicator. `presses`, `fillsIn`, and `selectsRadioButton` call it internally, so keyboard operability and a visible focus ring are asserted as a side effect of using them.

**For storefront interactions, use the actor methods by default.** Write `ShopCustomer.presses(...)` and `ShopCustomer.fillsIn(...)` rather than `click` and `fill`, in ordinary functional tests and not only in dedicated accessibility specs. Every interaction then also asserts keyboard operability and a visible focus indicator, which is coverage you would otherwise have to write by hand. This is the suite's own convention for storefront flows and the existing tasks are written that way.

```ts
// Preferred
await ShopCustomer.fillsIn(StorefrontAccountLogin.emailInput, customer.email);
await ShopCustomer.presses(StorefrontAccountLogin.loginButton);
```

Two things to know when using them:

- They drive the keyboard instead of clicking, so they skip some of Playwright's click actionability checks. Assert state first where a control may be hidden or disabled:

  ```ts
  await ShopCustomer.expects(submitButton).toBeVisible();
  await ShopCustomer.expects(submitButton).toBeEnabled();
  await ShopCustomer.presses(submitButton);
  ```

- Converting an existing suite surfaces genuine accessibility defects: a control that was never keyboard operable now fails. Triage those findings, do not suppress them by reverting to `click`.

**Administration flows are the documented exception.** The suite's own guidance prefers the keyboard first methods for storefront flows and says not to generalize that rule to the Administration, so use plain Playwright there unless the test is specifically about keyboard operability. The methods exist on `ShopAdmin` and work, but the Administration is not covered by the storefront convention and has no accessibility task equivalent.

### Tasks

A task is a reusable, named interaction exposed as a fixture, read as a sentence: `Login()`, `AddProductToCart(product, '5')`, `SubmitOrder()`. Destructure it and pass it to `attemptsTo`.

Author a project task the same way the suite does, as a fixture returning a function that returns the closure:

```ts
import { test as base } from '@playwright/test';
import type { FixtureTypes, Task } from '@shopware-ag/acceptance-test-suite';

export const AcceptCookieBanner = base.extend<{ AcceptCookieBanner: Task }, FixtureTypes>({
  AcceptCookieBanner: async ({ ShopCustomer, StorefrontHome }, use) => {
    const task = () => {
      return async function AcceptCookieBanner() {
        await ShopCustomer.presses(StorefrontHome.consentAcceptAllCookiesButton);
      };
    };
    await use(task);
  },
});
```

Name the returned function: `attemptsTo` derives the test step title from it. Keep assertions about the thing under test in the spec, not in the task.

### Test data

`TestDataService` creates entities through the admin API and cleans them up after each test. Method names follow a pattern: `createBasic*`, `create*With*`, `assign*`, `get*`.

```ts
const product = await TestDataService.createBasicProduct({ name: 'Test product' });
const category = await TestDataService.createCategory();
await TestDataService.assignProductCategory(product.id, category.id);
```

- Cleanup respects entity dependencies, so create through the service where possible.
- An entity created outside the service can be registered for cleanup; prefer that over leaving data behind.
- `ATS_SKIP_CLEANUP=1` keeps data for inspection. Never leave it set in CI.
- For anything the service does not model, use `AdminApiContext` or `StoreApiContext`.

### Locators

Preference order, most robust first:

1. `getByRole` with an accessible name. This tests what assistive technology exposes.
2. `getByLabel` and `getByAltText`.
3. `getByTestId`, for controls a test depends on, added deliberately in a template override. This step is general Playwright practice; ATS's own guidance lists only role, label, placeholder, and text.
4. `getByText`, only where the text is the contract.

Avoid XPath and CSS coupled to styling or DOM depth. Prefer a label or role over a placeholder: ATS's own guidance permits `getByPlaceholder` where it is stable, but placeholder text is translated and often absent from accessible markup, so it is rarely stable in the multilingual setups Shopware projects have. A field with no accessible label to target is usually an accessibility defect worth fixing rather than working around.

Prefer page object properties over inline locators.

## Accessibility

The suite ships exactly one accessibility task, `ValidateAccessibility`, built on the bundled `@axe-core/playwright`. It is bound to the shop customer's page: there is no administration equivalent, so do not promise administration accessibility coverage through it.

```ts
// tests/accessibility.spec.ts
import { test } from '../fixtures/AcceptanceTest';

test('The storefront implements accessibility best practices.', { tag: '@Accessibility' }, async ({
  ShopCustomer,
  TestDataService,
  ValidateAccessibility,
  StorefrontAccountLogin,
  StorefrontCategory,
}) => {
  test.slow();

  const category = await TestDataService.createCategory();

  await test.step('Login page', async () => {
    await ShopCustomer.goesTo(StorefrontAccountLogin.url());
    await ValidateAccessibility('Login', true)();
  });

  await test.step('Category page', async () => {
    await ShopCustomer.goesTo(StorefrontCategory.url(category.name));
    // Known violation: the first level headline is CMS configurable.
    await ValidateAccessibility('Category', 1)();
  });
});
```

Note the call style. `ValidateAccessibility` is the one task that **cannot** be passed to `attemptsTo`: its closure resolves to the axe results array, which does not satisfy `attemptsTo`'s `() => Promise<void>` parameter, so `tsc` rejects it. Published examples elsewhere do use `attemptsTo(ValidateAccessibility(...))`; that works at runtime (Playwright transpiles without typechecking) but fails a typecheck and discards the results. Call the closure directly, as above.

Signature and behavior:

```
ValidateAccessibility(pageName, assertViolations?, createReport?, ruleTags?, outputDir?)
```

- `pageName` labels the report. The filename strips every non alphabetic character, so `'Page Not Found'` becomes `PageNotFound.html`.
- `assertViolations` is polymorphic:
  - `true` (default) requires zero violations.
  - a **number** allows at most that many violations. This is a budget, not a targeted waiver: any new violation of any rule hides under the budget.
  - `false` asserts nothing and only returns results.
- `createReport` defaults to true and writes `test-results/AccessibilityReports/<PageName>.html`.
- `ruleTags` defaults to `['wcag2a', 'wcag2aa', 'wcag2aaa', 'wcag21a', 'wcag21aa', 'best-practice']`. That is broader than most projects' stated target: it enforces AAA rules and axe best practice rules, neither of which is a WCAG 2.1 AA requirement, so a default run can fail on issues that are not conformance failures. Pass an explicit list matching the project's target, for example `['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']`, and keep it consistent across specs.
- `outputDir` overrides the report directory.
- The closure returns the violations array, so a spec can inspect or attach it: `const violations = await ValidateAccessibility('Home', false)();`

Guidance:

- One page per `test.step`, so a failure names the page.
- Use `test.slow()`: axe analysis across several pages exceeds the default timeout.
- When a violation is accepted for now, put the reason in a comment beside the call and link a tracking issue. Prefer a tighter rule set over a numeric budget, because a budget hides regressions.
- Automated checks are a floor, not proof of accessibility. Keyboard traversal, focus management in modals and off canvas panels, and screen reader behavior still need explicit tests or manual review.

## Run tests

From `tests/acceptance/`:

```bash
npx playwright test                                  # everything
npx playwright test tests/checkout.spec.ts           # one file
npx playwright test --grep @Accessibility            # by tag
npx playwright test --headed --project=chromium      # watch it run
npx playwright show-report                           # last HTML report
```

Debugging:

- `--debug` opens the inspector; `--trace on` records a trace for `npx playwright show-trace`.
- `ATS_SKIP_CLEANUP=1` preserves created data so the shop can be inspected after a failure.
- Accessibility reports land in `test-results/AccessibilityReports/`.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| Fixture types missing, or `test.extend` type errors | `@playwright/test` does not match ATS's exact pinned peer. Align it. |
| `ERR_REQUIRE_ESM` or import failures | The test workspace is not ESM. Set `"type": "module"`. |
| Unresolved deep import | Only the package root is exported. Import from `@shopware-ag/acceptance-test-suite`. |
| `APP_URL` or credentials appear unset | Nothing loads `.env` by itself. Load it in `playwright.config.ts`. |
| Admin API calls fail with 401 | Credentials missing or wrong, or the API is not on `APP_URL`. Prefer integration credentials; set `ADMIN_API_URL` when the API lives elsewhere. |
| Tests act on the wrong surface | The bare Playwright `page` fixture is not the storefront page. Use the actors, or the explicit `StorefrontPage` and `AdminPage` fixtures. |
| Text based locators fail in another language | `LANG` is inherited from the shell. Set it explicitly. |
| Data from previous runs pollutes tests | `ATS_SKIP_CLEANUP` is set in the environment. |
| Playwright waits forever, or tries to start a server | Your own config's `webServer` block, not ATS: the published package ships only `dist/` and starts nothing. Omit `webServer` when testing a running instance, or use it purely as a readiness gate. |
| Accessibility spec times out | Add `test.slow()` and split page audits into steps. |
| `attemptsTo(ValidateAccessibility(...))` fails to typecheck | Expected. Call the closure directly: `await ValidateAccessibility('Name', true)();` |

## When guidance and reality disagree

The installed package is the authority. Verify against it rather than against this skill or older documentation:

```bash
cat node_modules/@shopware-ag/acceptance-test-suite/package.json
cat node_modules/@shopware-ag/acceptance-test-suite/dist/index.d.ts | grep -E 'export|declare' | head -50
```

The type definitions list the real exports, fixtures, and task names for the installed version. Published examples elsewhere sometimes use fixture names from older majors; prefer what the installed types show. If this skill and the installed package disagree, follow the package and say so.
