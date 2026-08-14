# Política de segurança

[English](SECURITY.en.md) · **Português (Brasil)**

## Versões suportadas

Até a primeira versão estável, correções de segurança são aplicadas somente à versão mais recente da branch principal. Instalações antigas devem atualizar antes de solicitar suporte.

## Reportar uma vulnerabilidade

Não publique tokens, senhas, endereços internos, IDs pessoais ou detalhes exploráveis em uma issue pública.

Use **Security > Report a vulnerability** no repositório GitHub para abrir um aviso privado. Inclua versão/commit, impacto, condições para reprodução e uma prova mínima sem dados reais. Se o recurso de avisos privados não estiver disponível, abra uma issue pública sem detalhes sensíveis solicitando um canal privado.

## Modelo de segurança

O AmpControl protege contra uso indevido por membros comuns do Discord e reduz a exposição de credenciais locais. Ele não protege uma máquina cujo root, conta AMP administrativa ou token Discord já tenham sido comprometidos.

Práticas obrigatórias:

- use um bot Discord exclusivo;
- use uma conta AMP exclusiva e com privilégio mínimo;
- mantenha `/ampconfig` limitado ao proprietário e a cargos confiáveis;
- mantenha o canal de auditoria privado;
- não exponha ADS ou RCON diretamente à internet;
- revise logs e backups antes de compartilhá-los;
- rotacione imediatamente qualquer segredo que tenha aparecido em texto claro.

Credenciais criptografadas com a chave do host protegem dados em repouso contra cópia casual, mas root na própria máquina ainda pode acessá-las. Isso é uma propriedade esperada do modelo do systemd.
