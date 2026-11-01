# Release and verification

Releases are built by `.github/workflows/release.yml` when a `vX.Y.Z` tag is pushed. Binaries are published for Linux, macOS and Windows, each on `amd64` and `arm64`. All targets are cross-compiled on one Linux runner; no macOS or Windows runner is needed.

## Artifacts

| File                                         | What                                                        |
| -------------------------------------------- | ----------------------------------------------------------- |
| `draincheck_<version>_<os>_<arch>.tar.gz`    | Linux and macOS: `draincheck`, `kubectl-drain_check` (the same program, for [krew](KREW.md)), `LICENSE`, `README.md` |
| `draincheck_<version>_windows_<arch>.zip`    | Windows: `draincheck.exe`, `kubectl-drain_check.exe`, `LICENSE`, `README.md` |
| `checksums.txt`                              | SHA-256 of every archive                                    |
| `checksums.txt.sigstore.json`                | cosign keyless signature bundle for `checksums.txt`         |
| `draincheck.intoto.jsonl`                    | SLSA provenance for every archive (slsa-github-generator)   |

## Install

`<os>` is `linux` or `darwin`; `<arch>` is `amd64` or `arm64`.

Linux and macOS:

```sh
VERSION=v0.1.0
OS=linux     # or darwin
ARCH=amd64   # or arm64
curl -fsSLO "https://github.com/Veinar/draincheck/releases/download/${VERSION}/draincheck_${VERSION#v}_${OS}_${ARCH}.tar.gz"
tar -xzf "draincheck_${VERSION#v}_${OS}_${ARCH}.tar.gz" draincheck
sudo install -m 0755 draincheck /usr/local/bin/draincheck
```

Windows (PowerShell): download `draincheck_<version>_windows_<arch>.zip` from the release page, run `Expand-Archive`, and put `draincheck.exe` on your `PATH`.

macOS: binaries are not notarized. If Gatekeeper blocks the binary, verify it (below), then run `xattr -d com.apple.quarantine draincheck`.

Verify before you install (below).

## Verify

Download the archive, `checksums.txt`, `checksums.txt.sigstore.json` and `draincheck.intoto.jsonl` from the release page, then:

1. Verify the checksum file signature ([cosign](https://github.com/sigstore/cosign)):

   ```sh
   cosign verify-blob \
     --bundle checksums.txt.sigstore.json \
     --certificate-identity "https://github.com/Veinar/draincheck/.github/workflows/release.yml@refs/tags/${VERSION}" \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     checksums.txt
   ```

2. Verify the archive matches the checksum file:

   ```sh
   sha256sum --check --ignore-missing checksums.txt   # macOS: shasum -a 256 --check --ignore-missing checksums.txt
   ```

3. Verify the SLSA provenance ([slsa-verifier](https://github.com/slsa-framework/slsa-verifier)):

   ```sh
   slsa-verifier verify-artifact \
     "draincheck_${VERSION#v}_${OS}_${ARCH}.tar.gz" \
     --provenance-path draincheck.intoto.jsonl \
     --source-uri github.com/Veinar/draincheck \
     --source-tag "${VERSION}"
   ```

## Cut a release

1. Move the `[Unreleased]` entries of `CHANGELOG.md` under `## [X.Y.Z] - <date>` and merge to `develop`, then tag a signed tag: `git tag -s vX.Y.Z -m "vX.Y.Z" && git push origin vX.Y.Z`. Keep a `### Stability` block in the new section (what is meant to hold, what can still change): `docs/USAGE.md` sends readers to the latest release section for it.
2. Watch the `Release` workflow. All three jobs must pass.
3. Run the verification steps above against the published assets.
4. Update `plugins/drain-check.yaml` with the new tag and the hashes from `checksums.txt` ([KREW.md](KREW.md)).

The workflow creates the release as a draft, attaches the SLSA provenance to it and only then publishes it. This is required when release immutability is enabled for the repository: assets cannot be added to a published release, and the tag name of a published release cannot be reused. If the run fails before the last job, the release stays a draft. Delete it (`gh release delete vX.Y.Z --yes`), delete the tag, fix the cause and tag again. Nothing has been published.

The release notes are the `## [X.Y.Z]` section of `CHANGELOG.md` for the tag (`.github/scripts/release-notes.sh`). If the section is missing or empty, the workflow fails before it builds anything. Only `vX.Y.Z` tags trigger the workflow; there are no release candidates or other suffixes.

## Test on a fork

Push a tag to a fork. The workflow runs there and publishes to the fork's releases. Replace `Veinar/draincheck` in the verification commands with the fork path.

## Notes

- Signing identity is the workflow file at the tag ref, so any change to `release.yml` changes the identity string.
- `slsa-github-generator` is referenced by tag (`@v2.1.0`), not by SHA: the generator checks its own ref and rejects a SHA. Scorecard `Pinned-Dependencies` may flag this line.
- Tokens: top-level `read-all`. `contents: write` and `id-token: write` only on `release`; `provenance` adds `actions: read`.
