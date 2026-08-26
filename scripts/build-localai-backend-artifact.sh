#!/usr/bin/env bash

set -euo pipefail

: "${LOCALAI_ROOT:?LOCALAI_ROOT is required}"
: "${BACKEND_ID:?BACKEND_ID is required}"
: "${TARGET_ID:?TARGET_ID is required}"
: "${BUILD_TYPE:?BUILD_TYPE is required}"
: "${GRPC_COMMIT:?GRPC_COMMIT is required}"
: "${BACKEND_SOURCE_COMMIT:?BACKEND_SOURCE_COMMIT is required}"
: "${PROTOBUF_VERSION:?PROTOBUF_VERSION is required}"
: "${GRPC_VERSION:?GRPC_VERSION is required}"

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
config_path="${LOCALAI_ARTIFACT_CONFIG:-${repository_root}/.github/localai-backend-artifacts.json}"
workflow_script="${repository_root}/scripts/localai-backend-artifact-workflow.mjs"
export TAG_LIB_GRPC="$GRPC_COMMIT"

if [[ "$TARGET_ID" == "windows-amd64" ]] && command -v cygpath >/dev/null 2>&1; then
	LOCALAI_ROOT="$(cygpath -u "$LOCALAI_ROOT")"
	config_path="$(cygpath -u "$config_path")"
	if [[ -n "${VCPKG_ROOT:-}" ]]; then
		VCPKG_ROOT="$(cygpath -u "$VCPKG_ROOT")"
		export VCPKG_ROOT
	fi
	export LOCALAI_ROOT
fi

config_value() {
	node -e '
const fs = require("node:fs");
let value = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
for (const key of process.argv.slice(2)) value = value?.[key];
if (value === undefined || value === null) process.exit(1);
if (Array.isArray(value)) value = value.join(" ");
process.stdout.write(String(value));
' "$config_path" "$@"
}

version_line() {
	local output
	output="$("$@" 2>&1)"
	printf '%s\n' "${output%%$'\n'*}"
}

assert_tool_version() {
	local label="$1"
	local expected="$2"
	shift 2
	local observed
	observed="$(version_line "$@")"
	if [[ "$observed" != *"$expected"* ]]; then
		echo "$label version mismatch: observed '$observed', expected '$expected'" >&2
		exit 1
	fi
}

make_command="${MAKE_COMMAND:-}"
if [[ -z "$make_command" ]]; then
	if command -v gmake >/dev/null 2>&1; then
		make_command="gmake"
	else
		make_command="make"
	fi
fi

verify_host_toolchain() {
	assert_tool_version "Go" "go$(config_value toolchain goVersion)" go version
	assert_tool_version "CMake" "$(config_value toolchain cmakeVersion)" cmake --version

	case "$TARGET_ID" in
		linux-amd64)
			assert_tool_version "GCC" "$(config_value hostToolchain linux gccVersion)" gcc --version
			assert_tool_version "GNU Make" "$(config_value hostToolchain linux makeVersion)" "$make_command" --version
			assert_tool_version "Ninja" "$(config_value hostToolchain linux ninjaVersion)" ninja --version
			assert_tool_version "pkg-config" "$(config_value hostToolchain linux pkgConfigVersion)" pkg-config --version
			;;
		darwin-arm64)
			assert_tool_version "GNU Make" "$(config_value hostToolchain macos makeVersion)" "$make_command" --version
			;;
		windows-amd64)
			assert_tool_version "GCC" "$(config_value hostToolchain windows msysPackages | sed 's/.*gcc=//' | cut -d- -f1)" gcc --version
			assert_tool_version "GNU Make" "$(config_value hostToolchain windows msysPackages | sed -n 's/^.*make=\([^ ]*\).*$/\1/p' | cut -d- -f1)" "$make_command" --version
			assert_tool_version "Ninja" "$(config_value hostToolchain windows msysPackages | sed -n 's/.*ninja=\([^ ]*\)$/\1/p' | cut -d- -f1)" ninja --version
			;;
		*)
			echo "unsupported target: $TARGET_ID" >&2
			exit 1
			;;
	esac
}

verify_host_toolchain

node "$workflow_script" verify-source \
	--config "$config_path" \
	--localai-root "$LOCALAI_ROOT" \
	--backend "$BACKEND_ID" \
	--target "$TARGET_ID"

