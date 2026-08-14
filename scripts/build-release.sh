#!/usr/bin/env bash

set -Eeuo pipefail

readonly SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly PROJECT_DIRECTORY="$(cd -- "$SCRIPT_DIRECTORY/.." && pwd -P)"
readonly VERSION="${1:-dev}"
readonly DIST_DIRECTORY="${AMPCONTROL_DIST_DIRECTORY:-$PROJECT_DIRECTORY/dist}"

[[ "$VERSION" =~ ^[A-Za-z0-9._-]+$ ]] || {
    printf 'Versão inválida: %s\n' "$VERSION" >&2
    exit 2
}
command -v go >/dev/null 2>&1 || {
    printf 'Go não encontrado.\n' >&2
    exit 1
}

mkdir -p "$DIST_DIRECTORY"
if [[ -n "$(find "$DIST_DIRECTORY" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
    printf 'O diretório de saída precisa estar vazio: %s\n' "$DIST_DIRECTORY" >&2
    exit 1
fi

for architecture in amd64 arm64; do
    package_name="ampcontrol_${VERSION}_linux_${architecture}"
    package_directory="$DIST_DIRECTORY/$package_name"
    mkdir -p "$package_directory/config" "$package_directory/packaging/systemd" "$package_directory/scripts"

    (
        cd "$PROJECT_DIRECTORY"
        CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" \
            go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$package_directory/ampcontrol" ./cmd/ampcontrol
    )
    install -m 0644 "$PROJECT_DIRECTORY/README.md" "$PROJECT_DIRECTORY/LICENSE" "$PROJECT_DIRECTORY/SECURITY.md" "$package_directory/"
    install -m 0644 "$PROJECT_DIRECTORY/config/ampcontrol.example.toml" "$PROJECT_DIRECTORY/config/idle.json" "$package_directory/config/"
    install -m 0644 "$PROJECT_DIRECTORY/packaging/systemd/ampcontrol.service" "$package_directory/packaging/systemd/"
    install -m 0755 \
        "$PROJECT_DIRECTORY/scripts/install.sh" \
        "$PROJECT_DIRECTORY/scripts/migrate-legacy.sh" \
        "$PROJECT_DIRECTORY/scripts/ampcontrol-amp" \
        "$PROJECT_DIRECTORY/scripts/ampcontrol-maintenance" \
        "$PROJECT_DIRECTORY/scripts/legacy_config.py" \
        "$package_directory/scripts/"

    tar -C "$DIST_DIRECTORY" -czf "$DIST_DIRECTORY/$package_name.tar.gz" "$package_name"
    rm -rf -- "$package_directory"
done

(
    cd "$DIST_DIRECTORY"
    sha256sum ./*.tar.gz >checksums.txt
)
printf 'Artefatos criados em %s\n' "$DIST_DIRECTORY"
