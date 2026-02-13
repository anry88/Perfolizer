#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="$ROOT_DIR/dist/windows"

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "error: required command not found: $cmd" >&2
		exit 1
	fi
}

require_cmd go
require_cmd python3

mkdir -p "$ROOT_DIR/.cache/go-build"
export GOCACHE="$ROOT_DIR/.cache/go-build"

mkdir -p "$DIST_DIR"

build_windows_bundle() {
	local app_dir_name="$1"
	local exe_name="$2"
	local source_pkg="$3"
	local icon_png="$4"
	local icon_ico_name="$5"
	local archive_name="$6"
	local shortcut_ps1_name="$7"
	local shortcut_name="$8"
	local archive_path="$DIST_DIR/$archive_name"

	local app_dir="$DIST_DIR/$app_dir_name"
	rm -rf "$app_dir"
	rm -f "$archive_path"
	mkdir -p "$app_dir/icons"

	if ! GOOS=windows GOARCH=amd64 go build -o "$app_dir/$exe_name" "$source_pkg"; then
		echo "warning: failed to build $source_pkg for windows/amd64" >&2
		echo "warning: UI build may require Windows-native toolchain for Fyne/CGO" >&2
		rm -rf "$app_dir"
		return 1
	fi

	python3 - "$icon_png" "$app_dir/icons/$icon_ico_name" <<'PY'
import sys
from PIL import Image

src = sys.argv[1]
dst = sys.argv[2]
img = Image.open(src).convert("RGBA")
img.save(dst, sizes=[(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
PY

	cat >"$app_dir/$shortcut_ps1_name" <<EOF
\$WshShell = New-Object -ComObject WScript.Shell
\$DesktopPath = [Environment]::GetFolderPath('Desktop')
\$Shortcut = \$WshShell.CreateShortcut((Join-Path \$DesktopPath '$shortcut_name'))
\$Shortcut.TargetPath = (Join-Path \$PSScriptRoot '$exe_name')
\$Shortcut.IconLocation = (Join-Path \$PSScriptRoot 'icons\\\\$icon_ico_name')
\$Shortcut.WorkingDirectory = \$PSScriptRoot
\$Shortcut.Save()
Write-Host "Shortcut created: \$DesktopPath\\$shortcut_name"
EOF

	(
		cd "$DIST_DIR"
		python3 -m zipfile -c "$archive_name" "$app_dir_name"
	)
	return 0
}

echo "Building Windows bundles..."

ui_built=false
if build_windows_bundle \
	"Perfolizer-windows-amd64" \
	"perfolizer.exe" \
	"./cmd/perfolizer" \
	"$ROOT_DIR/assets/icons/perfolizer-ui.png" \
	"perfolizer-ui.ico" \
	"Perfolizer-windows-amd64.zip" \
	"create-desktop-shortcut.ps1" \
	"Perfolizer.lnk"; then
	ui_built=true
else
	echo "UI Windows bundle skipped in this environment."
fi

agent_built=false
if build_windows_bundle \
	"Perfolizer-Agent-windows-amd64" \
	"perfolizer-agent.exe" \
	"./cmd/agent" \
	"$ROOT_DIR/assets/icons/perfolizer-agent.png" \
	"perfolizer-agent.ico" \
	"Perfolizer-Agent-windows-amd64.zip" \
	"create-desktop-shortcut.ps1" \
	"Perfolizer Agent.lnk"; then
	agent_built=true
else
	echo "Agent Windows bundle skipped in this environment."
fi

echo "Done:"
if [ "$ui_built" = true ]; then
	echo "  $DIST_DIR/Perfolizer-windows-amd64"
fi
if [ "$agent_built" = true ]; then
	echo "  $DIST_DIR/Perfolizer-Agent-windows-amd64"
fi
echo "  archives:"
if [ "$ui_built" = true ]; then
	echo "    $DIST_DIR/Perfolizer-windows-amd64.zip"
fi
if [ "$agent_built" = true ]; then
	echo "    $DIST_DIR/Perfolizer-Agent-windows-amd64.zip"
fi
