# Como contribuir

Contribuições são bem-vindas. Antes de uma alteração grande, abra uma discussão explicando o problema e a compatibilidade esperada.

## Ambiente

1. Instale Go 1.26.6 ou superior.
2. Crie uma branch a partir da principal.
3. Não use credenciais ou IDs reais em testes, exemplos ou commits.
4. Preserve compatibilidade com instalações existentes sempre que possível.

Antes de enviar:

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
bash scripts/test-packaging.sh
```

Testes de integração precisam de uma instalação AMP isolada e nunca devem apontar para produção:

```bash
go test -tags=integration ./...
```

Pull requests devem explicar comportamento anterior, novo comportamento, riscos, migração e testes executados. Mudanças de segurança precisam seguir `SECURITY.md`.

Não adicione logos, fontes ou outros recursos sem licença ou autorização compatível. Marcas usadas apenas para identificação devem permanecer separadas da licença do código.
