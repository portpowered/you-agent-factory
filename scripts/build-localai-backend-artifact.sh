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
	if [[ -n "${WINDOWS_NODE_DIR:-}" ]]; then
		node_bin_dir="$(cygpath -u "$WINDOWS_NODE_DIR")"
		export PATH="$node_bin_dir:$PATH"
	fi
	if [[ -n "${WINDOWS_GIT_DIR:-}" ]]; then
		git_bin_dir="$(cygpath -u "$WINDOWS_GIT_DIR")"
		export PATH="$git_bin_dir:$PATH"
	fi
	if [[ -n "${WINDOWS_GO_DIR:-}" ]]; then
		go_bin_dir="$(cygpath -u "$WINDOWS_GO_DIR")"
		export PATH="$go_bin_dir:$PATH"
	fi
	if [[ -n "${WINDOWS_CMAKE_DIR:-}" ]]; then
		cmake_bin_dir="$(cygpath -u "$WINDOWS_CMAKE_DIR")"
		export PATH="$cmake_bin_dir:$PATH"
	fi
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
			assert_tool_version "MinGW Make" "$(config_value hostToolchain windows msysPackages | sed -n 's/^.*mingw-w64-x86_64-make=\([^ ]*\).*$/\1/p' | cut -d- -f1)" mingw32-make --version
			assert_tool_version "Ninja" "$(config_value hostToolchain windows msysPackages | sed -n 's/.*ninja=\([^ ]*\)$/\1/p' | cut -d- -f1)" ninja --version
			;;
		*)
			echo "unsupported target: $TARGET_ID" >&2
			exit 1
			;;
	esac
}

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

build_shell="bash"
if [[ "$TARGET_ID" == "windows-amd64" ]]; then
	build_shell="msys2"
fi

build_strategy=""
case "$TARGET_ID:$BACKEND_ID" in
	windows-amd64:localai-llamacpp)
		build_strategy="windows-llamacpp-grpc"
		;;
	windows-amd64:localai-whisper)
		build_strategy="windows-whisper"
		;;
	windows-amd64:localai-vibevoice)
		build_strategy="windows-vibevoice"
		;;
	darwin-arm64:localai-llamacpp)
		build_strategy="darwin-llamacpp-grpc"
		;;
	linux-amd64:localai-llamacpp)
		build_strategy="linux-llamacpp-package"
		;;
	darwin-arm64:localai-whisper|darwin-arm64:localai-vibevoice)
		build_strategy="darwin-go-package"
		;;
	linux-amd64:localai-whisper|linux-amd64:localai-vibevoice)
		build_strategy="linux-go-build"
		;;
	*)
		echo "unsupported backend/target: ${BACKEND_ID}/${TARGET_ID}" >&2
		exit 1
		;;
esac

go_dynamic_loader="none"
if [[ "$TARGET_ID" == "windows-amd64" ]]; then
	case "$BACKEND_ID" in
		localai-whisper|localai-vibevoice)
			# purego v0.10.0 intentionally exposes Dlopen only on Unix. The
			# pinned LocalAI Go entrypoints use that API unconditionally, so the
			# Windows build adds a small build-tagged loader shim below.
			go_dynamic_loader="xsys-windows"
			;;
	esac
fi

# The pinned gRPC CMake project otherwise lets the Windows generator select
# C++14, which Abseil rejects before any backend target can compile. The
# declared vcpkg triplet is MinGW; without an explicit generator, hosted
# Windows runners select Visual Studio and mix MSVC with MinGW dependencies.
windows_cxx_standard=17
windows_cmake_generator="MinGW Makefiles"
windows_minimum_target="0x0602"
grpc_dependency_mode="default"
grpc_protobuf_source="system"

if [[ "$TARGET_ID" == "windows-amd64" ]]; then
	export CMAKE_GENERATOR="$windows_cmake_generator"
	# The pinned gRPC checkout builds its own protobuf and Abseil sources. Do
	# not pass the Windows vcpkg toolchain to that bootstrap: its rolling
	# protobuf/Abseil headers are not compatible with the pinned gRPC source.
	grpc_dependency_mode="standalone"
	grpc_protobuf_source="pinned"
fi

