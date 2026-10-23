package toolchain

import (
	"context"
	"fmt"
	"net/http"
)

// Pin downloads an archive and returns the asset that pins it: the address, the checksum of the
// archive as downloaded, the member and the checksum of that member. It trusts nothing about the
// download, so it is for the update job, which reads the result as a proposal for a person or a
// verifier to review, never for the shipped program.
func Pin(ctx context.Context, client *http.Client, url, member string) (Asset, error) {
	if !safeMember(member) {
		return Asset{}, fmt.Errorf("member %q must be a relative path inside the archive", member)
	}

	archive, err := download(ctx, client, url)
	if err != nil {
		return Asset{}, err
	}
	binary, err := extract(archive, member)
	if err != nil {
		return Asset{}, fmt.Errorf("%s: %w", url, err)
	}

	return Asset{URL: url, ArchiveSHA256: checksum(archive), Member: member, BinarySHA256: checksum(binary)}, nil
}
