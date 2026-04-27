#!/usr/bin/env bash
# Shared helpers for running Python commands from bash-based drill scripts.
set -euo pipefail

extract_python_version_parts() {
	local version_output="$1"
	if [[ "$version_output" =~ Python[[:space:]]+([0-9]+)\.([0-9]+) ]]; then
		printf "%s %s\n" "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"
		return 0
	fi
	return 1
}

python_version_satisfies() {
	local version_output="$1"
	local min_minor="$2"
	local major minor
	if ! read -r major minor < <(extract_python_version_parts "$version_output"); then
		return 1
	fi
	[[ "$major" -gt 3 || "$minor" -ge "$min_minor" ]]
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

init_python_runner() {
	local project_root="$1"
	local min_minor="${2:-10}"

	PYTHON_PROJECT_ROOT="$project_root"
	PYTHON_MIN_MINOR="$min_minor"
	PYTHON_PROJECT_ROOT_WIN="$(to_windows_path "$project_root")"
	PYTHON_DEPS_DIR="$project_root/artifacts/dpi-audit/.pydeps"
	PYTHON_DEPS_DIR_WIN="$(to_windows_path "$PYTHON_DEPS_DIR")"
	PYTHON_RUNNER="native"
	PYTHON_RUNNER_DESC=""
	PYTHON_BIN=""

	local native_bin native_version
	for native_bin in python3 python; do
		if native_version="$($native_bin --version 2>/dev/null)"; then
			if python_version_satisfies "$native_version" "$min_minor"; then
				PYTHON_BIN="$native_bin"
				PYTHON_RUNNER_DESC="$native_version"
				return 0
			fi
			PYTHON_RUNNER_DESC="$native_version (too old for this drill)"
		fi
	done

	if activate_powershell_python "$min_minor"; then
		return 0
	fi

	echo "ERROR: no usable Python >= 3.${min_minor} toolchain found" >&2
	return 1
}

log_python_runner() {
	echo "Python runner: $PYTHON_RUNNER_DESC"
}

run_python_inline() {
	local code="$1"
	if [[ "$PYTHON_RUNNER" == "native" ]]; then
		(
			cd "$PYTHON_PROJECT_ROOT"
			if [[ -d "$PYTHON_DEPS_DIR" ]]; then
				PYTHONPATH="$PYTHON_DEPS_DIR${PYTHONPATH:+:$PYTHONPATH}" "$PYTHON_BIN" -c "$code"
			else
				"$PYTHON_BIN" -c "$code"
			fi
		)
		return
	fi

	local pythonpath_assign=""
	if [[ -d "$PYTHON_DEPS_DIR" ]]; then
		pythonpath_assign="if (\$env:PYTHONPATH) { \$env:PYTHONPATH = $(quote_ps "$PYTHON_DEPS_DIR_WIN") + [IO.Path]::PathSeparator + \$env:PYTHONPATH } else { \$env:PYTHONPATH = $(quote_ps "$PYTHON_DEPS_DIR_WIN") }; "
	fi

	powershell.exe -NoProfile -Command "\$ErrorActionPreference='Stop'; Set-Location -LiteralPath $(quote_ps "$PYTHON_PROJECT_ROOT_WIN"); ${pythonpath_assign}if (Get-Command python -ErrorAction SilentlyContinue) { & python -c $(quote_ps "$code") } else { & py -3 -c $(quote_ps "$code") }"
}

python_has_modules() {
	local module_list=("$@")
	local code=""
	local mod
	for mod in "${module_list[@]}"; do
		code+="import ${mod}; "
	done
	code+="print('python-import-ok')"
	run_python_inline "$code" >/dev/null 2>&1
}

activate_powershell_python() {
	local min_minor="${1:-${PYTHON_MIN_MINOR:-10}}"
	if ! command -v powershell.exe >/dev/null 2>&1; then
		return 1
	fi

	local ps_version=""
	if ! ps_version="$(powershell.exe -NoProfile -Command "\$ErrorActionPreference='Stop'; if (Get-Command python -ErrorAction SilentlyContinue) { python --version } elseif (Get-Command py -ErrorAction SilentlyContinue) { py -3 --version } else { throw 'python unavailable' }" 2>/dev/null | tr -d '\r')"; then
		return 1
	fi

	if ! python_version_satisfies "$ps_version" "$min_minor"; then
		return 1
	fi

	PYTHON_RUNNER="powershell"
	PYTHON_BIN="python"
	PYTHON_RUNNER_DESC="$ps_version"
	return 0
}

install_python_requirements() {
	local requirements_path="$1"
	if [[ ! -f "$requirements_path" ]]; then
		echo "ERROR: Python requirements file not found: $requirements_path" >&2
		return 1
	fi

	mkdir -p "$PYTHON_DEPS_DIR"

	if [[ "$PYTHON_RUNNER" == "native" ]]; then
		if ! "$PYTHON_BIN" -m pip --version >/dev/null 2>&1; then
			if ! activate_powershell_python; then
				echo "ERROR: native python lacks pip, and powershell python fallback is unavailable" >&2
				return 1
			fi
		else
			(
				cd "$PYTHON_PROJECT_ROOT"
				"$PYTHON_BIN" -m pip install --disable-pip-version-check --target "$PYTHON_DEPS_DIR" -r "$requirements_path"
			)
			return
		fi
	fi

	if [[ "$PYTHON_RUNNER" == "native" ]]; then
		(
			cd "$PYTHON_PROJECT_ROOT"
			"$PYTHON_BIN" -m pip install --disable-pip-version-check --target "$PYTHON_DEPS_DIR" -r "$requirements_path"
		)
		return
	fi

	local requirements_win
	requirements_win="$(to_windows_path "$requirements_path")"
	powershell.exe -NoProfile -Command "\$ErrorActionPreference='Stop'; New-Item -ItemType Directory -Force -Path $(quote_ps "$PYTHON_DEPS_DIR_WIN") | Out-Null; if (Get-Command python -ErrorAction SilentlyContinue) { & python -m pip install --disable-pip-version-check --target $(quote_ps "$PYTHON_DEPS_DIR_WIN") -r $(quote_ps "$requirements_win") } else { & py -3 -m pip install --disable-pip-version-check --target $(quote_ps "$PYTHON_DEPS_DIR_WIN") -r $(quote_ps "$requirements_win") }"
}

ensure_python_modules() {
	local requirements_path="$1"
	shift
	local modules=("$@")
	if python_has_modules "${modules[@]}"; then
		return 0
	fi
	install_python_requirements "$requirements_path"
	python_has_modules "${modules[@]}"
}

run_python_script() {
	local rel_dir="$1"
	shift
	local script_path="$1"
	shift

	if [[ "$PYTHON_RUNNER" == "native" ]]; then
		(
			cd "$PYTHON_PROJECT_ROOT/$rel_dir"
			if [[ -d "$PYTHON_DEPS_DIR" ]]; then
				PYTHONPATH="$PYTHON_DEPS_DIR${PYTHONPATH:+:$PYTHONPATH}" "$PYTHON_BIN" "$script_path" "$@"
			else
				"$PYTHON_BIN" "$script_path" "$@"
			fi
		)
		return
	fi

	local ps_dir="${PYTHON_PROJECT_ROOT_WIN}\\${rel_dir//\//\\}"
	local script_win
	script_win="$(to_windows_path "$PYTHON_PROJECT_ROOT/$rel_dir/$script_path")"
	local arg_string=""
	local arg
	for arg in "$@"; do
		if [[ -n "$arg_string" ]]; then
			arg_string+=" "
		fi
		arg_string+="$(quote_ps "$arg")"
	done

	local pythonpath_assign=""
	if [[ -d "$PYTHON_DEPS_DIR" ]]; then
		pythonpath_assign="if (\$env:PYTHONPATH) { \$env:PYTHONPATH = $(quote_ps "$PYTHON_DEPS_DIR_WIN") + [IO.Path]::PathSeparator + \$env:PYTHONPATH } else { \$env:PYTHONPATH = $(quote_ps "$PYTHON_DEPS_DIR_WIN") }; "
	fi

	powershell.exe -NoProfile -Command "\$ErrorActionPreference='Stop'; Set-Location -LiteralPath $(quote_ps "$ps_dir"); ${pythonpath_assign}if (Get-Command python -ErrorAction SilentlyContinue) { & python $(quote_ps "$script_win") ${arg_string} } else { & py -3 $(quote_ps "$script_win") ${arg_string} }"
}
