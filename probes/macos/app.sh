#!/bin/sh
# probes the macOS application as Finder would start it, against scratch
# folders and a scratch port, never the operator's own mega. from the
# repository's root, after `make app`:
#   probes/macos/app.sh [APP] [PORT]
# it reports: whether a bare launch ran the daemon and opened the window,
# its LANG and PATH, how long the operator's shell took to resolve, where
# the harnesses were found, whether a menu bar item appeared, whether the
# window took the front from Finder, and whether opening the application
# again opened another window. it quits the application and removes its
# scratch at the end.
set -u
root=$(pwd)
app=${1:-build/mega.app}
port=${2:-18471}
scratch=$(mktemp -d)
bin="$app/Contents/MacOS/mega"
out="$scratch/menubar"
cc -o "$out" "$root/probes/macos/menubar.c" -framework CoreGraphics -framework CoreFoundation || exit 1

if pgrep -f "$bin" >/dev/null; then
	echo "this application is already running; quit it first" >&2
	exit 1
fi
mkdir -p "$scratch/config/mega" "$scratch/data" "$scratch/state"
printf 'listen: 127.0.0.1:%s\nname: probe\nenvironment:\n  timeout: 90s\n' "$port" > "$scratch/config/mega/mega.yaml"
log="$scratch/state/mega/mega.log"
msgs() { grep -o '"msg":"[^"]*' "$log" | cut -c8-; }
items() { "$out" | grep -c 'onscreen=1'; }

before=$(items)
open -a Finder
sleep 1.5
# what Finder gives an application: launchd's environment, and no LANG.
env -i HOME="$HOME" USER="$USER" LOGNAME="$USER" SHELL="${SHELL:-/bin/zsh}" TMPDIR="${TMPDIR:-/tmp}" \
	PATH=/usr/bin:/bin:/usr/sbin:/sbin /usr/bin/open \
	--env XDG_CONFIG_HOME="$scratch/config" --env XDG_DATA_HOME="$scratch/data" --env XDG_STATE_HOME="$scratch/state" \
	"$app"
for _ in $(seq 1 90); do
	sleep 1
	if grep -q "harness 'claude'" "$log" 2>/dev/null; then
		break
	fi
done
sleep 1
echo "launch:     $(msgs | grep -c 'daemon serving') daemon, $(pgrep -f "$bin window" | wc -l | tr -d ' ') window"
echo "LANG:       $(msgs | grep '^LANG' || echo 'already set')"
echo "PATH:       $(msgs | grep "own PATH" | sed "s/.*: //")"
start=$(grep 'resolve through' "$log" | grep -o '"time":"[^"]*' | cut -c9-)
found=$(grep "harness 'claude'" "$log" | grep -o '"time":"[^"]*' | cut -c9-)
echo "shell:      resolved between $start and $found"
msgs | grep '^harness' | cut -c1-160 | sed 's/^/harness:    /'
echo "menu bar:   $(( $(items) - before )) more items on screen"
front=$(lsappinfo front)
echo "front:      $(lsappinfo info -only name "$front" | cut -d= -f2) ($(lsappinfo info -only applicationtype "$front" | cut -d= -f2))"

pkill -f "$bin window"
sleep 1.5
open -a Finder
sleep 1.5
open "$app"
sleep 3
front=$(lsappinfo front)
echo "reopen:     $(pgrep -f "$bin\$" | wc -l | tr -d ' ') daemon, $(pgrep -f "$bin window" | wc -l | tr -d ' ') window, front $(lsappinfo info -only name "$front" | cut -d= -f2)"

pkill -f "$bin"
sleep 2
rm -rf "$scratch"
