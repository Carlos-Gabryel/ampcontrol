# AmpControl

O AmpControl conecta o [AMP, da CubeCoders](https://cubecoders.com/AMP), ao Discord para que uma comunidade acompanhe e controle servidores de jogos sem precisar acessar o painel administrativo.

O projeto foi desenvolvido para uma instalação AMP local, um servidor Discord e Linux com systemd. O instalador detecta o AMP e suas instâncias, solicita apenas os dados que não podem ser descobertos e armazena credenciais com `systemd-creds`.

## Principais recursos

- cartões fixos por servidor com jogo, endereço, estado, uso de CPU e memória, tempo online e jogadores;
- botões e comandos `/amp` para iniciar, parar, reiniciar, desligar e atualizar instâncias;
- proteção de partidas: usuários comuns não podem interromper um servidor com jogadores ativos;
- Idle automático individual, com 15 minutos por padrão;
- contagem de jogadores pela API do AMP e fallback RCON para jogos configurados;
- inventário automático via ADS, com fallback local pelo `ampinstmgr`;
- auditoria de comandos com usuário, horário e resultado;
- diagnóstico privado de Discord, AMP, Idle, RCON e painel;
- administração separada em `/ampconfig`;
- restrição opcional dos comandos a um único canal;
- segredos criptografados em repouso e entregues ao processo pelo systemd.

## Escopo atual

| Suportado agora | Planejado |
| --- | --- |
| Linux com systemd | Windows |
| Uma instalação AMP local | AMP remoto |
| Um servidor Discord por bot | Múltiplos servidores Discord |
| Instalação interativa | Pacotes e releases pré-compiladas |

## Instalação rápida

Pré-requisitos: AMP já instalado, Linux com systemd e `systemd-creds`, acesso `sudo` e Go 1.26.6 ou superior. Um binário pré-compilado também pode ser informado com `--binary`.

```bash
git clone https://github.com/Carlos-Gabryel/ampcontrol.git
cd ampcontrol
sudo ./scripts/install.sh
```

O assistente detecta o usuário e o inventário do AMP, configura Discord e permissões, cria as credenciais criptografadas e instala o serviço. Leia o [guia de instalação](docs/INSTALLATION.md) antes de instalar em produção.

## Comandos

Comandos públicos:

| Comando | Ação |
| --- | --- |
| `/amp status` | Atualiza o painel fixo |
| `/amp iniciar` | Inicia a instância e o jogo |
| `/amp parar` | Para o jogo e mantém a instância em Idle |
| `/amp reiniciar` | Reinicia o processo do jogo |
| `/amp desligar` | Desliga completamente a instância, após confirmação |
| `/amp atualizar` | Atualiza a instalação AMP da instância, após confirmação |

`/ampconfig` é reservado ao proprietário e aos cargos administrativos configurados. Ele oferece diagnóstico, apresentação das instâncias, visibilidade e cadastro no Idle. A lista completa está em [Configuração](docs/CONFIGURATION.md).

## Comportamento de novas instâncias

Uma instância criada no AMP passa a aparecer automaticamente no painel e nas opções dos comandos. Por segurança, ela não entra automaticamente no Idle. Um administrador pode usar `/ampconfig idle-adicionar`.

## Segurança

Tokens e senhas não ficam no TOML nem no repositório. O serviço usa credenciais criptografadas do systemd e um wrapper restrito para executar somente operações suportadas pelo `ampinstmgr`. Consulte a [política de segurança](SECURITY.md) e a [arquitetura](docs/ARCHITECTURE.md).

Antes de publicar logs ou pedir suporte, remova tokens, senhas, endereços privados e identificadores pessoais.

## Desenvolvimento

```bash
go test ./...
go vet ./...
```

Testes que dependem de uma instalação AMP real são opt-in:

```bash
go test -tags=integration ./...
```

Veja [Como contribuir](CONTRIBUTING.md).

## Licença e marcas

O código é disponibilizado sob a [licença MIT](LICENSE). AMP, Discord, Steam e os jogos mencionados pertencem aos respectivos titulares. Logos e outros recursos de terceiros não são relicenciados pela MIT; consulte [Avisos de terceiros](THIRD_PARTY_NOTICES.md).