if [[ "${LOCALAI_BUILD_PLAN_ONLY:-0}" == "1" ]]; then
	plan_git=""
	if [[ "$TARGET_ID" == "windows-amd64" && -n "${WINDOWS_GIT_DIR:-}" ]]; then
		plan_git=" git=$(command -v git || true)"
	fi
	plan_cxx_standard=""
	plan_cmake_generator=""
	plan_cmake_make_program=""
	plan_windows_target=""
	plan_grpc_protobuf_source=""
	plan_go_dynamic_loader=" go_dynamic_loader=$go_dynamic_loader"
	plan_grpc_dependency_mode=" grpc_dependency_mode=$grpc_dependency_mode"
	if [[ "$TARGET_ID" == "windows-amd64" ]]; then
		plan_cxx_standard=" cxx_standard=$windows_cxx_standard"
		plan_cmake_generator=" cmake_generator=mingw-makefiles"
		plan_cmake_make_program=" cmake_make_program=mingw32-make"
		plan_windows_target=" windows_minimum_target=$windows_minimum_target"
		plan_grpc_protobuf_source=" grpc_protobuf_source=$grpc_protobuf_source"
	fi
	printf 'LOCALAI_BACKEND_BUILD_PLAN backend=%s target=%s shell=%s strategy=%s binary=%s%s%s%s%s\n' \
		"$BACKEND_ID" "$TARGET_ID" "$build_shell" "$build_strategy" "$binary" "$plan_git" "$plan_cxx_standard" "$plan_cmake_generator$plan_cmake_make_program$plan_windows_target$plan_grpc_protobuf_source$plan_go_dynamic_loader$plan_grpc_dependency_mode"
	exit 0
fi

verify_host_toolchain

node "$workflow_script" verify-source \
	--config "$config_path" \
	--localai-root "$LOCALAI_ROOT" \
	--backend "$BACKEND_ID" \
	--target "$TARGET_ID"

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
		"-DCMAKE_MAKE_PROGRAM=mingw32-make"
		"-DCMAKE_C_COMPILER=gcc"
		"-DCMAKE_CXX_COMPILER=g++"
		"-DCMAKE_BUILD_TYPE=Release"
		"-DBUILD_SHARED_LIBS=OFF"
		"-DCMAKE_CXX_STANDARD=${windows_cxx_standard}"
		"-DCMAKE_CXX_STANDARD_REQUIRED=ON"
		"-DCMAKE_CXX_FLAGS=-D_WIN32_WINNT=${windows_minimum_target}"
	)
	export VCPKG_TRIPLET="$triplet"
	export CXXFLAGS="${CXXFLAGS:-} -static-libgcc -static-libstdc++"
	export LDFLAGS="${LDFLAGS:-} -static-libgcc -static-libstdc++"
	export CGO_ENABLED=1
fi
cmake_args_text="${cmake_args[*]:-}"
grpc_dependency_cmake_args_text="$cmake_args_text"
if [[ "$TARGET_ID" == "windows-amd64" ]]; then
	# Keep the pinned gRPC/protobuf/Abseil dependency graph independent from
	# vcpkg. The backend builds below still receive cmake_args_text and use the
	# static vcpkg triplet where their CMake projects need it.
	grpc_dependency_cmake_args=(
		"-DCMAKE_MAKE_PROGRAM=mingw32-make"
		"-DCMAKE_C_COMPILER=gcc"
		"-DCMAKE_CXX_COMPILER=g++"
		"-DCMAKE_BUILD_TYPE=Release"
		"-DBUILD_SHARED_LIBS=OFF"
		"-DCMAKE_CXX_STANDARD=${windows_cxx_standard}"
		"-DCMAKE_CXX_STANDARD_REQUIRED=ON"
		"-DgRPC_ZLIB_PROVIDER=module"
		"-DgRPC_CARES_PROVIDER=module"
		"-DgRPC_RE2_PROVIDER=module"
		"-DgRPC_SSL_PROVIDER=module"
		"-DgRPC_PROTOBUF_PROVIDER=module"
		"-DgRPC_ABSL_PROVIDER=module"
	)
	grpc_dependency_cmake_args_text="${grpc_dependency_cmake_args[*]}"
