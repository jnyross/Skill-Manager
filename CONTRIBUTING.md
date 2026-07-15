# Contributing

Thank you for helping improve Skillet. This is a small personal project, so a
proposal may be declined even when it is reasonable.

Before substantial work, open an issue describing the user problem and the
smallest observable outcome. Keep changes focused and preserve Skillet's
reversible-by-default behavior.

From the repository root, run:

```sh
gofmt -w cmd internal
go test ./...
go vet ./...
node --test packaging/npm/packages/skillet/test/*.test.cjs
node packaging/npm/scripts/validate-packages.mjs
```

Do not commit generated native binaries, npm tarballs, credentials, machine
state, copied agent skills, or unrelated formatting. By participating, you
agree to follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Security reports must
use the private route in [SECURITY.md](SECURITY.md).
