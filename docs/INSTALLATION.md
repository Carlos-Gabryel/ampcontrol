# Instalação no Linux

[English](INSTALLATION.en.md) · **Português (Brasil)**

Este guia instala uma cópia nova do AmpControl sem alterar as instâncias de jogos. Faça primeiro em uma janela de manutenção e mantenha backup da configuração atual.

## 1. Requisitos

- Linux com systemd **250 ou superior** (`systemd-creds` e `LoadCredentialEncrypted`), por exemplo Ubuntu 24.04 ou Debian 12. Ubuntu 22.04 e Debian 11 trazem versões anteriores e não são suportados. Confira com `systemctl --version`;
- AMP instalado na mesma máquina;
- usuário com `sudo`;
- Git, somente ao instalar a partir do código-fonte;
- Python 3 (leitura segura da configuração legada);
- `curl`, `tar` e `sha256sum` para atualizações;
- Go 1.26.9 ou superior, somente ao instalar a partir do código-fonte (o pacote de release e `--binary` dispensam Go);
- um servidor Discord e permissão para adicionar aplicações.

O instalador precisa de terminal interativo e deve ser executado na raiz do repositório ou do pacote de release extraído.

## 2. Criar a aplicação Discord

1. No [Discord Developer Portal](https://discord.com/developers/applications), crie uma aplicação e adicione um bot.
2. O token vem de **Bot → Reset Token**, não do Client Secret nem da Public Key; o instalador rejeita outros valores. Guarde o token em um gerenciador de senhas. Nunca o coloque em issue, commit, captura de tela ou arquivo TOML.
3. Em **Installation**, habilite instalação no servidor (Guild Install).
4. Gere o link com os escopos `bot` e `applications.commands`.
5. Conceda ao bot somente estas permissões no canal do painel: View Channel, Send Messages, Embed Links, Attach Files, Read Message History e Manage Messages.
6. Não conceda Administrator ao bot. O AmpControl não precisa dessa permissão global.

O instalador (`scripts/install.sh`) mostra o link de convite pronto, montado a partir do token, com estes escopos e permissões e o servidor já selecionado, depois de pedir o token e o ID do servidor; por isso os passos 4 e 5 podem ser pulados. O link é só texto: abra-o em qualquer dispositivo logado no Discord (PC ou celular), o que importa em servidores sem interface gráfica.

O projeto usa interações e não lê mensagens comuns, portanto não requer Message Content Intent.

Ative o Developer Mode no cliente Discord e copie:

- ID do servidor;
- ID do canal do painel e comandos;
- ID de um canal privado de auditoria;
- ID do usuário proprietário;
- IDs dos cargos administrativos, se existirem.

Para que `/ampconfig` seja exibido, o proprietário e os cargos administrativos devem ter a permissão Administrator no Discord. O AmpControl ainda valida os IDs internamente antes de executar a ação.

## 3. Criar uma conta AMP dedicada

Crie no AMP um usuário exclusivo para o bot, com acesso às instâncias que o bot vai gerenciar. Pela API, o AmpControl só chama:

| Método da API | Uso |
| --- | --- |
| `Core.Login` | autenticação no ADS e em cada instância |
| `ADSModule.GetInstances` | inventário das instâncias |
| `Core.GetStatus`, `Core.GetModuleInfo` | estado, jogadores e informações do jogo |
| `Core.GetUpdates` | leitura do console, usada somente para exibir o código de convite do Valheim |
| `Core.Start`, `Core.Stop` | iniciar e parar o processo do jogo (inclusive pelo Idle) |

Conceda somente as permissões correspondentes a essas ações. Para exibir o código de convite do Valheim, o usuário também precisa de permissão para ver o console dessas instâncias; sem ela, a linha do código simplesmente não aparece. O console é lido só enquanto o código é desconhecido e, depois, no máximo a cada 5 minutos. Iniciar, parar, reiniciar e atualizar a **instância** inteira não passam pela API: são feitos pelo `ampinstmgr`, através do wrapper restrito instalado com o `sudo`.

Valide o login diretamente no AMP antes de executar o instalador. Não reutilize a conta principal do administrador.

## 4. Executar o instalador

### Com um comando (recomendado)

```bash
curl -fsSL https://github.com/Carlos-Gabryel/ampcontrol/releases/latest/download/get.sh | sudo bash
```

Para repassar opções ao instalador ou fixar uma versão:

```bash
curl -fsSL https://github.com/Carlos-Gabryel/ampcontrol/releases/latest/download/get.sh | sudo bash -s -- --language en-US
curl -fsSL https://github.com/Carlos-Gabryel/ampcontrol/releases/latest/download/get.sh | sudo env AMPCONTROL_VERSION=v1.0.8 bash
```

O `get.sh` baixa a release mais recente para a arquitetura do servidor (amd64 ou arm64), confere o SHA-256 contra o `checksums.txt`, executa o instalador do pacote com o terminal como entrada (as perguntas funcionam mesmo com `curl | bash`) e apaga os arquivos temporários. Não precisa de Go.

Quem prefere ler antes de executar pode baixar o script (`curl -fsSLO https://github.com/Carlos-Gabryel/ampcontrol/releases/latest/download/get.sh`), inspecioná-lo e rodar `sudo bash get.sh`, ou usar o método manual abaixo.

### Pelo pacote de release (não precisa de Go)

Escolha a versão mais recente na página de [Releases](https://github.com/Carlos-Gabryel/ampcontrol/releases) e ajuste `VERSION` e `ARCH`:

```bash
VERSION=v1.0.7   # use a versão mais recente da página de Releases
ARCH=amd64       # ou arm64
curl -fLO "https://github.com/Carlos-Gabryel/ampcontrol/releases/download/$VERSION/ampcontrol_${VERSION}_linux_${ARCH}.tar.gz"
curl -fLO "https://github.com/Carlos-Gabryel/ampcontrol/releases/download/$VERSION/checksums.txt"
sha256sum -c --ignore-missing checksums.txt
tar -xzf "ampcontrol_${VERSION}_linux_${ARCH}.tar.gz"
cd "ampcontrol_${VERSION}_linux_${ARCH}"
sudo ./scripts/install.sh
```

O pacote já traz o binário compilado e o instalador o usa automaticamente.

### A partir do código-fonte (requer Go 1.26.9+)

```bash
git clone https://github.com/Carlos-Gabryel/ampcontrol.git
cd ampcontrol
sudo ./scripts/install.sh
```

A primeira pergunta escolhe o idioma de toda a instalação. Também é possível defini-lo diretamente com `--language pt-BR` ou `--language en-US`.

Alternativas:

```bash
sudo ./scripts/install.sh --binary /caminho/para/ampcontrol
sudo ./scripts/install.sh --no-start
```

O assistente:

1. detecta `ampinstmgr`, usuário Linux do AMP e diretório das instâncias;
2. valida que consegue ler o inventário como o usuário do AMP;
3. oferece ativar Idle de 15 minutos em todas, nenhuma ou uma seleção de instâncias;
4. solicita IDs e política de acesso do Discord;
5. solicita a conta da API, URLs e endereços públicos;
6. mostra um resumo antes de alterar o sistema;
7. instala binário, configuração, wrapper, regra sudoers e unidade systemd;
8. criptografa o token Discord e a senha AMP com a chave do host;
9. valida e inicia o serviço, salvo com `--no-start`.

Em reinstalações, o assistente oferece preservar o cadastro de Idle e cria backups antes de substituir arquivos gerenciados.

## 5. Verificar

```bash
sudo systemctl status ampcontrol --no-pager
sudo journalctl -u ampcontrol -n 100 --no-pager
```

No Discord:

1. confira se o guia e os cartões aparecem no canal escolhido;
2. execute `/amp status` como usuário comum;
3. execute `/ampconfig diagnostico` como proprietário;
4. confirme que `/amp` é recusado em outros canais se a restrição estiver ativa;
5. confirme que o canal de auditoria recebeu os eventos.

## 6. Rede e URL do AMP

`amp.ads_url` deve apontar para o ADS acessível pelo serviço, normalmente `http://127.0.0.1:8080`. `amp.public_url` é opcional e deve ser uma URL que o navegador do administrador consiga alcançar. Não exponha o painel AMP diretamente à internet sem TLS, autenticação e uma política de rede apropriada.

## 7. Atualizar ou recuperar

Para instalar a release estável mais recente:

```bash
sudo ampcontrol-maintenance update
```

Para escolher uma versão ou consultar o estado:

```bash
sudo ampcontrol-maintenance update v1.2.3
sudo ampcontrol-maintenance status
```

O atualizador baixa a release oficial, confere o SHA-256 publicado, valida a configuração atual em uma unidade systemd temporária e só então substitui o binário. Ele preserva TOML, credenciais, cadastro de Idle e demais estados locais. Uma falha de instalação, ou um serviço que não permanece ativo nos primeiros 15 segundos, aciona rollback automático.

Parar ou reiniciar o serviço (inclusive durante uma atualização) não interrompe operações em andamento, como um `/amp atualizar`: o bot recusa novas operações e espera essas operações terminarem, por até 15 minutos, antes de sair.

Para restaurar manualmente a versão anterior:

```bash
sudo ampcontrol-maintenance rollback
```

Os backups ficam em `/var/backups/ampcontrol`. Não remova o último backup antes de validar Discord, painel, Idle e RCON.

### Desinstalar

```bash
sudo ampcontrol-maintenance uninstall          # mantém configuração, dados e usuário
sudo ampcontrol-maintenance uninstall --purge  # remove tudo do AmpControl
```

O comando lista o que será removido e o que será mantido e pede para digitar `remover` (use `--yes` para pular a pergunta). Sem `--purge`, ele para e desabilita o serviço e remove a unit e seus drop-ins, a regra do sudoers, o wrapper `ampcontrol-amp`, `/usr/lib/ampcontrol` e o próprio `ampcontrol-maintenance`. Ficam `/etc/ampcontrol`, as credenciais criptografadas (`/etc/credstore.encrypted/ampcontrol.*`), `/var/lib/ampcontrol`, `/var/backups/ampcontrol` e o usuário `ampcontrol`; reinstalar reaproveita esses dados. Com `--purge`, tudo isso também é removido, inclusive o usuário e o grupo `ampcontrol`.

O AMP, o usuário `amp`, as instâncias de jogo e o `ampinstmgr` nunca são tocados.

## 8. Migrar uma instalação antiga

Instalações antigas em `/opt/ampcontrol`, baseadas em `.env`, devem usar o migrador transacional:

```bash
git clone https://github.com/Carlos-Gabryel/ampcontrol.git
cd ampcontrol
sudo ./scripts/migration-preflight.sh /opt/ampcontrol
sudo ./scripts/migrate-legacy.sh /opt/ampcontrol
```

O primeiro comando é somente leitura e apresenta um relatório sem exibir valores secretos. Corrija todas as linhas `FALHA` antes de executar a migração.

O migrador:

1. cria um snapshot dos arquivos gerenciados e registra o estado do serviço;
2. lê o `.env` sem executá-lo;
3. converte token Discord, senha AMP e senhas RCON para `systemd-creds`;
4. preserva `config/idle.json` e todo o conteúdo persistente de `data/`;
5. solicita apenas os dados ausentes, como o ID do servidor Discord;
6. reinicia o serviço com o layout compartilhável;
7. restaura automaticamente a instalação anterior se o novo serviço falhar.

A pasta legada e o snapshot permanecem intactos após uma migração bem-sucedida. Remova-os somente depois da validação funcional e de um backup externo.

O antigo modo baseado em `.env` continua aceito para migração, mas novas instalações devem usar TOML e credenciais systemd.
