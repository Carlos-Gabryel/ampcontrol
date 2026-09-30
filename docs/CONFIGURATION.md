# Configuração

[English](CONFIGURATION.en.md) · **Português (Brasil)**

A configuração não secreta fica em `/etc/ampcontrol/config.toml`. O modelo completo está em [`config/ampcontrol.example.toml`](../config/ampcontrol.example.toml).

| Chave | Uso |
| --- | --- |
| `language` | Idioma global: `pt-BR` ou `en-US`. Controla comandos, painéis, respostas, auditoria, logs e manutenção. |

## Discord

| Chave | Uso |
| --- | --- |
| `guild_id` | Servidor Discord em que os comandos são registrados |
| `notification_channel_id` | Canal do guia, painel e comandos |
| `audit_channel_id` | Canal privado que recebe auditoria |
| `owner_user_id` | Proprietário com acesso ao `/ampconfig` |
| `admin_role_ids` | Cargos adicionais autorizados no `/ampconfig` |
| `restrict_commands_to_channel` | Recusa comandos fora do canal do painel |
| `allow_discord_administrators` | Autoriza qualquer membro com Administrator, além da lista explícita. Padrão `false` quando omitido |
| `notification_ttl_minutes` | Tempo de vida das mensagens transitórias |
| `status_refresh_seconds` | Intervalo de atualização dos cartões |
| `command_user_cooldown_seconds` | Intervalo mínimo por usuário |
| `command_server_cooldown_seconds` | Intervalo mínimo de operações por instância |

Recomendação: mantenha `allow_discord_administrators = false` e liste explicitamente o proprietário e os cargos confiáveis.

## AMP

| Chave | Uso |
| --- | --- |
| `username` | Conta dedicada da API AMP |
| `ads_url` | Endpoint local do ADS |
| `public_url` | URL que o administrador abrirá pelo botão Detalhes |
| `game_server_address` | Endereço público padrão mostrado nos cartões |
| `system_user` | Usuário Linux proprietário da instalação AMP |
| `manager_path` | Caminho de `ampinstmgr` |
| `wrapper_path` | Wrapper privilegiado instalado pelo assistente |
| `sudo_path` | Binário sudo usado pelo serviço |

## Segredos

Novas instalações carregam duas credenciais pela unidade systemd:

| Credencial | Conteúdo |
| --- | --- |
| `discord_token` | Token do bot |
| `amp_password` | Senha da conta dedicada AMP |

Os arquivos criptografados ficam em `/etc/credstore.encrypted/ampcontrol.*` e só são descriptografados para o serviço pelo systemd. Para trocar uma credencial, a opção mais segura é reexecutar o instalador.

O fallback por RCON usa nomes `rcon_<instância>`. Em `config/idle.json`, referencie apenas o nome da credencial em `password_credential`; nunca grave a senha diretamente.

## Idle e detectores

O cadastro do Idle fica em dois arquivos, ambos lidos na inicialização (uma instância não pode aparecer nos dois):

| Arquivo | Quem escreve |
| --- | --- |
| `/var/lib/ampcontrol/data/idle_servers.json` | o instalador (instâncias escolhidas no assistente) e `/ampconfig idle-adicionar` |
| `/var/lib/ampcontrol/config/idle.json` | você, à mão, para ajustes finos como RCON e tempos por instância |

O caminho normal não exige editar JSON: o instalador e `/ampconfig idle-adicionar` cadastram a instância já em modo `active`, com o detector da API do AMP e 15 minutos de espera. `/ampconfig configurar` troca o método de detecção:

| Método no Discord | Detector | Fallback |
| --- | --- | --- |
| `amp` | `amp_players` | — |
| `amp_palworld_rcon` | `amp_players` | `palworld_rcon` |
| `amp_project_zomboid_rcon` | `amp_players` | `project_zomboid_rcon` |

