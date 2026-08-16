package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

// ConfigOp is one kind of edit applied to the configuration.
type ConfigOp string

const (
	ConfigOpSet    ConfigOp = "set"    // replace the property's value outright
	ConfigOpAdd    ConfigOp = "add"    // append values, skipping duplicates
	ConfigOpRemove ConfigOp = "remove" // drop the named values
	ConfigOpUnset  ConfigOp = "unset"  // delete the property, restoring its default
)

// ConfigEdit describes a single edit. Values is empty for unset, and may be
// empty for set — "set with no values" is how an explicitly empty list is
// expressed, which for project_markers is meaningfully different from unset.
type ConfigEdit struct {
	Operation ConfigOp
	Property  string
	Values    []string
}

// PropertySpec is the machine- and human-readable description of one editable
// property. The name and type come from the Config struct itself; only the
// prose and the validation rules are written by hand.
type PropertySpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Note        string `json:"note,omitempty"`
	// Allowed enumerates the accepted values, for properties that take a fixed
	// set. Empty means any value passing validation.
	Allowed []string `json:"allowed,omitempty"`
}

// propertyMeta carries what reflection cannot infer. Every field of Config
// must have an entry and every entry must correspond to a field; a test
// enforces both directions so the two cannot drift apart.
var propertyMeta = map[string]struct {
	description string
	note        string
	validate    func(string) error
	// defaults supplies the effective value when the property is unset, so
	// that add and remove extend the defaults rather than silently discarding
	// them. nil means an unset property is simply empty. A scalar property
	// returns its single default value here, so that everything reading
	// effective values can treat both kinds alike.
	defaults func() []string
	// allowed enumerates the accepted values, for properties taking a fixed
	// set. Reported by the schema so the choice is discoverable.
	allowed []string
}{
	"project_globs": {
		description: "Glob patterns naming directories that may hold projects. A leading ~/ expands to the home directory.",
		validate:    validateGlobValue,
	},
	"project_markers": {
		description: "Files whose presence marks a directory as a project. Literal names or glob patterns.",
		note:        "Unset restores the built-in defaults; an empty list turns filtering off entirely.",
		validate:    validateMarkerValue,
		defaults:    func() []string { return defaultProjectMarkers },
	},
	"prune_directories": {
		description: "Directory names that ** recursion does not descend into, such as node_modules.",
		note:        "Applies only to ** patterns. Unset restores the built-in defaults; an empty list disables pruning.",
		validate:    validateDirectoryName,
		defaults:    func() []string { return defaultPruneDirectories },
	},
	"spawn_mode": {
		description: "How claude places the sessions it creates inside an environment.",
		note: "same-dir shares the project directory; worktree gives each session its own git worktree, " +
			"which needs a git repository and excludes anything not committed — vendor directories, .env files, " +
			"local database state.",
		validate: validateSpawnMode,
		defaults: func() []string { return []string{string(defaultSpawnMode)} },
		allowed:  []string{string(SpawnSameDir), string(SpawnWorktree)},
	},
}

// configSchema lists the editable properties, derived from the Config struct.
func configSchema() []PropertySpec {
	t := reflect.TypeOf(Config{})

	specs := make([]PropertySpec, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		name := jsonFieldName(field)
		if name == "" {
			continue
		}
		meta := propertyMeta[name]
		specs = append(specs, PropertySpec{
			Name:        name,
			Type:        describeType(field.Type),
			Description: meta.description,
			Note:        meta.note,
			Allowed:     meta.allowed,
		})
	}
	return specs
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	return strings.Split(tag, ",")[0]
}

func describeType(t reflect.Type) string {
	switch {
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.String:
		return "string list"
	case t.Kind() == reflect.String:
		return "string"
	default:
		return t.Kind().String()
	}
}

