#!/usr/bin/env bash
# Shared helpers for running Go commands from bash-based drill scripts.
set -euo pipefail

extract_go_version_parts() {
	local version_output="$1"
	if [[ "$version_output" =~ go([0-9]+)\.([0-9]+) ]]; then
		printf "%s %s\n" "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"
		return 0
	fi
	return 1
}

go_version_satisfies() {
	local version_output="$1"
	local min_minor="$2"
	local major minor
	if ! read -r major minor < <(extract_go_version_parts "$version_output"); then
		return 1
	fi
	[[ "$major" -gt 1 || "$minor" -ge "$min_minor" ]]
}

to_windows_path() {
	local path="$1"
	if command -v wslpath >/dev/null 2>&1; then
		wslpath -w "$path"
	elif command -v cygpath >/dev/null 2>&1; then
		cygpath -w "$path"
	else
		printf "%s\n" "$path"
	fi
}

quote_ps() {
	local value="$1"
	value="${value//\'/\'\'}"
	printf "'%s'" "$value"
}

init_go_runner() {
	local project_root="$1"
	local min_minor="${2:-23}"

	GO_PROJECT_ROOT="$project_root"
	GO_RUNNER="native"
	GO_RUNNER_DESC=""
	GO_PROJECT_ROOT_WIN=""

	local native_version=""
	if native_version="$(go version 2>/dev/null)"; then
		if go_version_satisfies "$native_version" "$min_minor"; then
			GO_RUNNER_DESC="$native_version"
			return 0
		fi
		GO_RUNNER_DESC="$native_version (too old for this drill)"
	else
		GO_RUNNER_DESC="native go unavailable"
	fi

	if ! command -v powershell.exe >/dev/null 2>&1; then
		echo "ERROR: native go unavailable or too old, and powershell.exe is not available for fallback" >&2
		return 1
	fi

	local ps_version=""
	if ! ps_version="$(powershell.exe -NoProfile -Command "go version" 2>/dev/null | tr -d '\r')"; then
		echo "ERROR: native go unavailable or too old, and powershell.exe fallback could not find a usable Go toolchain" >&2
		return 1
	fi

	if ! go_version_satisfies "$ps_version" "$min_minor"; then
		echo "ERROR: no Go >= 1.${min_minor} toolchain found. Native: ${GO_RUNNER_DESC}; PowerShell: ${ps_version}" >&2
		return 1
	fi

	GO_RUNNER="powershell"
	GO_RUNNER_DESC="$ps_version"
	GO_PROJECT_ROOT_WIN="$(to_windows_path "$project_root")"
}

log_go_runner() {
	echo "Go runner: $GO_RUNNER_DESC"
}

run_go_cmd() {
	local rel_dir="$1"
	shift

	if [[ "$GO_RUNNER" == "native" ]]; then
		(
			cd "$GO_PROJECT_ROOT/$rel_dir"
			go "$@"
		)
		return
	fi

	local ps_dir="${GO_PROJECT_ROOT_WIN}\\${rel_dir//\//\\}"
	local arg_string=""
	local arg
	for arg in "$@"; do
		if [[ -n "$arg_string" ]]; then
			arg_string+=" "
		fi
		arg_string+="$(quote_ps "$arg")"
	done

	powershell.exe -NoProfile -Command "\$ErrorActionPreference='Stop'; Set-Location -LiteralPath $(quote_ps "$ps_dir"); & go $arg_string"
}
