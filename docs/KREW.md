# Install as a kubectl plugin (krew)

draincheck ships a [krew](https://krew.sigs.k8s.io/) manifest, `plugins/drain-check.yaml`, so it can run as `kubectl drain-check`.

> **Status: *planned*.** The manifest is in the repository, but it is not in the `krew-index` yet, so `kubectl krew install drain-check` does not work today. Install from the [releases page](RELEASE.md) until it does.

## Naming

kubectl turns the dashes of a plugin name into underscores in the file name: `kubectl drain-check` runs the executable `kubectl-drain_check` from `PATH`. (A file named `kubectl-drain-check` would be run by `kubectl drain check`, as a subcommand.) krew creates that link itself. Each release archive therefore holds two copies of the same program:

| File                                 | Use                                   |
| ------------------------------------ | ------------------------------------- |
| `draincheck` / `draincheck.exe`      | standalone command                    |
| `kubectl-drain_check` / `.exe`        | what krew links as the kubectl plugin |

Both are built from `./cmd/draincheck` (see `.goreleaser.yaml`). Behaviour, flags and exit codes are identical.

## Reading the other docs when you use the plugin

The README, `docs/USAGE.md` and the examples write the command as `draincheck`. Through krew, replace `draincheck` with `kubectl drain-check` and keep every flag, exit code and output as written: `kubectl drain-check --nodes worker-01,worker-02` is `draincheck --nodes worker-01,worker-02`. The help text and error messages print `draincheck` in both cases.

## Install once it is listed

```sh
kubectl krew install drain-check
kubectl drain-check --nodes worker-01,worker-02
```

Try the manifest from a release without the index:

```sh
kubectl krew install --manifest=plugins/drain-check.yaml
```

This needs the real `sha256` values (below); with the `REPLACE_` placeholders krew refuses the download.

## Update the manifest for a release (maintainer)

The `sha256` values are placeholders until the release exists, because they are the hashes of the published archives.

1. Tag and publish the release ([RELEASE.md](RELEASE.md)); the archives must contain `kubectl-drain_check`.
2. Take the hashes from the verified `checksums.txt` of that release. Do not hash a different download.
3. In `plugins/drain-check.yaml` set `spec.version` and every `uri` to the new tag and every `sha256` to the matching line of `checksums.txt`.
4. Run `go test ./tests -run Krew` and the `--manifest` install above on a real cluster.

## Submit to krew-index (maintainer, *planned*)

Done after a release, by the operator, because it opens a pull request on another project:

1. Fork `kubernetes-sigs/krew-index` and copy `plugins/drain-check.yaml` to `plugins/drain-check.yaml` there.
2. Run `kubectl krew install --manifest=plugins/drain-check.yaml` and `kubectl krew uninstall drain-check` once to validate it.
3. Open the pull request and follow the [krew-index review rules](https://krew.sigs.k8s.io/docs/developer-guide/release/submitting-to-krew-index/).
4. After it is merged, new versions are bumped by pull request or by the krew-release-bot; decide which when the first listing lands.
