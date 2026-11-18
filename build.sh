#!/bin/sh
# Build pummel. Requires go; zig for static Linux builds (https://ziglang.org/download/).
set -eu
cd "$(dirname "$0")"

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS/$ARCH" in
Linux/x86_64)
	CC="zig cc -target x86_64-linux-musl"
	;;
Linux/aarch64)
	CC="zig cc -target aarch64-linux-musl"
	;;
Linux/*)
	echo "unsupported linux arch: $ARCH" >&2
	exit 1
	;;
Darwin/*)
	CC="clang"
	;;
*)
	echo "unsupported os: $OS (on windows use: go build .)" >&2
	exit 1
	;;
esac

if [ "$OS" = "Linux" ] && ! command -v zig >/dev/null 2>&1; then
	echo "zig not found on PATH - install it from https://ziglang.org/download/" >&2
	exit 1
fi

CGO_ENABLED=1 CC="$CC" go build -trimpath -ldflags "-s -w" -o pummel .
echo "built ./pummel"
