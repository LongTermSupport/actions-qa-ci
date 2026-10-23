// Package installscript holds the install.sh of a release: a POSIX sh script that downloads (or
// takes) the bundle for the machine, checks it against a checksum written into the script, and
// unpacks it. The release fills the checksums and version into this template.
package installscript

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed install.sh
var template []byte

var (
	versionPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	checksumPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// platforms are the platforms the script supports, with the placeholder of each one's checksum.
var platforms = []struct{ name, placeholder string }{
	{"linux_amd64", "@SHA256_LINUX_AMD64@"},
	{"linux_arm64", "@SHA256_LINUX_ARM64@"},
}

// Template is the script as it is kept, with placeholders for the version and the checksums. It
// refuses to run until Fill has replaced them.
func Template() []byte {
	return append([]byte(nil), template...)
}

// Fill makes the script of a release from the version and the text of its checksums.txt, which
// has a line "<sha256>  actions-qa-ci_<version>_<platform>.tar.gz" for each supported platform.
// A missing or repeated line, a checksum that is not 64 lower-case hex digits and a version that
// is not three numbers each refuse it: a script that cannot verify must not be made.
func Fill(version, checksums string) ([]byte, error) {
	if !versionPattern.MatchString(version) {
		return nil, fmt.Errorf("the version %q must be three numbers such as 1.2.3, with no v", version)
	}

	found := map[string]string{}
	for _, line := range strings.Split(checksums, "\n") {
		if line == "" {
			continue
		}
		sum, name, ok := strings.Cut(line, "  ")
		if !ok || !checksumPattern.MatchString(sum) {
			return nil, fmt.Errorf("the checksums line %q is not a sha256 and a file name", line)
		}
		for _, platform := range platforms {
			if name == fmt.Sprintf("actions-qa-ci_%s_%s.tar.gz", version, platform.name) {
				if _, repeated := found[platform.name]; repeated {
					return nil, fmt.Errorf("the checksums name %s more than once", name)
				}
				found[platform.name] = sum
			}
		}
	}

	filled := string(template)
	filled = strings.ReplaceAll(filled, "@VERSION@", version)
	for _, platform := range platforms {
		sum, ok := found[platform.name]
		if !ok {
			return nil, fmt.Errorf("the checksums have no line for actions-qa-ci_%s_%s.tar.gz", version, platform.name)
		}
		filled = strings.ReplaceAll(filled, platform.placeholder, sum)
	}

	return []byte(filled), nil
}
