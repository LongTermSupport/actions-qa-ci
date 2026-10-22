// Package record reads the project record, the one file in which a project states what it
// has accepted: the exceptions to the defences, each with its justification. Nothing else in
// the repository can suppress a finding.
package record

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v4"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// Path is where a project keeps its record, relative to the repository root.
const Path = ".github/actions-qa-ci.yml"

// maxSize bounds a record: a file a person wrote is nowhere near it.
const maxSize = 1 << 20

// Record is a project's record. A field this build does not act on is not accepted, so a
// record never states something that is silently ignored.
type Record struct {
	// Version is the actions-qa-ci version the project pins.
	Version    string      `yaml:"version"`
	Exceptions []Exception `yaml:"exceptions"`
}

// Exception is one accepted finding, or, with no path, one defence disabled everywhere.
type Exception struct {
	// ID is the identifier of the defence the exception is from.
	ID string `yaml:"id"`
	// Path is the file the exception applies to; empty for a defence disabled everywhere.
	Path string `yaml:"path"`
	// Job and Step narrow the exception to a job, and a step in it, by id or name.
	Job  string `yaml:"job"`
	Step string `yaml:"step"`
	// Hazard says what the defence would report here, Scope why that is accepted only here,
	// and Decision where the acceptance was made.
	Hazard   string `yaml:"hazard"`
	Scope    string `yaml:"scope"`
	Decision string `yaml:"decision"`

	// Line is where the entry starts in the record file.
	Line int `yaml:"-"`
}

// DisablesDefence says whether the exception turns a defence off everywhere rather than
// accepting a finding in one place.
func (e Exception) DisablesDefence() bool {
	return e.Path == ""
}

// Load reads and validates the record at path. Any problem is named by the file.
func Load(path string, known func(id string) bool) (*Record, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read the project record: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a file", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read the project record: %w", err)
	}
	defer closeQuietly(file)

	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	rec, err := Parse(data, known)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return rec, nil
}

// Parse reads a record and refuses anything that is not exactly well-formed, reporting every
// fault at once. known says whether an identifier belongs to a defence this build has.
func Parse(data []byte, known func(id string) bool) (*Record, error) {
	if len(data) > maxSize {
		return nil, fmt.Errorf("the project record is larger than %d bytes", maxSize)
	}

	rec := &Record{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(rec); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("the project record is not valid: %w", err)
	}
	if err := decoder.Decode(&yaml.Node{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("the project record is not valid: it has more than one document")
	}

	lines, err := entryLines(data)
	if err != nil {
		return nil, err
	}
	for i := range rec.Exceptions {
		if i < len(lines) {
			rec.Exceptions[i].Line = lines[i]
		}
	}

	if faults := rec.faults(known); len(faults) > 0 {
		return nil, fmt.Errorf("the project record is not valid:\n  - %s", strings.Join(faults, "\n  - "))
	}

	return rec, nil
}

// entryLines finds the line each exception starts on.
func entryLines(data []byte) ([]int, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("the project record is not valid: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}

	top := root.Content[0].Content
	for i := 0; i+1 < len(top); i += 2 {
		if top[i].Value != "exceptions" || top[i+1].Kind != yaml.SequenceNode {
			continue
		}
		lines := make([]int, 0, len(top[i+1].Content))
		for _, item := range top[i+1].Content {
			lines = append(lines, item.Line)
		}

		return lines, nil
	}

	return nil, nil
}

func (r *Record) faults(known func(id string) bool) []string {
	var faults []string

	if strings.TrimSpace(r.Version) == "" {
		faults = append(faults, "version is missing: state the actions-qa-ci version the project pins")
	}

	for i, e := range r.Exceptions {
		where := fmt.Sprintf("exception %d (line %d)", i+1, e.Line)
		for _, fault := range e.shapeFaults(known) {
			faults = append(faults, where+": "+fault)
		}
	}

	return faults
}

func (e Exception) shapeFaults(known func(id string) bool) []string {
	var faults []string

	switch {
	case e.ID == "":
		faults = append(faults, "id is missing")
	default:
		if _, err := finding.ClassifyIdentifier(e.ID); err != nil {
			faults = append(faults, err.Error())
		} else if !known(e.ID) {
			faults = append(faults, fmt.Sprintf("id %q is unknown: no defence in this build has it", e.ID))
		}
	}

	if problem := pathProblem(e.Path); problem != "" {
		faults = append(faults, fmt.Sprintf("path %q %s", e.Path, problem))
	}
	if e.Path == "" && (e.Job != "" || e.Step != "") {
		faults = append(faults, "a job or step needs a path to be in")
	}

	return faults
}

// pathProblem says why a string cannot be a repository-relative file path, or returns "".
func pathProblem(path string) string {
	switch {
	case path == "":
		return ""
	case strings.HasPrefix(path, "/"):
		return "is absolute; use a path relative to the repository root"
	case strings.ContainsRune(path, '\\'):
		return "contains a backslash; use forward slashes"
	case strings.ContainsFunc(path, unicode.IsControl):
		return "contains a control character"
	case strings.ContainsAny(path, "*?[]{}"):
		return "contains a glob character; an exception names one file"
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return "is not a clean path inside the repository"
		}
	}

	return ""
}

func closeQuietly(file *os.File) {
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "actions-qa: cannot close %s: %v\n", file.Name(), err)
	}
}
