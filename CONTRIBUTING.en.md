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

CI also rehearses the installer end to end in a disposable systemd container (`scripts/test-installer.sh`, "Installer rehearsal" job). To repeat it locally with Docker:

```bash
CGO_ENABLED=0 go build -trimpath -o /tmp/ampcontrol ./cmd/ampcontrol
docker build -t ampcontrol-installer-test -f packaging/test/installer.Dockerfile packaging/test
docker run -d --rm --name installer-test --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw -v "$PWD:/src:ro" -v /tmp/ampcontrol:/tmp/ampcontrol:ro ampcontrol-installer-test
docker exec installer-test bash -c 'cp -r /src /work && /work/scripts/test-installer.sh /tmp/ampcontrol'
docker stop installer-test
```

Integration tests require an isolated AMP installation and must never target production:

```bash
go test -tags=integration ./...
```

Pull requests should describe previous behavior, new behavior, risks, migration, and tests. Security changes must follow `SECURITY.en.md`.

Do not add logos, fonts, or other assets without compatible authorization. Identification-only trademarks must remain separate from the code license.
