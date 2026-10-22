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
	"regexp"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v4"

	"github.com/LongTermSupport/actions-qa-ci/internal/finding"
)

// ErrInvalid marks a record that was read and is not a valid one, as opposed to a file that
// could not be read: the first is a mistake in the record, the second a fault of the machine.
var ErrInvalid = errors.New("the project record is not valid")

// InvalidError is a record that was read and is not a valid one, with each reason it was
// refused for. It matches ErrInvalid. The reasons can hold text from the record itself, so a
// caller that prints them escapes each one.
type InvalidError struct {
	// Path is the record file, once Load has read it; Parse knows no file.
	Path    string
	Reasons []string
}

func (e *InvalidError) Error() string {
	text := ErrInvalid.Error() + ":\n  - " + strings.Join(e.Reasons, "\n  - ")
	if e.Path != "" {
		text = e.Path + ": " + text
	}

	return text
}

// Is makes an InvalidError match ErrInvalid.
func (e *InvalidError) Is(target error) bool { return target == ErrInvalid }

func invalid(reasons ...string) *InvalidError { return &InvalidError{Reasons: reasons} }

// Unexceptable says whether an identifier belongs to a check on the record itself. Such a
// check cannot be excepted: an exception that silenced the judge of exceptions would be one
// nobody could judge.
func Unexceptable(id string) bool {
	return id == finding.ToolPrefix+".unjustifiedException" || id == finding.ToolPrefix+".staleException"
}

// Path is where a project keeps its record, relative to the repository root.
const Path = ".github/actions-qa-ci.yml"

// document is a record as the YAML decoder reads it, before its entries are checked.
type document struct {
	Version    string       `yaml:"version"`
	Templates  []*Template  `yaml:"templates"`
	Exceptions []*Exception `yaml:"exceptions"`
}

// maxSize bounds a record: a file a person wrote is nowhere near it.
const maxSize = 1 << 20

// Record is a project's record. A field this build does not act on is not accepted, so a
// record never states something that is silently ignored.
type Record struct {
	// Version is the actions-qa-ci version the project pins.
	Version string `yaml:"version"`
	// Templates declare the copies that have to stay byte-identical to a source.
	Templates  []Template  `yaml:"templates"`
	Exceptions []Exception `yaml:"exceptions"`
}

// Template is a source file and the copies of it the project keeps, each of which has to be
// byte-identical to it.
type Template struct {
	Source string   `yaml:"source"`
	Copies []string `yaml:"copies"`

	// Line is where the entry starts in the record file.
	Line int `yaml:"-"`

	// null marks an entry written as nothing at all, which Parse reports once.
	null bool
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
	var refused *InvalidError
	if errors.As(err, &refused) {
		refused.Path = path

		return nil, refused
	}
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
		return nil, invalid(fmt.Sprintf("it is larger than %d bytes", maxSize))
	}

	// The structure is judged before anything is decoded, so what the decoder fills in is
	// exactly what this reader has seen: no alias, anchor, tag or merge key can carry a value
	// past it.
	layout, err := readLayout(data)
	if err != nil {
		return nil, err
	}

	doc := document{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, invalid(err.Error())
	}
	if err := decoder.Decode(&yaml.Node{}); !errors.Is(err, io.EOF) {
		return nil, invalid("it has more than one document")
	}

	if len(layout.lines) != len(doc.Exceptions) || len(layout.templateLines) != len(doc.Templates) {
		return nil, invalid(fmt.Sprintf("cannot match %d exceptions and %d templates to their %d and %d lines",
			len(doc.Exceptions), len(doc.Templates), len(layout.lines), len(layout.templateLines)))
	}

	rec := &Record{Version: doc.Version, Exceptions: make([]Exception, 0, len(doc.Exceptions)), Templates: make([]Template, 0, len(doc.Templates))}
	var faults []string
	if layout.versionTag != "" && layout.versionTag != "!!int" && layout.versionTag != "!!str" {
		faults = append(faults, fmt.Sprintf("version must be a whole number or text, not %s", layout.versionTag))
	}
	if layout.versionTag == "!!int" && !wholeNumber.MatchString(doc.Version) {
		faults = append(faults, fmt.Sprintf("version %q must be written as plain digits", doc.Version))
	}
	for i, entry := range doc.Templates {
		if entry == nil {
			faults = append(faults, fmt.Sprintf("template %d (line %d) is empty", i+1, layout.templateLines[i]))
			rec.Templates = append(rec.Templates, Template{Line: layout.templateLines[i], null: true})

			continue
		}
		entry.Line = layout.templateLines[i]
		rec.Templates = append(rec.Templates, *entry)
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
		return nil, invalid(faults...)
	}

	return rec, nil
}