backend_path=""
binary=""
case "$BACKEND_ID" in
	localai-llamacpp)
		backend_path="${LOCALAI_ROOT}/backend/cpp/llama-cpp"
		binary="llama-cpp-cpu-all"
		;;
	localai-whisper)
		backend_path="${LOCALAI_ROOT}/backend/go/whisper"
		binary="whisper"
		;;
	localai-vibevoice)
		backend_path="${LOCALAI_ROOT}/backend/go/vibevoice-cpp"
		binary="vibevoice-cpp"
		;;
	*)
		echo "unsupported backend: ${BACKEND_ID}" >&2
		exit 1
		;;
esac

rm -rf "${backend_path}/package"

cmake_args=()
if [[ "$TARGET_ID" == "windows-amd64" ]]; then
	# The pinned cpp-httplib release rejects Windows 8 and older at compile time.
	# Build the PE artifact against the Windows 10 API surface it requires.
	windows_minimum_target="0x0A00"
	# A static vcpkg triplet keeps third-party gRPC/protobuf/abseil libraries out
	# of the runtime DLL closure. The link flags cover the MinGW C++ runtime;
	# any remaining native DLL is staged and verified below.
	if [[ -z "${VCPKG_ROOT:-}" ]]; then
		echo "VCPKG_ROOT is required for the pinned Windows dependency build" >&2
		exit 1
	fi
	triplet="${VCPKG_TRIPLET:-$(config_value hostToolchain windows vcpkgTriplet)}"
	toolchain="${VCPKG_ROOT}/scripts/buildsystems/vcpkg.cmake"
	overlay_triplets="${VCPKG_ROOT}/triplets/community"
	cmake_args=(
		"-DCMAKE_TOOLCHAIN_FILE=${toolchain}"
		"-DVCPKG_TARGET_TRIPLET=${triplet}"
		"-DVCPKG_OVERLAY_TRIPLETS=${overlay_triplets}"
		"-DCMAKE_BUILD_TYPE=Release"
		"-DBUILD_SHARED_LIBS=OFF"
		"-DCMAKE_CXX_FLAGS=-D_WIN32_WINNT=${windows_minimum_target}"
	)
	export VCPKG_TRIPLET="$triplet"
	export CXXFLAGS="${CXXFLAGS:-} -static-libgcc -static-libstdc++"
	export LDFLAGS="${LDFLAGS:-} -static-libgcc -static-libstdc++"
	export CGO_ENABLED=1
fi
cmake_args_text="${cmake_args[*]:-}"

build_grpc_dependencies() {
	local grpc_path="${LOCALAI_ROOT}/backend/cpp/grpc"
	local install_path="${grpc_path}/installed_packages"
	local protoc_path="${install_path}/bin/protoc"
	local plugin_path="${install_path}/bin/grpc_cpp_plugin"
	local grpc_cmake_args="${cmake_args_text}"

	if [[ ! -f "${grpc_path}/Makefile" ]]; then
		echo "pinned LocalAI gRPC build directory is missing: ${grpc_path}" >&2
		exit 1
	fi
	rm -rf "${grpc_path}/grpc_build" "${grpc_path}/grpc_repo" "$install_path"
	"$make_command" -C "$grpc_path" TAG_LIB_GRPC="$GRPC_COMMIT" CMAKE_ARGS="$grpc_cmake_args" build

	if [[ ! -x "$protoc_path" ]]; then
		echo "pinned gRPC build did not install ${protoc_path}" >&2
		exit 1
	fi
	if [[ ! -x "$plugin_path" ]]; then
		echo "pinned gRPC build did not install ${plugin_path}" >&2
		exit 1
	fi
	assert_tool_version "protobuf" "libprotoc ${PROTOBUF_VERSION}" "$protoc_path" --version
	if ! grep -Eq "set\\(PACKAGE_VERSION[[:space:]]+\"${GRPC_VERSION}\"\\)" "${grpc_path}/grpc_repo/grpc/CMakeLists.txt"; then
		echo "pinned gRPC source does not declare version ${GRPC_VERSION}" >&2
		exit 1
	fi
	# The pinned LocalAI Makefile refers to this historical short name while
	# gRPC installs the compiler as protoc. Keep the build in the upstream path,
	# but make the verified executable available under the name it requests.
	if [[ ! -e "${install_path}/bin/proto" ]]; then
		ln -s protoc "${install_path}/bin/proto"
	fi
}

