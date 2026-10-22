package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/LongTermSupport/actions-qa-ci/internal/agentsummary"
	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/listing"
)

// summaryFileLimit bounds a file the summary is put into: a file a person reads is nowhere near it.
const summaryFileLimit = 8 << 20

// writeAgentSummary puts the active defences into the generated region of the file, keeping
// everything else in it. A file whose region is broken is refused as it is, not rewritten.
func writeAgentSummary(typed string, entries []listing.Entry, stderr io.Writer) int {
	region, err := agentsummary.Region(entries)
	if err != nil {
		return fault(stderr, err)
	}

	existing, err := readSummaryFile(typed)
	if err != nil {
		return refusal(stderr, err.Error())
	}
	content, err := agentsummary.Splice(existing, region)
	if err != nil {
		return refusal(stderr, fmt.Sprintf("cannot put the summary in %s: %v", finding.DisplayPath(typed), err))
	}

	if err := writeOutputFile(typed, content); err != nil {
		return fault(stderr, err)
	}
	if _, err := fmt.Fprintf(stderr, "actions-qa: wrote the agent summary to %s\n", finding.DisplayPath(typed)); err != nil {
		return ExitToolchainFault
	}

	return ExitOK
}

// readSummaryFile reads the file the summary goes into, which may not exist yet. It never reads
// through a symbolic link or from anything but a regular file.
func readSummaryFile(typed string) ([]byte, error) {
	file, err := os.OpenFile(typed, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case errors.Is(err, syscall.ELOOP):
		return nil, fmt.Errorf("cannot write %s: it is a symbolic link", finding.DisplayPath(typed))
	case err != nil:
		return nil, fmt.Errorf("cannot read %s: %w", finding.DisplayPath(typed), err)
	}

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.Join(fmt.Errorf("cannot write %s: it is not a regular file", finding.DisplayPath(typed)), file.Close())
	}
	if info.Size() > summaryFileLimit {
		return nil, errors.Join(fmt.Errorf("cannot write %s: it is larger than %d bytes", finding.DisplayPath(typed), summaryFileLimit), file.Close())
	}

	data, readErr := io.ReadAll(io.LimitReader(file, summaryFileLimit+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", finding.DisplayPath(typed), err)
	}

	return data, nil
}
