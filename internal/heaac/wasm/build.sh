#!/bin/bash
# Builds aac.wasm: libavcodec's AAC decoder alone, and aac.c around it, for
# wasm32-wasip1, which internal/heaac runs through wazero. Ringo uses it
# for HE-AAC only, which MyGo's own AAC decoder doesn't decode. Nothing
# else of FFmpeg goes in: no demuxers (MyGo reads the MP4), no resampler,
# no threads.
#
#   internal/heaac/wasm/build.sh [work directory]
#
# It downloads wasi-sdk and FFmpeg's sources, at the versions pinned here,
# the same as internal/video/wasm/build.sh's, into the work directory (by
# default one under $TMPDIR). What of FFmpeg this builds is under the LGPL
# 2.1 or later, which the AGPL Ringo is under allows; those sources, and
# this, rebuild aac.wasm as it is.
set -euo pipefail

WASI_SDK=34
FFMPEG=8.1.3
FFMPEG_SHA256=7138d28c96d9d3e3af4ee3d8cad72741f8ffb40da90c1112235dea3ecd3178a3

here=$(cd "$(dirname "$0")" && pwd)
work=${1:-${TMPDIR:-/tmp}/ringo-aac}
mkdir -p "$work"
cd "$work"

case "$(uname -s)-$(uname -m)" in
Darwin-arm64) host=arm64-macos; sdk_sha=9c59398106b417f8f14913380fdf0097a8cc0ff4af9eb3ce0065a859e88d49e9 ;;
Darwin-x86_64) host=x86_64-macos; sdk_sha= ;;
Linux-x86_64) host=x86_64-linux; sdk_sha=b761e3a0721dbae9c09a0059e5fdb2bf917d1b4a8a7b430fb3b5aafb0984b2c4 ;;
Linux-aarch64) host=arm64-linux; sdk_sha= ;;
*) echo "no wasi-sdk for this machine" >&2; exit 1 ;;
esac

sdk=wasi-sdk-$WASI_SDK.0-$host
if [ ! -d "$sdk" ]; then
	curl -fsSLo sdk.tar.gz "https://github.com/WebAssembly/wasi-sdk/releases/download/wasi-sdk-$WASI_SDK/$sdk.tar.gz"
	if [ -n "$sdk_sha" ]; then
		echo "$sdk_sha  sdk.tar.gz" | shasum -a 256 -c -
	fi
	tar xzf sdk.tar.gz
fi
src=ffmpeg-$FFMPEG
if [ ! -d "$src" ]; then
	curl -fsSLo ffmpeg.tar.xz "https://ffmpeg.org/releases/$src.tar.xz"
	echo "$FFMPEG_SHA256  ffmpeg.tar.xz" | shasum -a 256 -c -
	tar xJf ffmpeg.tar.xz
fi

cc=$work/$sdk/bin/clang
sysroot=$work/$sdk/share/wasi-sysroot
flags="--target=wasm32-wasip1 -D_WASI_EMULATED_PROCESS_CLOCKS -O3 -msimd128 -mbulk-memory -mnontrapping-fptoint -msign-ext"

mkdir -p build-aac
cd build-aac
"../$src/configure" \
	--enable-cross-compile --target-os=none --arch=wasm32 \
	--cc="$cc" --ar="$work/$sdk/bin/llvm-ar" --ranlib="$work/$sdk/bin/llvm-ranlib" --nm="$work/$sdk/bin/llvm-nm" \
	--sysroot="$sysroot" --extra-cflags="$flags" --extra-ldflags="--target=wasm32-wasip1" \
	--disable-everything --disable-all --enable-avcodec --enable-avutil \
	--enable-decoder=aac \
	--disable-asm --disable-pthreads --disable-w32threads --disable-os2threads \
	--disable-network --disable-programs --disable-doc --disable-debug \
	--disable-runtime-cpudetect --disable-autodetect --disable-iconv \
	--enable-small --disable-stripping >/dev/null
make -j"$(getconf _NPROCESSORS_ONLN)" libavcodec/libavcodec.a libavutil/libavutil.a >/dev/null

# A reactor: no main, aac.c's exports called one at a time.
"$cc" $flags --sysroot="$sysroot" -mexec-model=reactor -I. -I"../$src" \
	"$here/aac.c" libavcodec/libavcodec.a libavutil/libavutil.a \
	-lm -lwasi-emulated-process-clocks \
	-Wl,--gc-sections -Wl,--strip-all -Wl,-z,stack-size=1048576 \
	-o "$here/aac.wasm"
ls -l "$here/aac.wasm"