fi
grpc_added_cmake_args=""

build_grpc_dependencies() {
	local grpc_path="${LOCALAI_ROOT}/backend/cpp/grpc"
	local install_path="${grpc_path}/installed_packages"
	local protoc_path="${install_path}/bin/protoc"
	local plugin_path="${install_path}/bin/grpc_cpp_plugin"
	local grpc_cmake_args="${grpc_dependency_cmake_args_text}"
	grpc_added_cmake_args="-Dabsl_DIR=${install_path}/lib/cmake/absl -DProtobuf_DIR=${install_path}/lib/cmake/protobuf -DProtobuf_INCLUDE_DIRS=${install_path}/include -DProtobuf_PROTOC_EXECUTABLE=${protoc_path} -D_PROTOBUF_PROTOC=${protoc_path} -D_GRPC_CPP_PLUGIN_EXECUTABLE=${plugin_path} -Dutf8_range_DIR=${install_path}/lib/cmake/utf8_range -DgRPC_DIR=${install_path}/lib/cmake/grpc -DCMAKE_CXX_STANDARD_INCLUDE_DIRECTORIES=${install_path}/include"

	if [[ ! -f "${grpc_path}/Makefile" ]]; then
		echo "pinned LocalAI gRPC build directory is missing: ${grpc_path}" >&2
		exit 1
	fi
	rm -rf "${grpc_path}/grpc_build" "${grpc_path}/grpc_repo" "$install_path"
	CMAKE_ARGS="$grpc_cmake_args" "$make_command" -C "$grpc_path" TAG_LIB_GRPC="$GRPC_COMMIT" build

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

ensure_localai_grpc_compat_path() {
	local grpc_path="${LOCALAI_ROOT}/backend/cpp/grpc"
	local compatibility_path="${LOCALAI_ROOT}/backend/grpc"
	local expected_path actual_path

	if [[ ! -d "$grpc_path" ]]; then
		echo "pinned LocalAI gRPC directory is missing: ${grpc_path}" >&2
		exit 1
	fi
	expected_path="$(cd "$grpc_path" && pwd -P)"
	if [[ -e "$compatibility_path" || -L "$compatibility_path" ]]; then
		if [[ ! -d "$compatibility_path" ]]; then
			echo "LocalAI gRPC compatibility path is not a directory: ${compatibility_path}" >&2
			exit 1
		fi
		actual_path="$(cd "$compatibility_path" && pwd -P)"
		if [[ "$actual_path" != "$expected_path" ]]; then
			echo "LocalAI gRPC compatibility path resolves to ${actual_path}, expected ${expected_path}" >&2
			exit 1
		fi
		return
	fi
	ln -s "$grpc_path" "$compatibility_path"
}

run_direct_grpc_server_make() {
	local cmake_args_text_arg="$1"
	shift
	local grpc_path="${LOCALAI_ROOT}/backend/cpp/grpc"
	env \
		"_PROTOBUF_PROTOC=${grpc_path}/installed_packages/bin/proto" \
		"_GRPC_CPP_PLUGIN_EXECUTABLE=${grpc_path}/installed_packages/bin/grpc_cpp_plugin" \
		"PATH=${grpc_path}/installed_packages/bin:${PATH}" \
		CMAKE_ARGS="$cmake_args_text_arg" \
		"$make_command" -C "$backend_path" "$@"
}

patch_llama_grpc_source() {
	local grpc_source="${backend_path}/llama.cpp/tools/grpc-server/grpc-server.cpp"
	"$make_command" -C "$backend_path" llama.cpp/tools/grpc-server
	if [[ ! -f "$grpc_source" ]]; then
		echo "pinned LocalAI llama gRPC source is missing: ${grpc_source}" >&2
		exit 1
	fi
	node -e '
const fs = require("node:fs");
const path = process.argv[1];
const source = fs.readFileSync(path, "utf8");
const needle = "reply->set_message(arr);";
const replacement = "reply->set_message(arr.dump());";
const count = source.split(needle).length - 1;
if (count !== 1) {
  console.error(`expected one multi-result protobuf string conversion in ${path}, found ${count}`);
  process.exit(1);
}
fs.writeFileSync(path, source.replace(needle, replacement));
' "$grpc_source"
}

generate_go_protocol() {
	local grpc_path="${LOCALAI_ROOT}/backend/cpp/grpc"
	local protoc_path="${grpc_path}/installed_packages/bin/protoc"
	local protocol_output="${LOCALAI_ROOT}/pkg/grpc/proto"
	local go_bin

	go_bin="$(go env GOPATH)/bin"
	if command -v cygpath >/dev/null 2>&1; then
		go_bin="$(cygpath -u "$go_bin")"
	fi
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@1958fcbe2ca8bd93af633f11e97d44e567e945af
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2
	mkdir -p "$protocol_output"
	PATH="$go_bin:$PATH" "$protoc_path" \
		--experimental_allow_proto3_optional \
		-I"${LOCALAI_ROOT}/backend" \
		--go_out="$protocol_output" \
		--go_opt=paths=source_relative \
		--go-grpc_out="$protocol_output" \
		--go-grpc_opt=paths=source_relative \
		"${LOCALAI_ROOT}/backend/backend.proto"
}

patch_windows_go_loader() {
	if [[ "$go_dynamic_loader" != "xsys-windows" ]]; then
		return
	fi

	local main_source="${backend_path}/main.go"
	local loader_source="${backend_path}/localai-backend-library_windows.go"
	if [[ ! -f "$main_source" ]]; then
		echo "pinned LocalAI Go backend entrypoint is missing: ${main_source}" >&2
		exit 1
	fi

	node - "$main_source" "$loader_source" <<'NODE'
const { readFileSync, writeFileSync } = require("node:fs");

const [mainPath, loaderPath] = process.argv.slice(2);
const source = readFileSync(mainPath, "utf8");
const needle = "purego.Dlopen(libName, purego.RTLD_NOW|purego.RTLD_GLOBAL)";
const occurrences = source.split(needle).length - 1;
if (occurrences !== 1) {
	console.error(`expected one purego dynamic-loader call in ${mainPath}, found ${occurrences}`);
	process.exit(1);
}

writeFileSync(mainPath, source.replace(needle, "loadBackendLibrary(libName)"));
writeFileSync(
	loaderPath,
	`//go:build windows

package main

import "golang.org/x/sys/windows"

func loadBackendLibrary(name string) (uintptr, error) {
	handle, err := windows.LoadLibrary(name)
	return uintptr(handle), err
}
`,
);
NODE
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

stage_darwin_go_package() {
	local package_root="${backend_path}/package"
	local library library_count=0
	mkdir -p "${package_root}/lib"
	cp -f "${backend_path}/${binary}" "${package_root}/${binary}"
	cp -f "${backend_path}/run.sh" "${package_root}/run.sh"
	# The pinned whisper checkout names its Darwin output with a .dylib suffix,
	# while other pinned Go backends may retain the CMake .so suffix. Stage both
	# Mach-O names so the package contains the library selected by run.sh.
	for library in "${backend_path}"/libgo*.dylib "${backend_path}"/libgo*.so; do
		if [[ -s "$library" ]]; then
			cp -f "$library" "${package_root}/"
			library_count=$((library_count + 1))
		fi
	done
	if (( library_count == 0 )); then
		echo "Darwin ${BACKEND_ID} build did not produce a Go backend dylib" >&2
		exit 1
	fi
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

build_grpc_dependencies
generate_go_protocol

os_make_args=()
if [[ "$TARGET_ID" == "darwin-arm64" ]]; then
	os_make_args=(OS=Darwin)
fi
if [[ "$BACKEND_ID" == "localai-llamacpp" ]]; then
	ensure_localai_grpc_compat_path
	patch_llama_grpc_source
fi
patch_windows_go_loader

case "$build_strategy" in
	windows-llamacpp-grpc)
	mkdir -p "${backend_path}/package"
	# The upstream package target is Unix-only. Build its gRPC target with
	# the static Windows toolchain and stage it under the canonical
	# llama-cpp-cpu-all entrypoint used by the package contract.
	run_direct_grpc_server_make "${cmake_args_text} ${grpc_added_cmake_args}" BUILD_TYPE=cpu BUILD_GRPC_FOR_BACKEND_LLAMA=1 grpc-server
	mkdir -p "${backend_path}/package"
	built_binary="$(find "$backend_path" -maxdepth 3 -type f \( -name 'grpc-server.exe' -o -name 'grpc-server' \) -size +0c -print -quit)"
	[[ -n "$built_binary" ]] || { echo "Windows llama gRPC executable was not produced" >&2; exit 1; }
	cp -f "$built_binary" "${backend_path}/package/${binary}.exe"
	stage_windows_runtime "${backend_path}/package"
	;;
	windows-whisper)
	mkdir -p "${backend_path}/package"
	"$make_command" -C "$backend_path" sources/whisper.cpp
	cmake -S "$backend_path" -B "${backend_path}/build-windows" -G "$windows_cmake_generator" "${cmake_args[@]}" -DGGML_NATIVE=OFF
	cmake --build "${backend_path}/build-windows" --config Release --target gowhisper
	go build -C "$backend_path" -o "${backend_path}/package/${binary}.exe" ./
	find "${backend_path}/build-windows" -type f -name 'libgowhisper*.dll' -size +0c -exec cp {} "${backend_path}/package/" \;
	stage_windows_runtime "${backend_path}/package"
	;;
	windows-vibevoice)
	mkdir -p "${backend_path}/package"
	"$make_command" -C "$backend_path" sources/vibevoice.cpp
	cmake -S "$backend_path" -B "${backend_path}/build-windows" -G "$windows_cmake_generator" "${cmake_args[@]}" -DGGML_NATIVE=OFF -DVIBEVOICE_BUILD_TESTS=OFF -DVIBEVOICE_BUILD_EXAMPLES=OFF
	cmake --build "${backend_path}/build-windows" --config Release --target govibevoicecpp
	go build -C "$backend_path" -o "${backend_path}/package/${binary}.exe" ./
	find "${backend_path}/build-windows" -type f -name 'libgovibevoicecpp*.dll' -size +0c -exec cp {} "${backend_path}/package/" \;
	stage_windows_runtime "${backend_path}/package"
	;;
	darwin-llamacpp-grpc)
	# The pinned CPU-all target always appends GGML_CPU_ALL_VARIANTS=ON,
	# which cannot be combined with GGML_CPU_ARM_ARCH. Build the same
	# pinned grpc-server directly with a generic Darwin arm64 setting, then
	# retain the existing CPU-all executable name and package contract.
	darwin_llama_cmake_args="${cmake_args_text} ${grpc_added_cmake_args} -DGGML_CPU_ARM_ARCH=armv8.2-a+dotprod"
	run_direct_grpc_server_make "$darwin_llama_cmake_args" "${os_make_args[@]}" BUILD_TYPE="$BUILD_TYPE" BUILD_GRPC_FOR_BACKEND_LLAMA=1 grpc-server
	test -s "${backend_path}/grpc-server"
	cp -f "${backend_path}/grpc-server" "${backend_path}/llama-cpp-cpu-all"
	stage_darwin_llama_package
	;;
	linux-llamacpp-package)
	CMAKE_ARGS="$cmake_args_text" "$make_command" -C "$backend_path" "${os_make_args[@]}" BUILD_TYPE="$BUILD_TYPE" BUILD_GRPC_FOR_BACKEND_LLAMA=1 llama-cpp-cpu-all
	# package.sh copies llama-cpp-*; remove the build directory so only
	# the real executable, run script, and runtime libraries are staged.
	rm -rf "${backend_path}/llama-cpp-cpu-all-build"
	"$make_command" -C "$backend_path" "${os_make_args[@]}" BUILD_TYPE="$BUILD_TYPE" package
	;;
	darwin-go-package)
	"$make_command" -C "$backend_path" "${os_make_args[@]}" BUILD_TYPE="$BUILD_TYPE" JOBS=2 "$binary"
	stage_darwin_go_package
	;;
	linux-go-build)
	"$make_command" -C "$backend_path" "${os_make_args[@]}" BUILD_TYPE="$BUILD_TYPE" build
		;;
	*)
		echo "unsupported build strategy: $build_strategy" >&2
		exit 1
		;;
esac

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
