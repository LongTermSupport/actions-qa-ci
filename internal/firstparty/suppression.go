package firstparty

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
	"github.com/LongTermSupport/actions-qa-ci/internal/registry"
)

// The identifiers of the checks that close the routes around the project record.
const (
	InlineSuppression       = finding.ToolPrefix + ".inlineSuppression"
	NativeSuppressionConfig = finding.ToolPrefix + ".nativeSuppressionConfig"
)

// inlineForms are the comments with which a tool is told to leave a finding out, in the place
// the finding is about. Matching is not case sensitive and ignores the spaces around the colon,
// so a spelling the tool reads cannot slip past.
var inlineForms = []struct {
	tool    string
	pattern *regexp.Regexp
}{
	{"zizmor", regexp.MustCompile(`(?i)#\s*zizmor\s*:\s*ignore\b`)},
	{"ratchet", regexp.MustCompile(`(?i)#\s*ratchet\s*:\s*exclude\b`)},
	{"ShellCheck", regexp.MustCompile(`(?i)#\s*shellcheck\b[^\n]*?\bdisable\s*=`)},
}

// CheckInlineSuppression is the check on one workflow or action file: a finding for each
// comment that tells zizmor, ratchet or ShellCheck to leave a finding out. A suppression is the
// project record's to make, with a justification, and not a comment at the call site.
func CheckInlineSuppression(file string) ([]finding.Finding, error) {
	data, err := os.ReadFile(filepath.FromSlash(file))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", file, err)
	}

	var found []finding.Finding
	for number, line := range yamlLines(string(data)) {
		for _, form := range inlineForms {
			for _, at := range form.pattern.FindAllStringIndex(line, -1) {
				message := fmt.Sprintf("a %s comment leaves a finding out with no recorded justification; remove it, and if the finding is accepted put an exception in .github/actions-qa-ci.yml", form.tool)
				f, err := suppressionFinding(InlineSuppression, file, number+1, utf8.RuneCountInString(line[:at[0]])+1, message)
				if err != nil {
					return nil, err
				}
				found = append(found, f)
			}
		}
	}
	finding.Sort(found)

	return found, nil
}

// yamlLines splits text into the lines the YAML reader counts: a line feed, a carriage return,
// both together, or the Unicode next line, line separator or paragraph separator ends a line.
func yamlLines(text string) []string {
	var lines []string
	start := 0

	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		switch {
		case r == '\r' && i+1 < len(text) && text[i+1] == '\n':
			lines = append(lines, text[start:i])
			i += 2
			start = i

			continue
		case r == '\n', r == '\r', r == 0x85, r == 0x2028, r == 0x2029:
			lines = append(lines, text[start:i])
			start = i + size
		}
		i += size
	}

	return append(lines, text[start:])
}

// suppressionKeys are the keys of each tool's configuration with which it is told to leave a
// finding out. A tool not listed is forbidden outright when its file exists.
var suppressionKeys = map[string]string{
	"actionlint": "ignore",
	"pinact":     "ignore_actions",
}

// CheckNativeSuppressionConfig is the check on one configuration file of a bundled tool. zizmor
// reads its file wherever it finds one, so the file's existence is the finding; actionlint's and
// pinact's files also carry calibrations the project record has a place for, so a finding is the
// key that suppresses, wherever in the file it is.
func CheckNativeSuppressionConfig(file string) ([]finding.Finding, error) {
	tool := toolOf(file)
	if tool == "" {
		return nil, fmt.Errorf("%s is not a configuration file of a tool this toolchain bundles", file)
	}

	data, err := os.ReadFile(filepath.FromSlash(file))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", file, err)
	}

	key, byKey := suppressionKeys[tool]
	if !byKey {
		message := fmt.Sprintf("%s reads this file and honours the findings it ignores or disables, with no recorded justification; delete it and put a calibration or an exception in .github/actions-qa-ci.yml", tool)
		f, err := suppressionFinding(NativeSuppressionConfig, file, 1, 1, message)
		if err != nil {
			return nil, err
		}

		return []finding.Finding{f}, nil
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var found []finding.Finding
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read %s as YAML: %w", file, err)
		}

		var keyed []*yaml.Node
		collectKeys(&document, key, &keyed)
		for _, node := range keyed {
			message := fmt.Sprintf("the %q key of %s's configuration leaves findings out with no recorded justification; remove it, and put an exception in .github/actions-qa-ci.yml", key, tool)
			f, err := suppressionFinding(NativeSuppressionConfig, file, node.Line, node.Column, message)
			if err != nil {
				return nil, err
			}
			found = append(found, f)
		}
	}
	finding.Sort(found)

	return found, nil
}

// toolOf names the bundled tool whose configuration file this is, by the name the tool looks
// for, or is empty for a file that is none.
func toolOf(file string) string {
	name := strings.TrimPrefix(path.Base(file), ".")
	for _, tool := range []string{"zizmor", "actionlint", "pinact"} {
		if strings.HasPrefix(name, tool+".") {
			return tool
		}
	}

	return ""
}

// collectKeys gathers the nodes of every mapping key with this name, at any depth. A key written
// as an alias counts as the scalar it stands for, and is reported where the alias is. An alias
// is otherwise not followed: what it stands for is found where it is written.
func collectKeys(node *yaml.Node, name string, into *[]*yaml.Node) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			named := key
			if key.Kind == yaml.AliasNode && key.Alias != nil {
				named = key.Alias
			}
			if named.Kind == yaml.ScalarNode && named.Value == name {
				*into = append(*into, key)
			}
		}
	}
	if node.Kind == yaml.AliasNode {
		return
	}
	for _, child := range node.Content {
		collectKeys(child, name, into)
	}
}

func suppressionFinding(id, file string, line, col int, message string) (finding.Finding, error) {
	f := finding.Finding{
		ID:       id,
		Tool:     Tool,
		Path:     file,
		Line:     max(line, 1),
		Col:      max(col, 1),
		Severity: finding.SeverityError,
		Message:  message,
		DocPath:  registry.DocPath(id),
	}
	if err := f.Validate(); err != nil {
		return finding.Finding{}, fmt.Errorf("the suppression check produced a finding that cannot be reported: %w", err)
	}

	return f, nil
}