// layout is what the decoder does not keep: where each entry starts and what kind of scalar
// the version was written as.
type layout struct {
	lines         []int
	templateLines []int
	versionTag    string
}

func readLayout(data []byte) (layout, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return layout{}, invalid(err.Error())
	}
	if err := rejectIndirection(&root); err != nil {
		return layout{}, invalid(err.Error())
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return layout{}, nil
	}

	var found layout
	top := root.Content[0].Content
	for i := 0; i+1 < len(top); i += 2 {
		switch top[i].Value {
		case "version":
			if top[i+1].Kind == yaml.ScalarNode {
				found.versionTag = top[i+1].ShortTag()
			}
		case "exceptions":
			if top[i+1].Kind != yaml.SequenceNode {
				continue
			}
			for _, item := range top[i+1].Content {
				found.lines = append(found.lines, item.Line)
			}
		case "templates":
			if top[i+1].Kind != yaml.SequenceNode {
				continue
			}
			for _, item := range top[i+1].Content {
				found.templateLines = append(found.templateLines, item.Line)
			}
		}
	}

	return found, nil
}

var wholeNumber = regexp.MustCompile(`^[0-9]+$`)

// rejectIndirection refuses every part of YAML that lets a value reach the record from
// somewhere other than where it is written: aliases, anchors, merge keys, explicit tags, and
// keys that are not plain text. A hand-written record has no use for them, and each is a way
// for the decoder and this reader to disagree about what the record says.
func rejectIndirection(node *yaml.Node) error {
	switch {
	case node.Kind == yaml.AliasNode:
		return fmt.Errorf("line %d: an alias (*%s) is not supported; write the value out", node.Line, node.Value)
	case node.Anchor != "":
		return fmt.Errorf("line %d: an anchor (&%s) is not supported; write the value out", node.Line, node.Anchor)
	case node.Style&yaml.TaggedStyle != 0:
		return fmt.Errorf("line %d: an explicit tag (%s) is not supported", node.Line, node.Tag)
	}

	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind == yaml.AliasNode {
				return fmt.Errorf("line %d: an alias (*%s) is not supported as a key; write the key out", key.Line, key.Value)
			}
			if key.Kind == yaml.ScalarNode && key.ShortTag() == "!!merge" {
				return fmt.Errorf("line %d: a merge key (<<) is not supported; write the values out", key.Line)
			}
			if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
				return fmt.Errorf("line %d: a key must be plain text, not %s", key.Line, describe(key))
			}
		}
	}

	for _, child := range node.Content {
		if err := rejectIndirection(child); err != nil {
			return err
		}
	}

	return nil
}

func describe(node *yaml.Node) string {
	if node.Kind == yaml.ScalarNode {
		return node.ShortTag() + " " + node.Value
	}

	return "a collection"
}

func (r *Record) faults(known func(id string) bool) []string {
	var faults []string

	if strings.TrimSpace(r.Version) == "" {
		faults = append(faults, "version is missing: state the actions-qa-ci version the project pins")
	}

	faults = append(faults, templateFaults(r.Templates)...)

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
		} else if Unexceptable(e.ID) {
			faults = append(faults, fmt.Sprintf("id %q is a check on the record itself and cannot be excepted; fix the record", e.ID))
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
