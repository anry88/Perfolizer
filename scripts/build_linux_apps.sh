#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist/linux"

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "error: required command not found: $cmd" >&2
		exit 1
	fi
}

require_cmd go
require_cmd tar

mkdir -p "$ROOT_DIR/.cache/go-build"
export GOCACHE="$ROOT_DIR/.cache/go-build"

mkdir -p "$DIST_DIR"

build_linux_bundle() {
	local app_dir_name="$1"
	local bin_name="$2"
	local source_pkg="$3"
	local icon_png="$4"
	local app_title="$5"
	local desktop_name="$6"
	local comment="$7"
	local archive_name="$8"
	local archive_path="$DIST_DIR/$archive_name"

	local app_dir="$DIST_DIR/$app_dir_name"
	rm -rf "$app_dir"
	rm -f "$archive_path"
	mkdir -p "$app_dir/bin" "$app_dir/icons"

	if ! GOOS=linux GOARCH=amd64 go build -o "$app_dir/bin/$bin_name" "$source_pkg"; then
		echo "warning: failed to build $source_pkg for linux/amd64" >&2
		echo "warning: UI build usually requires Linux host toolchain for Fyne/CGO" >&2
		rm -rf "$app_dir"
		return 1
	fi
	chmod +x "$app_dir/bin/$bin_name"
	cp "$icon_png" "$app_dir/icons/app-icon.png"

	cat >"$app_dir/$desktop_name" <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=$app_title
Comment=$comment
Exec=\${APPDIR}/bin/$bin_name
Icon=\${APPDIR}/icons/app-icon.png
Terminal=false
Categories=Development;Utility;
EOF

	tar -C "$DIST_DIR" -czf "$archive_path" "$app_dir_name"
	return 0
}

echo "Building Linux bundles..."

ui_built=false
if build_linux_bundle \
	"Perfolizer-linux-amd64" \
	"perfolizer" \
	"./cmd/perfolizer" \
	"$ROOT_DIR/assets/icons/perfolizer-ui.png" \
	"Perfolizer" \
	"perfolizer.desktop" \
	"Perfolizer UI application" \
	"Perfolizer-linux-amd64.tar.gz"; then
	ui_built=true
else
	echo "UI Linux bundle skipped in this environment."
fi

agent_built=false
if build_linux_bundle \
	"Perfolizer-Agent-linux-amd64" \
	"perfolizer-agent" \
	"./cmd/agent" \
	"$ROOT_DIR/assets/icons/perfolizer-agent.png" \
	"Perfolizer Agent" \
	"perfolizer-agent.desktop" \
	"Perfolizer Agent service" \
	"Perfolizer-Agent-linux-amd64.tar.gz"; then
	agent_built=true
else
	echo "Agent Linux bundle skipped in this environment."
fi

echo "Done:"
if [ "$ui_built" = true ]; then
	echo "  $DIST_DIR/Perfolizer-linux-amd64"
fi
if [ "$agent_built" = true ]; then
	echo "  $DIST_DIR/Perfolizer-Agent-linux-amd64"
fi
echo "  archives:"
if [ "$ui_built" = true ]; then
	echo "    $DIST_DIR/Perfolizer-linux-amd64.tar.gz"
fi
if [ "$agent_built" = true ]; then
	echo "    $DIST_DIR/Perfolizer-Agent-linux-amd64.tar.gz"
fi
