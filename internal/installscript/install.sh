#!/bin/sh
# install.sh for actions-qa-ci @VERSION@
#
# Installs the actions-qa-ci bundle for this machine after checking it against the checksum
# written into this script when the release was made. Download this file, read it, then run it:
# it is not meant to be piped into a shell.
#
#   sh install.sh [--prefix DIR]
#
# Environment:
#   ACTIONS_QA_CI_BUNDLE        use this local tarball instead of downloading one
#   ACTIONS_QA_CI_DOWNLOAD_URL  download from this address instead of the release's
#   ACTIONS_QA_CI_HOME          install into a directory named for the version under this one
#   XDG_DATA_HOME               otherwise under $XDG_DATA_HOME/actions-qa-ci, or under
#                               $HOME/.local/share/actions-qa-ci

set -eu

VERSION='@VERSION@'
SHA256_LINUX_AMD64='@SHA256_LINUX_AMD64@'
SHA256_LINUX_ARM64='@SHA256_LINUX_ARM64@'

fail() {
	printf 'install.sh: %s\n' "$*" >&2
	exit 1
}

case $VERSION in
@*) fail "this is the template of install.sh, not the script of a release; it has no version or checksum to verify against" ;;
esac

prefix=
prefix_given=no
while [ $# -gt 0 ]; do
	case $1 in
	--prefix)
		[ $# -ge 2 ] || fail "--prefix needs a directory"
		prefix=$2
		prefix_given=yes
		shift 2
		;;
	--prefix=*)
		prefix=${1#--prefix=}
		prefix_given=yes
		shift
		;;
	-h | --help)
		printf 'usage: sh install.sh [--prefix DIR]\n'
		exit 0
		;;
	*) fail "unknown argument $1" ;;
	esac
done

if [ "$prefix_given" = yes ] && [ -z "$prefix" ]; then
	fail "--prefix needs a directory, and was given an empty one"
fi

system=$(uname -s)
machine=$(uname -m)
if [ "$system" != Linux ]; then
	fail "unsupported platform $system $machine: only Linux is supported"
fi
case $machine in
x86_64 | amd64)
	platform=linux_amd64
	expected=$SHA256_LINUX_AMD64
	;;
aarch64 | arm64)
	platform=linux_arm64
	expected=$SHA256_LINUX_ARM64
	;;
*) fail "unsupported platform $system $machine: only amd64 and arm64 are supported" ;;
esac
asset=actions-qa-ci_${VERSION}_${platform}.tar.gz

if [ -n "$prefix" ]; then
	dest=$prefix
else
	if [ -n "${ACTIONS_QA_CI_HOME:-}" ]; then
		base=$ACTIONS_QA_CI_HOME
	elif [ -n "${XDG_DATA_HOME:-}" ]; then
		base=$XDG_DATA_HOME/actions-qa-ci
	elif [ -n "${HOME:-}" ]; then
		base=$HOME/.local/share/actions-qa-ci
	else
		fail "cannot tell where to install: set HOME, ACTIONS_QA_CI_HOME or pass --prefix"
	fi
	dest=$base/$VERSION
fi
case $dest in
/*) ;;
*) dest=$PWD/$dest ;;
esac
if [ -e "$dest" ]; then
	fail "actions-qa-ci $VERSION is already installed at $dest; remove it first to install again"
fi

if command -v sha256sum >/dev/null; then
	hash_tool=sha256sum
elif command -v shasum >/dev/null; then
	hash_tool=shasum
elif command -v openssl >/dev/null; then
	hash_tool=openssl
else
	fail "needs sha256sum, shasum or openssl to check the bundle, and none is installed"
fi

checksum_of() {
	case $hash_tool in
	sha256sum) sha256sum "$1" | cut -d ' ' -f 1 ;;
	shasum) shasum -a 256 "$1" | cut -d ' ' -f 1 ;;
	openssl) openssl dgst -sha256 -r "$1" | cut -d ' ' -f 1 ;;
	esac
}

work=
staging=
cleanup() {
	if [ -n "$work" ]; then rm -rf "$work"; fi
	if [ -n "$staging" ]; then rm -rf "$staging"; fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

# Each directory is named before it is made, so that a signal at any point finds the name in the
# variable cleanup reads. A name that is already taken is refused, never reused.
work=${TMPDIR:-/tmp}/actions-qa-ci-install.$$
if ! mkdir -m 700 "$work"; then
	work=
	fail "cannot make the working directory ${TMPDIR:-/tmp}/actions-qa-ci-install.$$; remove it if it is left from an earlier run"
fi
archive=$work/$asset

if [ -n "${ACTIONS_QA_CI_BUNDLE:-}" ]; then
	[ -f "$ACTIONS_QA_CI_BUNDLE" ] || fail "ACTIONS_QA_CI_BUNDLE ($ACTIONS_QA_CI_BUNDLE) is not a file"
	cp -- "$ACTIONS_QA_CI_BUNDLE" "$archive"
else
	base_url=${ACTIONS_QA_CI_DOWNLOAD_URL:-https://github.com/LongTermSupport/actions-qa-ci/releases/download/v$VERSION}
	url=${base_url%/}/$asset
	if command -v curl >/dev/null; then
		curl --fail --silent --show-error --location --output "$archive" "$url" || fail "could not download $url"
	elif command -v wget >/dev/null; then
		wget --quiet --output-document="$archive" "$url" || fail "could not download $url"
	else
		fail "needs curl or wget to download $url, and neither is installed; download it elsewhere and set ACTIONS_QA_CI_BUNDLE"
	fi
fi

actual=$(checksum_of "$archive")
if [ "$actual" != "$expected" ]; then
	fail "the checksum of $asset is '$actual', not the '$expected' written into this script; nothing was installed"
fi

parent=$(dirname "$dest")
mkdir -p "$parent"
staging=$parent/.actions-qa-ci-install.$$
if ! mkdir -m 700 "$staging"; then
	staging=
	fail "cannot make the staging directory $parent/.actions-qa-ci-install.$$; remove it if it is left from an earlier run"
fi
tar -xzf "$archive" -C "$staging" || fail "could not unpack $asset; nothing was installed"
[ -x "$staging/bin/actions-qa" ] || fail "$asset holds no bin/actions-qa; nothing was installed"
chmod 755 "$staging"
if [ -e "$dest" ]; then
	fail "actions-qa-ci $VERSION is already installed at $dest; remove it first to install again"
fi
mv "$staging" "$dest"
staging=

printf 'Installed actions-qa-ci %s in %s\n' "$VERSION" "$dest"
printf 'Put %s on your PATH, then run: actions-qa install\n' "$dest/bin"
