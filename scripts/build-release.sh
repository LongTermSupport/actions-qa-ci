#!/bin/sh
# Builds the assets of one release into a new directory: the bundle of each platform,
# checksums.txt over both, and the install.sh that verifies them. The release workflow publishes
# what it writes; CI runs it for a trial version so the same path is proven on every pull request.
#
#   sh scripts/build-release.sh X.Y.Z <new output directory>
#
# Run from the repository root. Needs go and jq.

set -eu

if [ $# -ne 2 ]; then
	printf 'usage: sh scripts/build-release.sh X.Y.Z <new output directory>\n' >&2
	exit 2
fi
version=$1
out=$2
if [ -e "$out" ]; then
	printf 'build-release.sh: %s exists; a release is written into a new directory\n' "$out" >&2
	exit 2
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# The program's licence is LICENSE; each pinned tool's is under licences/. ShellCheck is GPL-3.0
# and shipped as a separate program beside actions-qa, so the bundle names its source.
mkdir "$work/licences"
cp licences/*.txt "$work/licences/"
cp LICENSE "$work/licences/actions-qa-ci.txt"
shellcheck_tag=$(jq -er '.tools.shellcheck.version' internal/toolchain/manifest.json)
printf 'ShellCheck %s is distributed in this bundle under the GPL-3.0 (shellcheck.txt), as a separate\nprogram aggregated with actions-qa. Its corresponding source is the tag %s of\nhttps://github.com/koalaman/shellcheck\n' \
	"$shellcheck_tag" "$shellcheck_tag" >"$work/licences/shellcheck-source.txt"

mkdir -p "$out"
: >"$work/checksums.txt"
for platform in linux_amd64 linux_arm64; do
	arch=${platform#linux_}
	mkdir -p "$work/$platform/tools"
	go run ./cmd/actions-qa-dev fetch --platform "$platform" "$work/$platform/tools"
	CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -o "$work/$platform/actions-qa" ./cmd/actions-qa
	go run ./cmd/actions-qa-dev bundle --version "$version" --platform "$platform" \
		--program "$work/$platform/actions-qa" --tools "$work/$platform/tools" \
		--licences "$work/licences" --out "$work/$platform/release"
	cp "$work/$platform/release/actions-qa-ci_${version}_${platform}.tar.gz" "$out/"
	cat "$work/$platform/release/checksums.txt" >>"$work/checksums.txt"
done
sort -k 2 "$work/checksums.txt" >"$out/checksums.txt"
go run ./cmd/actions-qa-dev install-script --version "$version" --checksums "$out/checksums.txt" --out "$out/install.sh"
