#!/usr/bin/env bash
# Pull merch-backend from git and rebuild the production stack.
#
# From your laptop (after git push):
#   ./deploy/update.sh
#
# On the VPS:
#   /opt/merch/deploy/update.sh
set -euo pipefail

SSH_HOST="${MERCH_SSH_HOST:-merch}"
REMOTE_ROOT="/opt/merch"
HEALTH_URL="${MERCH_HEALTH_URL:-https://api.merch-wallet.ru/healthz}"
BRANCH="${MERCH_BRANCH:-main}"

on_vps() {
	[[ -d "${REMOTE_ROOT}/.git" ]] && [[ -f "${REMOTE_ROOT}/deploy/docker-compose.prod.yml" ]]
}

deploy_on_vps() {
	cd "${REMOTE_ROOT}"

	echo "==> git fetch ${BRANCH}"
	git fetch origin "${BRANCH}"

	local ahead
	ahead="$(git rev-list --count HEAD.."origin/${BRANCH}" 2>/dev/null || echo 0)"
	echo "==> ${ahead} commit(s) behind origin/${BRANCH} ($(git rev-parse --short HEAD) → $(git rev-parse --short "origin/${BRANCH}"))"

	if ! git diff --quiet || ! git diff --cached --quiet; then
		echo "==> local uncommitted changes on VPS — stashing"
		git stash push -m "deploy/update.sh $(date -u +%FT%TZ)"
	fi

	git checkout "${BRANCH}"
	git pull --ff-only origin "${BRANCH}"

	echo "==> docker compose up --build (1 GB RAM: this can take several minutes)"
	export GOMAXPROCS="${GOMAXPROCS:-1}"
	export DOCKER_BUILDKIT=1
	cd "${REMOTE_ROOT}/deploy"
	docker compose -f docker-compose.prod.yml up -d --build

	echo "==> compose ps"
	docker compose -f docker-compose.prod.yml ps

	echo "==> health ${HEALTH_URL}"
	local i
	for i in 1 2 3 4 5 6 7 8 9 10; do
		if curl -fsS --max-time 5 "${HEALTH_URL}"; then
			echo
			echo "==> deploy ok"
			return 0
		fi
		sleep 2
	done
	echo "==> health check failed — last api logs:" >&2
	docker compose -f docker-compose.prod.yml logs --tail=40 api >&2
	exit 1
}

if on_vps; then
	deploy_on_vps
	exit 0
fi

if [[ ! -t 0 && -z "${BASH_SOURCE[0]:-}" ]]; then
	# piped over ssh: bash -s
	deploy_on_vps
	exit 0
fi

echo "==> running on ${SSH_HOST}"
exec ssh -o ServerAliveInterval=15 "${SSH_HOST}" "bash -s" < "$0"
