package cjxl

import "strings"

// Arg is one cjxl option from a preset rule. The Key is the flag exactly as cjxl
// expects it (short "-d" or long "--lossless_jpeg"). Valueless flags (written as
// `true` in a preset) set Valueless; a preset value of `false` should be dropped
// by the caller rather than turned into an Arg.
type Arg struct {
	Key       string
	Value     string
	Valueless bool
}

// Command assembles the full argv for a single conversion:
//
//	binary [options...] input output
//
// Long flags are emitted as a single "--key=value" token; short flags as two
// tokens ("-k", "value"). Both forms are accepted by cjxl. Options precede the
// input and output paths, matching the command preview in the UI.
func Command(binary string, args []Arg, input, output string) []string {
	argv := make([]string, 0, len(args)*2+3)
	argv = append(argv, binary)
	argv = append(argv, Args(args)...)
	argv = append(argv, input, output)
	return argv
}

// Args renders just the option tokens (no binary or paths), for the command
// preview and for composing larger command lines.
func Args(args []Arg) []string {
	out := make([]string, 0, len(args)*2)
	for _, a := range args {
		if a.Key == "" {
			continue
		}
		if a.Valueless {
			out = append(out, a.Key)
			continue
		}
		if strings.HasPrefix(a.Key, "--") {
			out = append(out, a.Key+"="+a.Value)
		} else {
			out = append(out, a.Key, a.Value)
		}
	}
	return out
}
