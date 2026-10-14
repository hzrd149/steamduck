#!/usr/bin/env bash
# steamduck installer for SteamOS (Steam Deck). Run it in desktop mode, in
# Konsole, as the deck user:
#
#   curl -fsSL https://raw.githubusercontent.com/hzrd149/steamduck/master/install.sh | bash
#
# It downloads one steamduck release (the steamduck binary, a kwakore
# bundle and kwakore's install helper), checks every file against the
# release's SHA256SUMS, and runs `steamduck install`. Everything goes into
# ~/.local and ~/.config; no root, and SteamOS updates leave it in place.
# Run it again to update.
set -euo pipefail

repo="hzrd149/steamduck"
channel=""      # "" = latest release, else nightly; or a tag
restart="ask"   # restart Steam so the new shortcuts show up: ask, yes, no
modify_path="ask"  # add ~/.local/bin to PATH in the shell's startup file: ask, yes, no

usage() {
	cat <<'EOF'
Usage: install.sh [--nightly | --version TAG] [--restart-steam | --no-restart-steam]
                  [--modify-path | --no-modify-path]

  --nightly             install the rolling build of master
  --version TAG         install release TAG (such as v0.1.0)
  --restart-steam       restart Steam without asking, to show the shortcuts
  --no-restart-steam    leave Steam running; shortcuts appear after it restarts
  --modify-path         add ~/.local/bin to PATH in your shell's startup file without asking
  --no-modify-path      leave your shell's startup files alone

Without --nightly or --version the latest release is installed, or the
nightly build when there is no release yet.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--nightly) channel="nightly"; shift ;;
	--version) channel="${2:?--version needs a tag}"; shift 2 ;;
	--restart-steam) restart="yes"; shift ;;
	--no-restart-steam) restart="no"; shift ;;
	--modify-path) modify_path="yes"; shift ;;
	--no-modify-path) modify_path="no"; shift ;;
	-h | --help) usage; exit 0 ;;
	*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

# ask QUESTION succeeds unless the user answers n. It reads the terminal,
# because stdin is this script when it is piped from curl; with no
# terminal the answer is no.
ask() {
	local answer
	if { exec 3<>/dev/tty; } 2>/dev/null; then
		printf '%s [Y/n] ' "$1" >&3
		read -r answer <&3 || answer="n"
		exec 3>&-
		case "$answer" in [nN]*) return 1 ;; *) return 0 ;; esac
	fi
	return 1
}

