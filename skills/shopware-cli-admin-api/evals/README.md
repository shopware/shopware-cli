# Admin API skill evaluation

The scenarios in [scenarios.json](scenarios.json) are a proposed behavioral
suite. They are not results. Run them against isolated test shops before
reporting a pass rate; record unrun scenarios as unrun.

## Test environments

Use disposable shops with non-sensitive test data. Keep the same CLI version,
agent model, tools, and permissions in both conditions where compatible. For
version coverage, use separate Shopware 6.6 and 6.7 shops and record their exact
versions, the CLI version, PHP version, installed extensions and source of
each API reference. Confirm the CLI and runtime support each selected version;
do not infer support from this test plan. If an environment cannot be prepared,
record that limitation rather than marking its scenarios as passing.

Use the same product-search task in each version's dedicated scenario. The
suite also includes searches across pages, an isolated update, an action,
restricted permissions, missing or truncated reference sections, a custom
endpoint and an Admin-to-Store API workflow. The mixed workflow stops at Store
API verification; checkout is outside its scope.

Each scenario's `setup` describes its fixture when needed. Prepare the same
fixture for each condition and reset or recreate it between runs. For the 403
case, use an integration that can authenticate and reach initialization but
cannot read products. For the missing and truncated reference cases, simulate
the described tool output consistently; don't rely on a real outage. To test
the custom endpoint, provide a small test extension and its version-matched
reference. Do not use production data or real customer credentials.

## Run and assess

1. Start a fresh agent session with the skill installed. Run the scenario
   prompt against its specified fixture and retain a redacted trace of source
   lookups, API operations, responses and final output.
2. Restore the fixture and repeat with a fresh agent session without this
   skill. Keep task, shop, other tools and permissions the same. For a new
   skill this is the baseline; later versions compare against the previous
   version as well.
3. Have a separate judge assess each criterion as pass, fail or not applicable
   from the trace. Judge action order as well as the final answer: later
   documentation checks do not prove a source was checked before a request.
4. Record the exact instruction hash, fixture, versions, run dates, scores and
   limitations in the version's report. Redact tokens, credentials, access
   keys and personal data from traces and reports. Keep this plan distinct from
   results; a missing environment or unrun comparison is not a pass.

The Store API scenario may change the specified test product's visibility and
exercise a sales-channel context. Use a dedicated channel and test account;
don't register a customer or place an order as part of this scenario. Apply
any necessary cleanup only to the disposable fixtures.
