package flags

import (
	"bufio"
	"io"
	"strings"
)

// Parse reads the output of `cjxl --help -v -v -v -v` and extracts the flag set.
//
// The help format (libjxl 0.12) uses:
//   - section headers at column 0 ending in ":" (e.g. "Basic options:"),
//   - flag lines beginning with a single space then a dash
//     (e.g. " -d DISTANCE, --distance=DISTANCE" or " --quiet"),
//   - description lines indented with four spaces.
//
// Lines that fit none of these (the "Usage:" banner, the INPUT/OUTPUT block, or
// a description line that lost its indentation) are ignored for flag purposes.
func Parse(r io.Reader) ([]Flag, error) {
	var (
		out     []Flag
		section string
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Column-0 line: a section header (ends in ":") or noise.
		if !strings.HasPrefix(line, " ") {
			if h, ok := sectionHeader(line); ok {
				section = h
			}
			continue
		}

		// Four-space indent: description continuation of the current flag.
		if strings.HasPrefix(line, "    ") {
			if len(out) > 0 && out[len(out)-1].Description == "" {
				out[len(out)-1].Description = strings.TrimSpace(line)
			}
			continue
		}

		// A flag line starts with exactly one space and a dash.
		if strings.HasPrefix(line, " -") {
			if f, ok := parseFlagLine(strings.TrimSpace(line)); ok {
				f.Section = section
				out = append(out, f)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// sectionHeader recognises "Some heading:" lines. It rejects the "Usage:" line,
// whose colon is not at the end.
func sectionHeader(line string) (string, bool) {
	if strings.HasSuffix(line, ":") && !strings.Contains(line, " -") {
		return strings.TrimSuffix(line, ":"), true
	}
	return "", false
}

// parseFlagLine parses one trimmed flag line such as
// "-d DISTANCE, --distance=DISTANCE" or "-m 0|1, --modular=0|1" or "--quiet".
func parseFlagLine(line string) (Flag, bool) {
	var f Flag
	got := false
	for _, spelling := range strings.Split(line, ", ") {
		spelling = strings.TrimSpace(spelling)
		if spelling == "" {
			continue
		}
		switch {
		case strings.HasPrefix(spelling, "--"):
			name, spec := splitLong(spelling[2:])
			if name == "" {
				continue
			}
			f.Long = name
			if spec != "" {
				f.TakesValue = true
				if f.ValueSpec == "" {
					f.ValueSpec = spec
				}
			}
			got = true
		case strings.HasPrefix(spelling, "-") && len(spelling) >= 2:
			f.Short = string(spelling[1])
			if spec := strings.TrimSpace(spelling[2:]); spec != "" {
				f.TakesValue = true
				if f.ValueSpec == "" {
					f.ValueSpec = spec
				}
			}
			got = true
		}
	}
	return f, got && (f.Short != "" || f.Long != "")
}

// splitLong separates a long spelling like "distance=DISTANCE" into the name
// ("distance") and value spec ("DISTANCE"). "quiet" yields ("quiet", "").
func splitLong(s string) (name, spec string) {
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}
