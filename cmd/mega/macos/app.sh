#!/bin/sh
# builds the macOS application: a menu bar app that runs the daemon, with its
# own window, and the claude shim inside it. `make app` runs it from the
# repository root, after building the page.
#   app.sh APP    e.g. build/mega.app
#
# the binary is the bundle's executable, never a script that starts it: on
# macOS 26 a script there leaves the menu bar item unshown. so nothing runs
# the operator's shell first; mega resolves it for each project itself.
set -eu

if [ "$(uname -s)" != Darwin ]; then
	echo "app.sh builds the macOS application, on macOS only" >&2
	exit 1
fi

app=${1:?usage: app.sh APP}
root=$(pwd)
macos="$root/cmd/mega/macos"
version=$(git -C "$root" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
version=${version:-0.0.0}

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources/shim/claude"

# unstamped: the binary finds the shim in Contents/Resources, not a checkout.
CGO_ENABLED=1 go build -o "$app/Contents/MacOS/mega" ./cmd/mega

# the shim and exactly its locked dependencies, without the sdk's optional
# native builds of claude code, which mega never runs.
shim="$app/Contents/Resources/shim/claude"
cp shim/claude/shim.mjs shim/claude/package.json shim/claude/package-lock.json "$shim/"
npm --prefix "$shim" ci --omit=optional --no-audit --no-fund

sed "s/@VERSION@/$version/g" "$macos/Info.plist" > "$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist" >/dev/null

# the icon, from the hub's 512-pixel rendering (render-icons.mjs renders none
# larger, so the 512@2x slot stays empty).
iconset=$(mktemp -d)/mega.iconset
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
	sips -z $size $size cmd/mega/desktop-icon-512.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
	double=$((size * 2))
	if [ $double -le 512 ]; then
		sips -z $double $double cmd/mega/desktop-icon-512.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
	fi
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/mega.icns"
rm -rf "$(dirname "$iconset")"

# ad hoc: enough to run on the machine that built it, nothing more.
codesign --force --deep --sign - "$app"
codesign --verify --deep --strict "$app"
echo "built '$app' ($version)"
