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

// ErrInvalid marks a record that was read and is not a valid one, as opposed to a file that
// could not be read: the first is a mistake in the record, the second a fault of the machine.
var ErrInvalid = errors.New("the project record is not valid")

// Path is where a project keeps its record, relative to the repository root.
const Path = ".github/actions-qa-ci.yml"

// document is a record as the YAML decoder reads it, before its entries are checked.
type document struct {
	Version    string       `yaml:"version"`
	Exceptions []*Exception `yaml:"exceptions"`
}

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

	// null marks an entry written as nothing at all (a bare dash, null or ~), which Parse
	// reports once and which has no fields to judge.
	null bool
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
// fault at once. known says whether an identifier belongs to a defence this build has. A
// refusal wraps ErrInvalid.
func Parse(data []byte, known func(id string) bool) (*Record, error) {
	if len(data) > maxSize {
		return nil, fmt.Errorf("%w: it is larger than %d bytes", ErrInvalid, maxSize)
	}

	doc := document{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := decoder.Decode(&yaml.Node{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: it has more than one document", ErrInvalid)
	}

	layout, err := readLayout(data)
	if err != nil {
		return nil, err
	}
	if len(layout.lines) != len(doc.Exceptions) {
		return nil, fmt.Errorf("%w: cannot match %d entries to their %d lines", ErrInvalid, len(doc.Exceptions), len(layout.lines))
	}

	rec := &Record{Version: doc.Version, Exceptions: make([]Exception, 0, len(doc.Exceptions))}
	var faults []string
	if layout.versionTag != "" && layout.versionTag != "!!int" && layout.versionTag != "!!str" {
		faults = append(faults, fmt.Sprintf("version must be a whole number or text, not %s", layout.versionTag))
	}
	for i, entry := range doc.Exceptions {
		if entry == nil {
			faults = append(faults, fmt.Sprintf("exception %d (line %d) is empty", i+1, layout.lines[i]))
			rec.Exceptions = append(rec.Exceptions, Exception{Line: layout.lines[i], null: true})

			continue
		}
		entry.Line = layout.lines[i]
		rec.Exceptions = append(rec.Exceptions, *entry)
	}

	faults = append(faults, rec.faults(known)...)
	if len(faults) > 0 {
		return nil, fmt.Errorf("%w:\n  - %s", ErrInvalid, strings.Join(faults, "\n  - "))
	}

	return rec, nil
}

// layout is what the decoder does not keep: where each entry starts and what kind of scalar
// the version was written as.
type layout struct {
	lines      []int
	versionTag string
}

func readLayout(data []byte) (layout, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return layout{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return layout{}, nil
	}

	var found layout
	top := root.Content[0].Content
	for i := 0; i+1 < len(top); i += 2 {
		switch top[i].Value {
		case "version":
			node := top[i+1]
			if node.Kind == yaml.AliasNode && node.Alias != nil {
				node = node.Alias
			}
			if node.Kind == yaml.ScalarNode {
				found.versionTag = node.ShortTag()
			}
		case "exceptions":
			if top[i+1].Kind != yaml.SequenceNode {
				continue
			}
			for _, item := range top[i+1].Content {
				found.lines = append(found.lines, item.Line)
			}
		}
	}

	return found, nil
}

func (r *Record) faults(known func(id string) bool) []string {
	var faults []string

	if strings.TrimSpace(r.Version) == "" {
		faults = append(faults, "version is missing: state the actions-qa-ci version the project pins")
	}

	for i, e := range r.Exceptions {
		if e.null {
			continue // reported once, by Parse
		}
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