stage_darwin_llama_package() {
	local package_root="${backend_path}/package"
	mkdir -p "${package_root}/lib"
	cp -f "${backend_path}/llama-cpp-cpu-all" "${package_root}/llama-cpp-cpu-all"
	cp -f "${backend_path}/run.sh" "${package_root}/run.sh"
	shopt -s nullglob
	local shared_libraries=("${backend_path}/ggml-shared-libs/"*.dylib)
	if (( ${#shared_libraries[@]} > 0 )); then
		cp -f "${shared_libraries[@]}" "${package_root}/lib/"
	fi
	shopt -u nullglob
}

stage_windows_runtime() {
	local package_root="$1"
	local package_root_abs path name output dep_path
	local -a queue=()
	local -a package_files=()
	package_root_abs="$(cd "$package_root" && pwd)"
	if ! command -v ldd >/dev/null 2>&1; then
		echo "MSYS2 ldd is required to verify the Windows runtime closure" >&2
		exit 1
	fi
	while IFS= read -r -d '' path; do queue+=("$path"); done < <(find "$package_root" -maxdepth 1 -type f \( -iname '*.exe' -o -iname '*.dll' \) -print0)
	if (( ${#queue[@]} == 0 )); then
		echo "Windows package has no executable or DLL to inspect" >&2
		exit 1
	fi

	local index=0
	while (( index < ${#queue[@]} )); do
		path="${queue[index]}"
		index=$((index + 1))
		output="$(PATH="${package_root}:$PATH" ldd "$path" 2>&1)" || {
			echo "could not inspect Windows runtime dependencies for $path" >&2
			echo "$output" >&2
			exit 1
		}
		if grep -qi 'not found' <<<"$output"; then
			echo "Windows runtime dependency is missing for $path" >&2
			echo "$output" >&2
			exit 1
		fi
		while IFS= read -r dep_path; do
			[[ -n "$dep_path" ]] || continue
			shopt -s nocasematch
			if [[ "$dep_path" == "$package_root_abs"/* || "$dep_path" == /c/windows/* || "$dep_path" == /d/windows/* ]]; then
				shopt -u nocasematch
				continue
			fi
			shopt -u nocasematch
			if [[ "$dep_path" != /mingw64/* && "$dep_path" != /mingw32/* ]]; then
				echo "Windows package depends on an unstaged non-system runtime: $dep_path" >&2
				exit 1
			fi
			name="$(basename "$dep_path")"
			if [[ ! -f "${package_root}/${name}" ]]; then
				cp -f "$dep_path" "${package_root}/${name}"
				queue+=("${package_root}/${name}")
			fi
		done < <(awk '/=>/ && tolower($3) ~ /^\/.*\.dll$/ { print $3 } /^[[:space:]]*\/.*\.dll[[:space:]]*\(/ && tolower($1) ~ /\.dll$/ { print $1 }' <<<"$output")
	done

	while IFS= read -r -d '' path; do package_files+=("$path"); done < <(find "$package_root" -maxdepth 1 -type f \( -iname '*.exe' -o -iname '*.dll' \) -print0)
	for path in "${package_files[@]}"; do
		output="$(PATH="${package_root}:$PATH" ldd "$path" 2>&1)" || {
			echo "final Windows runtime verification failed for $path" >&2
			echo "$output" >&2
			exit 1
		}
		if grep -qi 'not found' <<<"$output"; then
			echo "final Windows runtime closure still has an unresolved dependency for $path" >&2
			echo "$output" >&2
			exit 1
		fi
		while IFS= read -r dep_path; do
			[[ -n "$dep_path" ]] || continue
			shopt -s nocasematch
			if [[ "$dep_path" == "$package_root_abs"/* || "$dep_path" == /c/windows/* || "$dep_path" == /d/windows/* ]]; then
				shopt -u nocasematch
				continue
			fi
			shopt -u nocasematch
			name="$(basename "$dep_path")"
			if [[ ! -f "${package_root}/${name}" ]]; then
				echo "final Windows package omits non-system dependency $name" >&2
				exit 1
			fi
		done < <(awk '/=>/ && tolower($3) ~ /^\/.*\.dll$/ { print $3 } /^[[:space:]]*\/.*\.dll[[:space:]]*\(/ && tolower($1) ~ /\.dll$/ { print $1 }' <<<"$output")
	done
}

if [[ "$BACKEND_ID" == "localai-llamacpp" ]]; then
	build_grpc_dependencies
fi

os_make_args=()
if [[ "$TARGET_ID" == "darwin-arm64" ]]; then
	os_make_args=(OS=Darwin)
fi

if [[ "$TARGET_ID" == "windows-amd64" ]]; then
	mkdir -p "${backend_path}/package"
	case "$BACKEND_ID" in
		localai-llamacpp)
			# The upstream package target is Unix-only. Build its gRPC target with
			# the static Windows toolchain and stage it under the canonical
			# llama-cpp-cpu-all entrypoint used by the package contract.
			"$make_command" -C "$backend_path" BUILD_TYPE=cpu BUILD_GRPC_FOR_BACKEND_LLAMA=1 CMAKE_ARGS="$cmake_args_text" grpc-server
			mkdir -p "${backend_path}/package"
			built_binary="$(find "$backend_path" -maxdepth 3 -type f \( -name 'grpc-server.exe' -o -name 'grpc-server' \) -size +0c -print -quit)"
			[[ -n "$built_binary" ]] || { echo "Windows llama gRPC executable was not produced" >&2; exit 1; }
			cp -f "$built_binary" "${backend_path}/package/${binary}.exe"
			;;
		localai-whisper)
			"$make_command" -C "$backend_path" sources/whisper.cpp
			cmake -S "$backend_path" -B "${backend_path}/build-windows" -G "MinGW Makefiles" "${cmake_args[@]}" -DGGML_NATIVE=OFF
			cmake --build "${backend_path}/build-windows" --config Release --target gowhisper
			go build -C "$backend_path" -o "${backend_path}/package/${binary}.exe" ./
			find "${backend_path}/build-windows" -type f -name 'libgowhisper*.dll' -size +0c -exec cp {} "${backend_path}/package/" \;
			;;
		localai-vibevoice)
			"$make_command" -C "$backend_path" sources/vibevoice.cpp
			cmake -S "$backend_path" -B "${backend_path}/build-windows" -G "MinGW Makefiles" "${cmake_args[@]}" -DGGML_NATIVE=OFF -DVIBEVOICE_BUILD_TESTS=OFF -DVIBEVOICE_BUILD_EXAMPLES=OFF
			cmake --build "${backend_path}/build-windows" --config Release --target govibevoicecpp
			go build -C "$backend_path" -o "${backend_path}/package/${binary}.exe" ./
			find "${backend_path}/build-windows" -type f -name 'libgovibevoicecpp*.dll' -size +0c -exec cp {} "${backend_path}/package/" \;
			;;
		*)
			echo "unsupported Windows backend: $BACKEND_ID" >&2
			exit 1
			;;
	esac
	stage_windows_runtime "${backend_path}/package"
else
	case "$BACKEND_ID" in
		localai-llamacpp)
			"$make_command" -C "$backend_path" "${os_make_args[@]}" BUILD_TYPE="$BUILD_TYPE" BUILD_GRPC_FOR_BACKEND_LLAMA=1 CMAKE_ARGS="$cmake_args_text" llama-cpp-cpu-all
			if [[ "$TARGET_ID" == "darwin-arm64" ]]; then
				stage_darwin_llama_package
			else
				# package.sh copies llama-cpp-*; remove the build directory so only
				# the real executable, run script, and runtime libraries are staged.
				rm -rf "${backend_path}/llama-cpp-cpu-all-build"
				"$make_command" -C "$backend_path" "${os_make_args[@]}" BUILD_TYPE="$BUILD_TYPE" package
			fi
			;;
		localai-whisper|localai-vibevoice)
			"$make_command" -C "$backend_path" "${os_make_args[@]}" BUILD_TYPE="$BUILD_TYPE" build
			;;
		*)
			echo "unsupported Unix backend: $BACKEND_ID" >&2
			exit 1
			;;
	esac
fi

package_root="${backend_path}/package"
node "$workflow_script" verify-payload \
	--package-root "$package_root" \
	--binary "$binary" \
	--target "$TARGET_ID"

node "$workflow_script" metadata \
	--config "$config_path" \
	--localai-root "$LOCALAI_ROOT" \
	--backend "$BACKEND_ID" \
	--target "$TARGET_ID" \
	--output "${package_root}/build-metadata.json"

echo "LOCALAI_BACKEND_PACKAGE_OK backend=${BACKEND_ID} target=${TARGET_ID} path=${package_root}"
