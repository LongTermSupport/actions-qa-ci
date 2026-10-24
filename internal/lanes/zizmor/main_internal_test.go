package zizmor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/LongTermSupport/actions-qa-ci/internal/toolchain"
)

// TestMain points the lane at the pinned binaries a development checkout fetched into its bin
// directory (go run ./cmd/actions-qa-dev fetch bin), unless the environment already says where
// they are. The lane fails when the binary is absent, so a missing fetch fails the tests.
func TestMain(m *testing.M) {
	if os.Getenv(toolchain.ToolsDirectoryVariable) == "" {
		bin, err := filepath.Abs(filepath.Join("..", "..", "..", "bin"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot find the repository's bin directory: %v\n", err)
			os.Exit(1)
		}
		if err := os.Setenv(toolchain.ToolsDirectoryVariable, bin); err != nil {
			fmt.Fprintf(os.Stderr, "cannot set %s: %v\n", toolchain.ToolsDirectoryVariable, err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}
