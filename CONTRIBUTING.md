# Como contribuir

[English](CONTRIBUTING.en.md) · **Português (Brasil)**

Contribuições são bem-vindas. Antes de uma alteração grande, abra uma discussão explicando o problema e a compatibilidade esperada.

## Ambiente

1. Instale Go 1.26.6 ou superior.
2. Crie uma branch a partir da principal.
3. Não use credenciais ou IDs reais em testes, exemplos ou commits.
4. Preserve compatibilidade com instalações existentes sempre que possível.
5. Toda mudança visível ao usuário deve preservar os comportamentos `pt-BR` e `en-US`.

Antes de enviar:

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
bash scripts/test-packaging.sh
```

O CI também ensaia o instalador de ponta a ponta num container descartável com systemd (`scripts/test-installer.sh`, job "Installer rehearsal"). Para repetir localmente com Docker:

```bash
CGO_ENABLED=0 go build -trimpath -o /tmp/ampcontrol ./cmd/ampcontrol
docker build -t ampcontrol-installer-test -f packaging/test/installer.Dockerfile packaging/test
docker run -d --rm --name installer-test --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw -v "$PWD:/src:ro" -v /tmp/ampcontrol:/tmp/ampcontrol:ro ampcontrol-installer-test
docker exec installer-test bash -c 'cp -r /src /work && /work/scripts/test-installer.sh /tmp/ampcontrol'
docker stop installer-test
```

Testes de integração precisam de uma instalação AMP isolada e nunca devem apontar para produção:

```bash
go test -tags=integration ./...
```

Pull requests devem explicar comportamento anterior, novo comportamento, riscos, migração e testes executados. Mudanças de segurança precisam seguir `SECURITY.md`.

Não adicione logos, fontes ou outros recursos sem licença ou autorização compatível. Marcas usadas apenas para identificação devem permanecer separadas da licença do código.