say() { printf '\033[1;33m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -ne 0 ] || fail "run this as your normal user (deck), not with sudo"
[ "$(uname -s)" = Linux ] || fail "steamduck runs on Linux (SteamOS)"
[ "$(uname -m)" = x86_64 ] || fail "only x86_64 builds are published (the Steam Deck is x86_64)"
for tool in curl sha256sum tar gzip systemctl flock od stat awk; do
	command -v "$tool" >/dev/null || fail "$tool not found"
done
systemctl --user show-environment >/dev/null 2>&1 ||
	fail "systemctl --user does not work here; run this from a desktop-mode terminal, not over sudo/su"

if ! grep -qs '^ID=steamos' /etc/os-release; then
	say "this is not SteamOS; continuing anyway"
fi

base="https://github.com/$repo/releases"
if [ -z "$channel" ]; then
	if curl -fsSLI -o /dev/null "$base/latest/download/SHA256SUMS" 2>/dev/null; then
		url="$base/latest/download"
		say "installing the latest steamduck release"
	else
		url="$base/download/nightly"
		say "no steamduck release yet; installing the nightly build"
	fi
else
	url="$base/download/$channel"
	say "installing steamduck $channel"
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/steamduck-install.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

files=(steamduck-linux-amd64 kwakore-linux-amd64.tar.gz kwakore-install.sh)
for f in SHA256SUMS "${files[@]}"; do
	say "downloading $f"
	curl -fSL --retry 3 --proto '=https' -o "$work/$f" "$url/$f" ||
		fail "could not download $url/$f"
done

say "verifying checksums"
for f in "${files[@]}"; do
	want=$(awk -v name="$f" '$2 == name || $2 == "*" name { print $1 }' "$work/SHA256SUMS")
	[ -n "$want" ] || fail "SHA256SUMS has no entry for $f"
	got=$(sha256sum -- "$work/$f" | awk '{ print $1 }')
	[ "$want" = "$got" ] || fail "checksum mismatch for $f"
done

chmod 0755 "$work/steamduck-linux-amd64"
# kwakore's helper refuses windows below group-writable directories; fix the
# usual culprit before it fails half way.
mkdir -p "$HOME/.local"
chmod go-w "$HOME/.local" 2>/dev/null || true

say "installing kwakore and steamduck"
# kwakore's helper reads the SHA256SUMS beside the archive
"$work/steamduck-linux-amd64" install \
	--kwakore-archive "$work/kwakore-linux-amd64.tar.gz" \
	--kwakore-installer "$work/kwakore-install.sh"

steamduck="$HOME/.local/lib/steamduck/steamduck"

say "checking the runtime"
"$steamduck" doctor || true

# Steam reads shortcuts.vdf only when it starts.
if pgrep -x steam >/dev/null 2>&1; then
	if [ "$restart" = ask ]; then
		if ask "Steam must restart to show napplets in your library. Restart Steam now?"; then
			restart="yes"
		else
			restart="no"
		fi
	fi
	if [ "$restart" = yes ]; then
		say "restarting Steam to add the napplet shortcuts"
		"$steamduck" sync --restart-steam
	else
		say "napplet shortcuts are added the next time Steam starts"
	fi
else
	"$steamduck" sync
fi

# Put ~/.local/bin (steamduck and kwak) on PATH for new terminals, in the
# startup file of the user's shell. A marker line keeps reruns from adding
# it twice.
path_marker="# Added by the steamduck installer"
add_to_path() {
	local shell rc
	shell=$(basename "${SHELL:-bash}")
	case "$shell" in
	fish)
		rc="${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/steamduck.fish"
		if [ -f "$rc" ]; then
			say "~/.local/bin is already added to PATH in $rc; open a new terminal to use it"
			return 0
		else
			mkdir -p "$(dirname "$rc")"
			cat >"$rc" <<'FISH'
# Added by the steamduck installer
contains -- $HOME/.local/bin $PATH; or set -gx PATH $HOME/.local/bin $PATH
FISH
		fi
		;;
	*)
		if [ "$shell" = zsh ]; then rc="$HOME/.zshrc"; else rc="$HOME/.bashrc"; fi
		if grep -qsF "$path_marker" "$rc"; then
			say "~/.local/bin is already added to PATH in $rc; open a new terminal to use it"
			return 0
		else
			cat >>"$rc" <<'SH'

# Added by the steamduck installer
case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) export PATH="$HOME/.local/bin:$PATH" ;; esac
SH
		fi
		;;
	esac
	say "added ~/.local/bin to PATH in $rc; it takes effect in new terminals"
}

case ":$PATH:" in
*":$HOME/.local/bin:"*) ;;
*)
	if [ "$modify_path" = ask ]; then
		if ask "Add ~/.local/bin to your PATH, so \"steamduck\" and \"kwak\" work in any terminal?"; then
			modify_path="yes"
		else
			modify_path="no"
		fi
	fi
	if [ "$modify_path" = yes ]; then
		add_to_path
	else
		say "~/.local/bin is not on your PATH; run steamduck as $steamduck"
	fi
	;;
esac

cat <<EOF

steamduck is installed.

  - Your installed napplets and "Napplet Store" are in the Steam library,
    in the "Napplets" collection (switch to Game Mode to try them).
  - Run "steamduck doctor" in a terminal to check the setup again.
  - Logs from games launched by Steam: ~/.local/share/steamduck/steamduck.log
  - Update: run this installer again. Remove: steamduck uninstall

EOF
