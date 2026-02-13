#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

scripts=(
	"$SCRIPT_DIR/build_macos_apps.sh"
	"$ROOT_DIR/scripts/build_linux_apps.sh"
	"$ROOT_DIR/scripts/windows/build_windows_apps.sh"
)

failed=()

for script in "${scripts[@]}"; do
	rel_script="${script#$ROOT_DIR/}"
	echo
	echo "=== Running $rel_script ==="
	if ! "$script"; then
		echo "warning: failed: $script" >&2
		failed+=("$script")
	fi
done

if [ "${#failed[@]}" -gt 0 ]; then
	echo "Completed with failures:" >&2
	for f in "${failed[@]}"; do
		echo "  - $f" >&2
	done
	exit 1
fi

echo
echo "All target build scripts completed."
