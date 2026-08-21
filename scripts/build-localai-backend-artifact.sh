#!/usr/bin/env bash

set -euo pipefail

: "${LOCALAI_ROOT:?LOCALAI_ROOT is required}"
: "${BACKEND_ID:?BACKEND_ID is required}"
: "${TARGET_ID:?TARGET_ID is required}"
: "${BUILD_TYPE:?BUILD_TYPE is required}"
: "${GRPC_COMMIT:?GRPC_COMMIT is required}"

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
		binary="grpc-server"
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

if [[ "$TARGET_ID" == "windows-amd64" ]]; then
	# The upstream package.sh files target ELF/Mach-O runtime layout. Windows
	# still builds from the same pinned sources, but stages the PE executable and
	# DLLs without pretending that a Unix loader script is executable on Windows.
	if [[ -z "${VCPKG_ROOT:-}" ]]; then
		echo "VCPKG_ROOT is required for the pinned Windows dependency build" >&2
		exit 1
	fi
	triplet="${VCPKG_TRIPLET:-x64-mingw-dynamic}"
	toolchain="${VCPKG_ROOT}/scripts/buildsystems/vcpkg.cmake"
	mkdir -p "${backend_path}/package"

	case "$BACKEND_ID" in
		localai-llamacpp)
			make -C "$backend_path" BUILD_TYPE=cpu BUILD_GRPC_FOR_BACKEND_LLAMA=1 \
				CMAKE_ARGS="-DCMAKE_TOOLCHAIN_FILE=${toolchain} -DVCPKG_TARGET_TRIPLET=${triplet} -DCMAKE_BUILD_TYPE=Release" \
				grpc-server
			find "$backend_path" -maxdepth 3 -type f \( -name 'grpc-server.exe' -o -name 'grpc-server' \) -size +0c -exec cp {} "${backend_path}/package/grpc-server.exe" \; -quit
			;;
		localai-whisper)
			make -C "$backend_path" sources/whisper.cpp
			cmake -S "$backend_path" -B "${backend_path}/build-windows" -G "MinGW Makefiles" \
				-DCMAKE_TOOLCHAIN_FILE="$toolchain" -DVCPKG_TARGET_TRIPLET="$triplet" \
				-DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF -DCMAKE_BUILD_TYPE=Release
			cmake --build "${backend_path}/build-windows" --config Release --target gowhisper
			go build -C "$backend_path" -o "${backend_path}/package/whisper.exe" ./
			find "${backend_path}/build-windows" -type f -name 'libgowhisper*.dll' -size +0c -exec cp {} "${backend_path}/package/" \;
			;;
		localai-vibevoice)
			make -C "$backend_path" sources/vibevoice.cpp
			cmake -S "$backend_path" -B "${backend_path}/build-windows" -G "MinGW Makefiles" \
				-DCMAKE_TOOLCHAIN_FILE="$toolchain" -DVCPKG_TARGET_TRIPLET="$triplet" \
				-DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF -DVIBEVOICE_BUILD_TESTS=OFF \
				-DVIBEVOICE_BUILD_EXAMPLES=OFF -DCMAKE_BUILD_TYPE=Release
			cmake --build "${backend_path}/build-windows" --config Release --target govibevoicecpp
			go build -C "$backend_path" -o "${backend_path}/package/vibevoice-cpp.exe" ./
			find "${backend_path}/build-windows" -type f -name 'libgovibevoicecpp*.dll' -size +0c -exec cp {} "${backend_path}/package/" \;
			;;
	esac
else
	case "$BACKEND_ID" in
		localai-llamacpp)
			# CPU_ALL_VARIANTS emits one gRPC executable plus runtime-selected CPU
			# libraries. It is the portable CPU/Metal shape used by run.sh.
			make -C "$backend_path" BUILD_TYPE="$BUILD_TYPE" \
				BUILD_GRPC_FOR_BACKEND_LLAMA=1 llama-cpp-cpu-all
			make -C "$backend_path" BUILD_TYPE="$BUILD_TYPE" package
			;;
		localai-whisper|localai-vibevoice)
			make -C "$backend_path" BUILD_TYPE="$BUILD_TYPE" build
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
