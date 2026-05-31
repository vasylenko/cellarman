package brew

import "strings"

// readyMarker is brew doctor's all-clear message. Matching it is more reliable
// than trusting the exit code, which the UI shouldn't have to thread through.
const readyMarker = "Your system is ready to brew"

// parseDoctor groups `brew doctor` output into warning blocks. Each block
// starts with a "Warning:" line; subsequent non-empty lines are its details,
// until the next "Warning:" or end of output. The leading explanatory preamble
// brew prints before any warning is skipped because it isn't a warning.
func parseDoctor(s string) *DoctorReport {
	report := &DoctorReport{Raw: s}
	if strings.Contains(s, readyMarker) {
		report.OK = true
		return report
	}

	var current *Warning
	flush := func() {
		if current != nil {
			report.Warnings = append(report.Warnings, *current)
			current = nil
		}
	}
	for _, line := range strings.Split(s, "\n") {
		if title, ok := strings.CutPrefix(line, "Warning:"); ok {
			flush()
			current = &Warning{Title: strings.TrimSpace(title)}
			continue
		}
		if current == nil {
			continue // preamble before the first warning
		}
		// Preserve leading indentation: brew indents the affected items (the
		// formulae/paths the warning is about) under flush-left prose, and the UI
		// uses that indentation to highlight them as the actual problem items.
		if strings.TrimSpace(line) != "" {
			current.Details = append(current.Details, strings.TrimRight(line, " \t"))
		}
	}
	flush()

	report.OK = len(report.Warnings) == 0
	return report
}
