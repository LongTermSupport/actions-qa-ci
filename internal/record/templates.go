package record

import "fmt"

// templateFaults judges the templates section as a whole: each entry has a clean source and at
// least one clean copy, no path is a copy twice, and no file is at once a source and a copy
// (a chain would make "identical to its source" mean two things).
func templateFaults(templates []Template) []string {
	var faults []string

	sources := map[string]bool{}
	for _, t := range templates {
		if !t.null && t.Source != "" {
			sources[t.Source] = true
		}
	}

	declared := map[string]bool{}
	for i, t := range templates {
		if t.null {
			continue // reported once, by Parse
		}
		where := fmt.Sprintf("template %d (line %d)", i+1, t.Line)

		if t.Source == "" {
			faults = append(faults, where+": source is missing")
		} else if problem := pathProblem(t.Source); problem != "" {
			faults = append(faults, fmt.Sprintf("%s: source %q %s", where, t.Source, problem))
		}
		if len(t.Copies) == 0 {
			faults = append(faults, where+": copies is missing: name at least one copy")
		}

		for _, copied := range t.Copies {
			twice := declared[copied]
			declared[copied] = true

			switch {
			case copied == "":
				faults = append(faults, where+": copies has an empty entry")
			case pathProblem(copied) != "":
				faults = append(faults, fmt.Sprintf("%s: copies entry %q %s", where, copied, pathProblem(copied)))
			case copied == t.Source:
				faults = append(faults, fmt.Sprintf("%s: copy %q is the source itself", where, copied))
			case sources[copied]:
				faults = append(faults, fmt.Sprintf("%s: %q is both a source and a copy", where, copied))
			case twice:
				faults = append(faults, fmt.Sprintf("%s: copy %q is declared twice", where, copied))
			}
		}
	}

	return faults
}
