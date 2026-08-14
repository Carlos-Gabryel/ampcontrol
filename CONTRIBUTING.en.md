# Contributing

**English** · [Português (Brasil)](CONTRIBUTING.md)

Contributions are welcome. Before a large change, open a discussion describing the problem and expected compatibility.

## Environment

1. Install Go 1.26.6 or newer.
2. Create a branch from main.
3. Never use real credentials or IDs in tests, examples, or commits.
4. Preserve compatibility with existing installations whenever possible.
5. Any user-facing change must preserve both `pt-BR` and `en-US` behavior.

Before submitting:

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
bash scripts/test-packaging.sh
```

Integration tests require an isolated AMP installation and must never target production:

```bash
go test -tags=integration ./...
```

Pull requests should describe previous behavior, new behavior, risks, migration, and tests. Security changes must follow `SECURITY.en.md`.

Do not add logos, fonts, or other assets without compatible authorization. Identification-only trademarks must remain separate from the code license.