Os métodos com RCON só são aceitos para instâncias que já tenham endereço e credencial RCON configurados (veja [RCON](#rcon)).

### Formato do `idle.json`

Exemplo completo (também em [`config/idle.example.json`](../config/idle.example.json)):

```json
{
  "check_interval_seconds": 30,
  "default_idle_timeout_minutes": 15,
  "servers": [
    {
      "instance": "Palworld01",
      "display_name": "Meu servidor",
      "game": "Palworld",
      "enabled": true,
      "mode": "active",
      "detector": "amp_players",
      "fallback_detector": "palworld_rcon",
      "idle_timeout_minutes": 15,
      "startup_grace_minutes": 5,
      "rcon": {
        "address": "127.0.0.1:25575",
        "password_credential": "rcon_Palworld01"
      }
    }
  ]
}
```

| Chave | Padrão | Descrição |
| --- | --- | --- |
| `check_interval_seconds` | `30` | Intervalo entre verificações |
| `default_idle_timeout_minutes` | `15` | Espera sem jogadores quando a instância não define `idle_timeout_minutes` |
| `instance` | — | Nome da instância no AMP (obrigatório) |
| `display_name` | nome da instância | Nome exibido no Discord |
| `game` | — | Nome do jogo exibido no Discord |
| `enabled` | `false` | Liga o Idle para a instância |
| `mode` | `observe` | `active` para o processo do jogo; `observe` apenas registra no log o que faria |
| `detector` | — | Obrigatório com `enabled: true`: `amp_players`, `palworld_rcon` ou `project_zomboid_rcon` |
| `fallback_detector` | — | Detector consultado quando o primário não consegue contar os jogadores |
| `idle_timeout_minutes` | `default_idle_timeout_minutes` | Espera sem jogadores até parar |
| `startup_grace_minutes` | `5` | Tolerância depois que o jogo inicia |
| `rcon.address` | — | `host:porta` do RCON, obrigatório com detector RCON |
| `rcon.password_credential` | — | Nome da credencial systemd com a senha RCON |

> **Atenção ao `mode`:** sem `"mode": "active"`, a instância fica em `observe` e o Idle **nunca para o servidor**, só registra no log. Chaves desconhecidas ou valores inválidos fazem o serviço recusar o arquivo e **não iniciar**. Guarde uma cópia antes de editar e, depois de reiniciar, confira `systemctl status ampcontrol.service` e `journalctl -u ampcontrol.service -n 20`.

**Instância apagada do AMP:** se uma instância cadastrada some do inventário do AMP por 10 minutos seguidos, o Idle registra um aviso único e deixa de consultá-la, em vez de repetir o erro a cada ciclo. Se ela voltar (backup restaurado, instância recriada com o mesmo nome), o monitoramento recomeça sozinho com a mesma configuração. O `idle.json` nunca é alterado pelo bot: o `/ampconfig diagnostico` lista essas instâncias para você removê-las do arquivo quando quiser. Falhas do AMP ou inventário vazio não contam como instância apagada.

### RCON

O RCON dá uma contagem de jogadores mais confiável para Palworld e Project Zomboid. A senha nunca vai no JSON: ela é uma credencial criptografada do systemd, com nome no formato `rcon_<instância>`. Para configurar numa instalação nova (exemplo com `Palworld01`):

```bash
# 1. Criptografar a senha (lida sem eco e sem ir para o histórico do shell)
read -rsp 'Senha RCON: ' RCON_PASSWORD; echo
printf '%s' "$RCON_PASSWORD" | sudo systemd-creds encrypt --with-key=host --name=rcon_Palworld01 - /etc/credstore.encrypted/ampcontrol.rcon_Palworld01
unset RCON_PASSWORD
sudo chmod 0600 /etc/credstore.encrypted/ampcontrol.rcon_Palworld01

# 2. Entregar a credencial ao serviço
echo 'LoadCredentialEncrypted=rcon_Palworld01:/etc/credstore.encrypted/ampcontrol.rcon_Palworld01' | sudo tee -a /etc/systemd/system/ampcontrol.service.d/credentials.conf
```

3. Em `/var/lib/ampcontrol/config/idle.json`, adicione a instância com o bloco `rcon` do exemplo acima. Se ela já estiver em `data/idle_servers.json` (cadastrada pelo instalador ou pelo Discord), remova-a de lá, porque a mesma instância não pode estar nos dois arquivos.
4. Aplique: `sudo systemctl daemon-reload && sudo systemctl restart ampcontrol.service`, e confira com `/ampconfig diagnostico`.

Ao reexecutar o instalador, responda **sim** quando ele perguntar se deve manter a configuração de Idle existente: ele recria o `credentials.conf` a partir das referências `password_credential` do `idle.json`, e assim as credenciais RCON continuam carregadas.

Falhas ou contagens ambíguas são tratadas de forma conservadora: comandos destrutivos de usuários comuns são bloqueados quando não é possível confirmar com segurança que o servidor está vazio.

## Administração pelo Discord

`/ampconfig` contém:

| Subcomando | Ação |
| --- | --- |
| `diagnostico` | Verifica as integrações |
| `configurar` | Personaliza nome, jogo, endereço, máximo e detector |
| `detalhes` | Mostra a configuração efetiva |
| `restaurar` | Remove as personalizações |
| `ocultar` / `exibir` | Controla a presença no painel e nos comandos |
| `listar` | Lista instâncias ocultas |
| `idle-adicionar` | Cadastra uma instância no Idle automático de 15 minutos |

As preferências do Discord e o estado do Idle ficam no diretório de dados configurado pela instalação. Esses arquivos devem permanecer graváveis apenas pelo usuário do serviço.

## Compatibilidade com variáveis de ambiente

Variáveis legadas continuam aceitas durante a migração e têm precedência sobre valores equivalentes do TOML. Evite esse modo em instalações novas: ele aumenta o risco de segredos aparecerem em arquivos, dumps ou ferramentas de inspeção.
