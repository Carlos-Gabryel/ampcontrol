# Instalação no Linux

Este guia instala uma cópia nova do AmpControl sem alterar as instâncias de jogos. Faça primeiro em uma janela de manutenção e mantenha backup da configuração atual.

## 1. Requisitos

- Linux com systemd e `systemd-creds`;
- AMP instalado na mesma máquina;
- usuário com `sudo`;
- Git;
- Python 3 (leitura segura da configuração legada);
- `curl`, `tar` e `sha256sum` para atualizações;
- Go 1.26.6 ou superior, salvo se usar `--binary`;
- um servidor Discord e permissão para adicionar aplicações.

O instalador precisa de terminal interativo e deve ser executado na raiz do repositório.

## 2. Criar a aplicação Discord

1. No [Discord Developer Portal](https://discord.com/developers/applications), crie uma aplicação e adicione um bot.
2. Guarde o token em um gerenciador de senhas. Nunca o coloque em issue, commit, captura de tela ou arquivo TOML.
3. Em **Installation**, habilite instalação no servidor (Guild Install).
4. Gere o link com os escopos `bot` e `applications.commands`.
5. Conceda ao bot somente estas permissões no canal do painel: View Channel, Send Messages, Embed Links, Attach Files, Read Message History e Manage Messages.
6. Não conceda Administrator ao bot. O AmpControl não precisa dessa permissão global.

O projeto usa interações e não lê mensagens comuns, portanto não requer Message Content Intent.

Ative o Developer Mode no cliente Discord e copie:

- ID do servidor;
- ID do canal do painel e comandos;
- ID de um canal privado de auditoria;
- ID do usuário proprietário;
- IDs dos cargos administrativos, se existirem.

Para que `/ampconfig` seja exibido, o proprietário e os cargos administrativos devem ter a permissão Administrator no Discord. O AmpControl ainda valida os IDs internamente antes de executar a ação.

## 3. Criar uma conta AMP dedicada

Crie no AMP um usuário exclusivo para o bot. Conceda somente as permissões necessárias para listar e consultar instâncias e executar as operações que você pretende disponibilizar: iniciar, parar, reiniciar, desligar e atualizar.

Valide o login diretamente no AMP antes de executar o instalador. Não reutilize a conta principal do administrador.

## 4. Executar o instalador

```bash
git clone https://github.com/Carlos-Gabryel/ampcontrol.git
cd ampcontrol
sudo ./scripts/install.sh
```

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

O atualizador baixa a release oficial, confere o SHA-256 publicado, valida a configuração atual em uma unidade systemd temporária e só então substitui o binário. Ele preserva TOML, credenciais, cadastro de Idle e demais estados locais. Uma falha de instalação ou inicialização aciona rollback automático.

Para restaurar manualmente a versão anterior:

```bash
sudo ampcontrol-maintenance rollback
```

Os backups ficam em `/var/backups/ampcontrol`. Não remova o último backup antes de validar Discord, painel, Idle e RCON.

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
