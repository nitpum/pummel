#!/bin/sh
# Build pummel with zig cc on every platform.
# Requires go and zig (https://ziglang.org/download/).
set -eu
cd "$(dirname "$0")"

OS="$(uname -s)"
ARCH="$(uname -m)"

if ! command -v zig >/dev/null 2>&1; then
	echo "zig not found on PATH - install it from https://ziglang.org/download/" >&2
	exit 1
fi

case "$OS/$ARCH" in
Linux/x86_64)
	CC="zig cc -target x86_64-linux-musl"
	;;
Linux/aarch64)
	CC="zig cc -target aarch64-linux-musl"
	;;
Darwin/arm64)
	SDK="$(xcrun --show-sdk-path)"
	CC="zig cc -target aarch64-macos -isysroot $SDK -L$SDK/usr/lib -F$SDK/System/Library/Frameworks"
	;;
Darwin/x86_64)
	SDK="$(xcrun --show-sdk-path)"
	CC="zig cc -target x86_64-macos -isysroot $SDK -L$SDK/usr/lib -F$SDK/System/Library/Frameworks"
	;;
MINGW*/*|MSYS*/*|CYGWIN*/*)
	CC="zig cc -target x86_64-windows-gnu"
	;;
*)
	echo "unsupported platform: $OS/$ARCH" >&2
	exit 1
	;;
esac

CGO_ENABLED=1 CC="$CC" go build -trimpath -ldflags "-s -w" -o pummel .
echo "built ./pummel"