// applyConfigEdit mutates cfg in place. It validates everything before
// changing anything, so a rejected edit leaves the configuration untouched
// rather than half-applied.
//
// Notes describe adjustments the caller did not ask for but should know about,
// such as an unset property being seeded from its defaults.
func applyConfigEdit(cfg *Config, edit ConfigEdit) (notes []string, err error) {
	field, err := configField(cfg, edit.Property)
	if err != nil {
		return nil, err
	}
	meta := propertyMeta[edit.Property]

	if edit.Operation != ConfigOpUnset && meta.validate != nil {
		for _, v := range edit.Values {
			if err := meta.validate(v); err != nil {
				return nil, fmt.Errorf("invalid value %q for %s: %w", v, edit.Property, err)
			}
		}
	}

	switch field.Kind() {
	case reflect.String:
		return nil, applyScalarEdit(field, edit)
	case reflect.Slice:
		if field.Type().Elem().Kind() != reflect.String {
			return nil, fmt.Errorf("property %q has an unsupported type", edit.Property)
		}
	default:
		return nil, fmt.Errorf("property %q has an unsupported type", edit.Property)
	}

	current := field.Interface().([]string)

	switch edit.Operation {
	case ConfigOpUnset:
		if len(edit.Values) > 0 {
			return nil, fmt.Errorf("unset takes no values")
		}
		field.Set(reflect.Zero(field.Type()))
		return nil, nil

	case ConfigOpSet:
		// Always a non-nil slice, including when empty: for a property with
		// defaults, "explicitly empty" and "unset" mean different things and
		// the difference has to survive being written out.
		next := make([]string, 0, len(edit.Values))
		next = append(next, dedupe(edit.Values)...)
		field.Set(reflect.ValueOf(next))
		return nil, nil

	case ConfigOpAdd:
		if len(edit.Values) == 0 {
			return nil, fmt.Errorf("add requires at least one value")
		}
		next, notes := effectiveValues(current, edit.Property, meta.defaults)
		for _, v := range edit.Values {
			if contains(next, v) {
				notes = append(notes, fmt.Sprintf("%q is already present, skipped", v))
				continue
			}
			next = append(next, v)
		}
		field.Set(reflect.ValueOf(next))
		return notes, nil

	case ConfigOpRemove:
		if len(edit.Values) == 0 {
			return nil, fmt.Errorf("remove requires at least one value")
		}
		next, notes := effectiveValues(current, edit.Property, meta.defaults)
		// Reject unknown values before removing any, so a typo cannot half
		// apply. A value that is not there is far more likely a mistake than
		// an intent.
		for _, v := range edit.Values {
			if !contains(next, v) {
				return nil, fmt.Errorf("%s does not contain %q", edit.Property, v)
			}
		}
		kept := make([]string, 0, len(next))
		for _, v := range next {
			if !contains(edit.Values, v) {
				kept = append(kept, v)
			}
		}
		field.Set(reflect.ValueOf(kept))
		return notes, nil

	default:
		return nil, fmt.Errorf("unknown operation %q", edit.Operation)
	}
}

// applyScalarEdit handles a property holding a single value.
//
// add and remove are rejected rather than quietly reinterpreted as set: a
// scalar has nothing to append to, and pretending otherwise would make
// "config add spawn_mode worktree" look like it did something different from
// what it did.
func applyScalarEdit(field reflect.Value, edit ConfigEdit) error {
	switch edit.Operation {
	case ConfigOpUnset:
		if len(edit.Values) > 0 {
			return fmt.Errorf("unset takes no values")
		}
		field.SetString("")
		return nil

	case ConfigOpSet:
		if len(edit.Values) != 1 {
			return fmt.Errorf("%s takes exactly one value; use unset to restore the default", edit.Property)
		}
		field.SetString(edit.Values[0])
		return nil

	case ConfigOpAdd, ConfigOpRemove:
		return fmt.Errorf("%s holds a single value, so %s does not apply — use set or unset",
			edit.Property, edit.Operation)

	default:
		return fmt.Errorf("unknown operation %q", edit.Operation)
	}
}

// effectiveValues returns the list an add or remove should start from. For an
// unset property that has defaults, that is the defaults: "add a marker" means
// extend what is in force, not replace it with a single entry.
func effectiveValues(current []string, property string, defaults func() []string) ([]string, []string) {
	if current != nil || defaults == nil {
		return append([]string(nil), current...), nil
	}
	seeded := append([]string(nil), defaults()...)
	note := fmt.Sprintf(
		"%s was unset, so the %d built-in defaults were written out first; it no longer tracks future default changes",
		property, len(seeded))
	return seeded, []string{note}
}

func configField(cfg *Config, property string) (reflect.Value, error) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := range t.NumField() {
		if jsonFieldName(t.Field(i)) == property {
			return v.Field(i), nil
		}
	}
	return reflect.Value{}, fmt.Errorf("unknown property %q (valid: %s)",
		property, strings.Join(configPropertyNames(), ", "))
}

func configPropertyNames() []string {
	specs := configSchema()
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	return names
}

// --- per-property value validation ---

func validateGlobValue(v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("must not be blank")
	}
	if _, err := filepath.Match(v, ""); err != nil {
		return err
	}
	return nil
}

func validateMarkerValue(v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("must not be blank")
	}
	// Markers are matched against directory entry names, so a value carrying a
	// separator could never match anything. Rejecting it here beats letting it
	// sit in the config doing nothing.
	if strings.ContainsAny(v, `/\`) {
		return fmt.Errorf("must be a single file name, not a path")
	}
	if isGlobPattern(v) {
		if _, err := filepath.Match(v, ""); err != nil {
			return err
		}
	}
	return nil
}

func validateSpawnMode(v string) error {
	switch SpawnMode(v) {
	case SpawnSameDir, SpawnWorktree:
		return nil
	default:
		return fmt.Errorf("must be %s or %s", SpawnSameDir, SpawnWorktree)
	}
}

// validateDirectoryName accepts a bare directory name. Prune entries are
// compared against a single path component, so a pattern or a path would never
// match anything.
func validateDirectoryName(v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("must not be blank")
	}
	if strings.ContainsAny(v, `/\`) {
		return fmt.Errorf("must be a single directory name, not a path")
	}
	if isGlobPattern(v) {
		return fmt.Errorf("must be a literal directory name, not a pattern")
	}
	return nil
}

// --- small helpers ---

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func dedupe(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
