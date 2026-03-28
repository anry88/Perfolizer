#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/compose.yaml"
SERVICE_NAME="perfolizer-agent"
ACTION="${1:-up}"
shift $(( $# > 0 ? 1 : 0 ))

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "error: required command not found: $cmd" >&2
		exit 1
	fi
}

require_cmd docker

run_compose() {
	docker compose -f "$COMPOSE_FILE" "$@"
}

case "$ACTION" in
	up)
		run_compose up --build -d "$SERVICE_NAME" "$@"
		echo "Perfolizer agent is available at http://127.0.0.1:9090"
		run_compose ps "$SERVICE_NAME"
		;;
	down)
		run_compose down "$@"
		;;
	logs)
		run_compose logs -f "$SERVICE_NAME" "$@"
		;;
	ps)
		run_compose ps "$SERVICE_NAME" "$@"
		;;
	restart)
		run_compose restart "$SERVICE_NAME" "$@"
		;;
	health)
		require_cmd curl
		curl -fsS http://127.0.0.1:9090/healthz
		;;
	*)
		echo "usage: $0 [up|down|logs|ps|restart|health]" >&2
		exit 1
		;;
esac
