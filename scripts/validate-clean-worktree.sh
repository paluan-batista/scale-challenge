#!/usr/bin/env sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
clean_dir=$(mktemp -d "${TMPDIR:-/tmp}/scale-challenge-clean.XXXXXX")

cleanup() {
	rm -rf "$clean_dir"
}
trap cleanup EXIT HUP INT TERM

printf '%s\n' "Creating clean clone at $clean_dir"
git clone --no-local --quiet "$repository_dir" "$clean_dir"

# Overlay the candidate working tree so uncommitted changes are validated too,
# while retaining the clean clone's repository metadata and checkout behavior.
tar -C "$repository_dir" \
	--exclude=.git \
	--exclude=.env \
	--exclude=postgres-data \
	--exclude=prometheus-data \
	--exclude=grafana-data \
	-cf - . | tar -C "$clean_dir" -xf -

cd "$clean_dir"
make format-check
make vet
make unit
make integration
make race
make acceptance
printf '%s\n' "Clean worktree validation passed"
