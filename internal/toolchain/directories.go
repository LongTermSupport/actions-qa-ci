package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
)

// ToolsDirectoryVariable names the environment variable that points the program at the
// directory holding its pinned binaries. It exists so a test or a development checkout can use
// a directory other than the one the program was installed in.
const ToolsDirectoryVariable = "ACTIONS_QA_CI_TOOLS_DIR"

// Directories lists where the program looks for its pinned binaries: the directory named by
// ToolsDirectoryVariable alone when it is set, otherwise the directory the running program is
// in, which is where the release bundle puts them. Every directory is absolute.
func Directories() ([]string, error) {
	if dir := os.Getenv(ToolsDirectoryVariable); dir != "" {
		if !filepath.IsAbs(dir) {
			return nil, fmt.Errorf("%s must be an absolute path, got %q", ToolsDirectoryVariable, dir)
		}

		return []string{dir}, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot find the running program: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve the running program %s: %w", executable, err)
	}

	return []string{filepath.Dir(resolved)}, nil
}
