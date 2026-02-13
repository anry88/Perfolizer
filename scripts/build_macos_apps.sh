#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"

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

build_icns_from_png() {
	local source_png="$1"
	local app_basename="$2"
	local icns_path="$DIST_DIR/${app_basename}.icns"

	rm -f "$icns_path"
	python3 - "$source_png" "$icns_path" <<'PY'
import sys
from PIL import Image

src = sys.argv[1]
dst = sys.argv[2]
img = Image.open(src).convert("RGBA")

# Normalize alpha to avoid platform-dependent matte artifacts in .icns previews.
flattened = Image.new("RGBA", img.size, (0, 0, 0, 255))
flattened.alpha_composite(img)
flattened.convert("RGB").save(dst)
PY

	echo "$icns_path"
}

create_app_bundle() {
	local app_name="$1"
	local bundle_id="$2"
	local executable_name="$3"
	local executable_path="$4"
	local icon_icns_path="$5"

	local app_dir="$DIST_DIR/${app_name}.app"
	local contents_dir="$app_dir/Contents"

	rm -rf "$app_dir"
	mkdir -p "$contents_dir/MacOS" "$contents_dir/Resources"

	cp "$executable_path" "$contents_dir/MacOS/$executable_name"
	chmod +x "$contents_dir/MacOS/$executable_name"
	cp "$icon_icns_path" "$contents_dir/Resources/AppIcon.icns"

	cat >"$contents_dir/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleExecutable</key>
	<string>${executable_name}</string>
	<key>CFBundleIconFile</key>
	<string>AppIcon</string>
	<key>CFBundleIdentifier</key>
	<string>${bundle_id}</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>${app_name}</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>1.0</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
EOF
}

mkdir -p "$DIST_DIR"

echo "Building darwin/arm64 binaries..."
GOOS=darwin GOARCH=arm64 go build -o "$DIST_DIR/perfolizer-darwin-arm64" ./cmd/perfolizer
GOOS=darwin GOARCH=arm64 go build -o "$DIST_DIR/agent-darwin-arm64" ./cmd/agent

echo "Packaging macOS .app bundles with custom icons..."
ui_icns="$(build_icns_from_png "$ROOT_DIR/assets/icons/perfolizer-ui.png" "PerfolizerUI")"
agent_icns="$(build_icns_from_png "$ROOT_DIR/assets/icons/perfolizer-agent.png" "PerfolizerAgent")"

create_app_bundle \
	"Perfolizer" \
	"com.github.anry88.perfolizer" \
	"perfolizer" \
	"$DIST_DIR/perfolizer-darwin-arm64" \
	"$ui_icns"

create_app_bundle \
	"Perfolizer Agent" \
	"com.github.anry88.perfolizer.agent" \
	"perfolizer-agent" \
	"$DIST_DIR/agent-darwin-arm64" \
	"$agent_icns"

rm -f "$ui_icns" "$agent_icns"

echo "Done:"
echo "  UI app:    $DIST_DIR/Perfolizer.app"
echo "  Agent app: $DIST_DIR/Perfolizer Agent.app"
