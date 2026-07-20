# Releasing

Releases are automated by GoReleaser in GitHub Actions. Pushing a version tag
starts the release workflow; do not run GoReleaser or create the GitHub release
manually.

## Create a release

1. Ensure the commit to release is on the intended branch and all required CI
   checks have passed.
2. Create an annotated [semantic version](https://semver.org/) tag, for example:

   ```sh
   git tag -a v1.2.3 -m "v1.2.3"
   ```

   Use a prerelease suffix such as `v1.2.3-rc.1` for prereleases.
3. Push only the new tag:

   ```sh
   git push origin v1.2.3
   ```

The tag push triggers the `Release` workflow. It runs GoReleaser, which creates
the GitHub release and publishes the configured artifacts and package updates.
Watch the workflow to completion and verify the resulting GitHub release.

## If a release fails

Investigate and fix the workflow failure before retrying. Do not move or reuse
a published version tag; create a new version tag for a corrected release.
