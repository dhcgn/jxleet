package preset

import (
	"fmt"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"gopkg.in/yaml.v3"
)

// UnmarshalYAML decodes a rule, preserving the order of its args map (a plain
// map would lose order, breaking the command preview). Args values are taken
// verbatim from the YAML scalar text; a boolean true means a valueless flag and
// a boolean false drops the flag entirely.
func (r *Rule) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Match []string  `yaml:"match"`
		Args  yaml.Node `yaml:"args"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	args, err := argsFromNode(&raw.Args)
	if err != nil {
		return err
	}
	r.Match = raw.Match
	r.Args = args
	return nil
}

// MarshalYAML re-emits a rule with its args in order.
func (r Rule) MarshalYAML() (interface{}, error) {
	return struct {
		Match []string   `yaml:"match"`
		Args  *yaml.Node `yaml:"args"`
	}{
		Match: r.Match,
		Args:  argsToNode(r.Args),
	}, nil
}

// argsFromNode converts a YAML mapping node into an ordered arg slice.
func argsFromNode(n *yaml.Node) ([]cjxl.Arg, error) {
	if n == nil || n.Kind == 0 {
		return nil, nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("preset: rule args must be a mapping, got %s", kindName(n.Kind))
	}
	args := make([]cjxl.Arg, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i]
		val := n.Content[i+1]
		if val.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("preset: value for %q must be a scalar", key.Value)
		}
		if val.Tag == "!!bool" {
			// true => valueless flag; false => omit the flag.
			if val.Value == "true" {
				args = append(args, cjxl.Arg{Key: key.Value, Valueless: true})
			}
			continue
		}
		args = append(args, cjxl.Arg{Key: key.Value, Value: val.Value})
	}
	return args, nil
}

// argsToNode builds a YAML mapping node from an ordered arg slice, preserving
// order and re-emitting valueless flags as `true`.
func argsToNode(args []cjxl.Arg) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, a := range args {
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: a.Key}
		var val *yaml.Node
		if a.Valueless {
			val = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
		} else {
			// Empty tag lets the emitter infer the scalar type (numbers stay
			// unquoted, strings are quoted only when necessary).
			val = &yaml.Node{Kind: yaml.ScalarNode, Value: a.Value}
		}
		m.Content = append(m.Content, key, val)
	}
	return m
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.MappingNode:
		return "mapping"
	case yaml.AliasNode:
		return "alias"
	case yaml.DocumentNode:
		return "document"
	default:
		return "unknown"
	}
}

// Marshal serialises a preset to YAML bytes.
func Marshal(p Preset) ([]byte, error) {
	return yaml.Marshal(p)
}

// Unmarshal parses a preset from YAML bytes without migrating it. Callers that
// load from disk should use the store, which also migrates older versions.
func Unmarshal(data []byte) (Preset, error) {
	var p Preset
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Preset{}, err
	}
	return p, nil
}
