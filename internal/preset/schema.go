package preset

import (
	"os"
	"path/filepath"
)

// SchemaFileName is the JSON schema written alongside preset YAML files so
// editors (via the YAML language server modeline) can validate hand edits.
const SchemaFileName = "preset.schema.json"

// schemaModeline is prepended to every written preset so a YAML-aware editor
// resolves the sibling schema file automatically.
const schemaModeline = "# yaml-language-server: $schema=" + SchemaFileName

// presetSchema is a JSON Schema (draft 2020-12) describing the preset file
// format documented in the README.
const presetSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/dhcgn/jxleet/preset.schema.json",
  "title": "jxleet preset",
  "description": "A jxleet preset pairs file-format filters with verbatim cjxl arguments.",
  "type": "object",
  "required": ["name", "rules"],
  "additionalProperties": false,
  "properties": {
    "name": {
      "type": "string",
      "minLength": 1,
      "description": "Unique preset name."
    },
    "description": {
      "type": "string",
      "description": "Human-readable summary of what the preset does."
    },
    "version": {
      "type": "integer",
      "minimum": 1,
      "description": "Preset schema version for migrations."
    },
    "read_only": {
      "type": "boolean",
      "description": "Built-in presets are read-only and cannot be edited in the app."
    },
    "output": {
      "type": "object",
      "additionalProperties": false,
      "description": "Where converted files are written and how collisions are handled.",
      "properties": {
        "policy": {
          "type": "string",
          "enum": ["alongside", "subfolder", "replace"],
          "description": "alongside = next to the original; subfolder = into ./<subfolder>/; replace = original goes to the recycle bin after verification."
        },
        "subfolder": {
          "type": "string",
          "description": "Target subfolder name when policy is 'subfolder'."
        },
        "on_collision": {
          "type": "string",
          "enum": ["skip", "number", "overwrite"],
          "description": "What to do when the output path already exists."
        }
      }
    },
    "rules": {
      "type": "array",
      "minItems": 1,
      "description": "First-matching rule wins. Add a trailing '*' rule as the catch-all fallback.",
      "items": {
        "type": "object",
        "required": ["match"],
        "additionalProperties": false,
        "properties": {
          "match": {
            "type": "array",
            "minItems": 1,
            "description": "Format names (JPEG, PNG, APNG, GIF, EXR, PPM, PGM, PAM, PFM, PGX, JXL) or '*' for the catch-all.",
            "items": { "type": "string" }
          },
          "args": {
            "type": "object",
            "description": "cjxl flags passed verbatim. Keys are flag names (e.g. '-d', '--effort'); a boolean true means a valueless flag.",
            "additionalProperties": {
              "type": ["string", "number", "integer", "boolean"]
            }
          }
        }
      }
    }
  }
}
`

// EnsureSchema writes the preset JSON schema into the store directory when it is
// missing or out of date. It is safe to call repeatedly.
func (s *Store) EnsureSchema() error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.Dir, SchemaFileName)
	if existing, err := os.ReadFile(path); err == nil && string(existing) == presetSchema {
		return nil
	}
	return os.WriteFile(path, []byte(presetSchema), 0o644)
}
