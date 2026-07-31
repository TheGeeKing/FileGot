# Tag-driven app version

Release versions come from git tags (`v0.2.0`), not from editing `FyneApp.toml` on each release. CD passes `--app-version` (tag with the leading `v` stripped) and `--app-build` (`github.run_number`) to `fyne package`, so About reads the baked Fyne metadata while the committed toml stays a local packaging default. We use a native OS matrix instead of fyne-cross so Windows/macOS release packaging does not require a host macOS SDK or cross-release constraints.
