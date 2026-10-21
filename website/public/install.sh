#!/bin/sh
# Installs Moonshed on Linux as a systemd service:
#   curl -fsSL https://moonshed.siddhesh.me/install.sh | sh
#
# It downloads the release archive from GitHub, checks it against the release's
# SHA256SUMS, unpacks it and runs `moonshed install -e production`. Nothing else.
# Source: https://github.com/SiddheshNan/moonshed/blob/main/website/public/install.sh
#
# MOONSHED_DIR      install folder (default /opt/moonshed)
# MOONSHED_VERSION  release tag, e.g. v0.3.0 (default: the newest release)
set -eu

repo="SiddheshNan/moonshed"
dir="${MOONSHED_DIR:-/opt/moonshed}"

fail() {
	echo "moonshed: $*" >&2
	exit 1
}

# Everything runs from main, so a download cut off halfway runs nothing.
main() {
	[ "$(uname -s)" = Linux ] || fail "this script is for Linux. See https://moonshed.siddhesh.me/docs/installation for Windows."
	command -v systemctl > /dev/null || fail "systemd is required"
	command -v curl > /dev/null || fail "curl is required"
	command -v tar > /dev/null || fail "tar is required"
	command -v sha256sum > /dev/null || fail "sha256sum is required"

	case "$(uname -m)" in
		x86_64 | amd64) arch=amd64 ;;
		aarch64 | arm64) arch=arm64 ;;
		*) fail "no release for $(uname -m); only x86-64 and ARM64 are built" ;;
	esac

	sudo=""
	if [ "$(id -u)" -ne 0 ]; then
		command -v sudo > /dev/null || fail "run as root or install sudo"
		sudo="sudo"
	fi

	# Unpacking over an install would replace its config, so upgrades stay manual.
	if [ -e "$dir/moonshed" ]; then
		fail "Moonshed is already installed in $dir. To upgrade, see https://moonshed.siddhesh.me/docs/upgrading"
	fi
	[ ! -e "$dir" ] || fail "$dir already exists; remove it or set MOONSHED_DIR"

	# Every release is a pre-release until 1.0, so releases/latest is empty.
	tag="${MOONSHED_VERSION:-$(curl -fsSL "https://api.github.com/repos/$repo/releases?per_page=1" | grep -m1 '"tag_name"' | cut -d'"' -f4)}"
	[ -n "$tag" ] || fail "could not find the latest release"

	echo "Installing Moonshed $tag (linux-$arch) into $dir"
	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT
	archive="moonshed-linux-$arch.tar.gz"
	curl -fsSL -o "$tmp/$archive" "https://github.com/$repo/releases/download/$tag/$archive"
	curl -fsSL -o "$tmp/SHA256SUMS" "https://github.com/$repo/releases/download/$tag/SHA256SUMS"
	(cd "$tmp" && grep "[ *]$archive$" SHA256SUMS | sha256sum -c -) || fail "checksum mismatch for $archive"
	tar -xzf "$tmp/$archive" -C "$tmp"

	$sudo mkdir -p "$(dirname "$dir")"
	$sudo mv "$tmp/moonshed" "$dir"
	$sudo "$dir/moonshed" install -e production

	echo
	echo "Moonshed is running. Open http://<your-server>:5050 to create the admin account."
	echo "Config: $dir/runtime/config/production.yaml"
}

main "$@"
