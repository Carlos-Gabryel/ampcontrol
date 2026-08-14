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
| `allow_discord_administrators` | Autoriza qualquer membro com Administrator, além da lista explícita |
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

`/var/lib/ampcontrol/config/idle.json` contém o cadastro por instância. Exemplo:

```json
{
  "check_interval_seconds": 30,
  "servers": [
    {
      "instance": "Palworld01",
      "enabled": true,
      "idle_after_minutes": 15,
      "player_source": "amp_palworld_rcon",
      "rcon": {
        "address": "127.0.0.1:25575",
        "password_credential": "rcon_Palworld01"
      }
    }
  ]
}
```

Detectores disponíveis:

- `amp`: usa a telemetria da API AMP;
- `amp_palworld_rcon`: prioriza a detecção RCON de Palworld;
- `amp_project_zomboid_rcon`: prioriza a detecção RCON de Project Zomboid.

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
