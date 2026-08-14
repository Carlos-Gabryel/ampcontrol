#!/usr/bin/env python3
"""Conversão estrita da configuração legada do AmpControl.

O utilitário nunca executa o conteúdo do .env. Ele também evita gravar
segredos em arquivos intermediários: o comando env imprime apenas a chave
solicitada, para que o chamador possa encaminhá-la diretamente ao
systemd-creds.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import sys
from pathlib import Path

ENV_NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
CREDENTIAL_NAME = re.compile(r"[^A-Za-z0-9_.-]+")
ENGLISH = os.environ.get("AMPCONTROL_LANGUAGE", "").lower() in {"en", "en-us"}


def msg(portuguese: str, english: str) -> str:
    return english if ENGLISH else portuguese


def parse_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for number, original in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = original.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].lstrip()
        if "=" not in line:
            raise ValueError(msg(f"linha {number} do .env não contém '='", f"line {number} of .env does not contain '='"))
        name, raw_value = line.split("=", 1)
        name = name.strip()
        if not ENV_NAME.fullmatch(name):
            raise ValueError(msg(f"nome inválido na linha {number} do .env: {name!r}", f"invalid name on line {number} of .env: {name!r}"))
        lexer = shlex.shlex(raw_value, posix=True)
        lexer.whitespace_split = True
        lexer.commenters = "#"
        parts = list(lexer)
        if len(parts) > 1:
            raise ValueError(msg(f"valor inválido na linha {number} do .env", f"invalid value on line {number} of .env"))
        values[name] = parts[0] if parts else ""
    return values


def parse_systemd_environment(path: Path) -> dict[str, str]:
    """Parse the escaped output of `systemctl show -p Environment --value`."""
    values: dict[str, str] = {}
    for item in shlex.split(path.read_text(encoding="utf-8"), posix=True):
        if "=" not in item:
            raise ValueError(msg("entrada inválida no ambiente do systemd", "invalid systemd environment entry"))
        name, value = item.split("=", 1)
        if not ENV_NAME.fullmatch(name):
            raise ValueError(msg(f"nome inválido no ambiente do systemd: {name!r}", f"invalid name in systemd environment: {name!r}"))
        values[name] = value
    return values


def credential_name(instance: str) -> str:
    normalized = CREDENTIAL_NAME.sub("_", instance.strip()).strip("_")
    if not normalized:
        raise ValueError(msg("instância RCON sem nome válido", "RCON instance has no valid name"))
    return f"rcon_{normalized}"


def migrate_idle(source: Path, destination: Path, manifest: Path) -> None:
    document = json.loads(source.read_text(encoding="utf-8"))
    servers = document.get("servers")
    if not isinstance(servers, list):
        raise ValueError(msg("a configuração de Idle não contém uma lista servers", "the Idle configuration does not contain a servers list"))

    credentials: list[tuple[str, str]] = []
    seen: set[str] = set()
    for server in servers:
        if not isinstance(server, dict):
            raise ValueError(msg("entrada inválida na lista servers", "invalid entry in the servers list"))
        rcon = server.get("rcon")
        if rcon is None:
            continue
        if not isinstance(rcon, dict):
            raise ValueError(msg("configuração RCON inválida", "invalid RCON configuration"))
        legacy_name = str(rcon.get("password_env", "")).strip()
        current_name = str(rcon.get("password_credential", "")).strip()
        if legacy_name:
            if not ENV_NAME.fullmatch(legacy_name):
                raise ValueError(msg(f"variável RCON inválida: {legacy_name!r}", f"invalid RCON variable: {legacy_name!r}"))
            current_name = credential_name(str(server.get("instance", "")))
            rcon.pop("password_env", None)
            rcon["password_credential"] = current_name
            if current_name not in seen:
                credentials.append((current_name, legacy_name))
                seen.add(current_name)
        elif current_name and current_name not in seen:
            credentials.append((current_name, ""))
            seen.add(current_name)

    destination.write_text(
        json.dumps(document, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    manifest.write_text(
        "".join(f"{credential}\t{environment}\n" for credential, environment in credentials),
        encoding="utf-8",
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)

    env_parser = subparsers.add_parser("env")
    env_parser.add_argument("file", type=Path)
    env_parser.add_argument("name")

    systemd_env_parser = subparsers.add_parser("systemd-env")
    systemd_env_parser.add_argument("file", type=Path)
    systemd_env_parser.add_argument("name")

    idle_parser = subparsers.add_parser("migrate-idle")
    idle_parser.add_argument("source", type=Path)
    idle_parser.add_argument("destination", type=Path)
    idle_parser.add_argument("manifest", type=Path)

    args = parser.parse_args()
    try:
        if args.command in {"env", "systemd-env"}:
            if not ENV_NAME.fullmatch(args.name):
                raise ValueError(msg("nome de variável inválido", "invalid variable name"))
            parser = parse_env if args.command == "env" else parse_systemd_environment
            value = parser(args.file).get(args.name)
            if value is None:
                return 3
            sys.stdout.write(value)
            return 0
        migrate_idle(args.source, args.destination, args.manifest)
        return 0
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"{msg('Erro', 'Error')}: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
