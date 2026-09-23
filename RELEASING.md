# Releasing

Releases are automated by GoReleaser in GitHub Actions. Pushing a version tag
starts the release workflow; do not run GoReleaser or create the GitHub release
manually.

## Create a release

1. Ensure the commit to release is on the intended branch and all required CI
   checks have passed.
2. Create an annotated [semantic version](https://semver.org/) tag, for example:

   ```sh
   git tag -s 1.2.3 -m "1.2.3"
   ```
   
   Use a prerelease suffix such as `1.2.3-rc.1` for prereleases.

   The `-s` flag makes the tag "signed", similar to commits it is considered best practice.
   For it to work you might need to refer to [GitHub's documentation on the matter](https://docs.github.com/en/authentication/managing-commit-signature-verification/associating-an-email-with-your-gpg-key).

3. Push only the new tag:

   ```sh
   git push origin 1.2.3
   ```

The tag push triggers the `Release` workflow. It runs GoReleaser, which creates
the GitHub release and publishes the configured artifacts and package updates.
Watch the workflow to completion and verify the resulting GitHub release.

## Verify attestations

The release workflow publishes GitHub attestations for the files listed in
`dist/checksums.txt` and the Docker images listed in `dist/digests.txt`.

After a successful release, verify a downloaded archive or a published image:

```sh
gh attestation verify --owner shopware shopware-cli_Linux_x86_64.tar.gz
gh attestation verify --owner shopware oci://ghcr.io/shopware/shopware-cli:latest
```

## If a release fails

Investigate and fix the workflow failure before retrying. Do not move or reuse
a published version tag; create a new version tag for a corrected release.
