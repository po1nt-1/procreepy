package silica

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// keyed is a decoded NSKeyedArchiver graph: a flat $objects table plus the $top
// entry points. Values inside an object are either plain scalars or Refs into
// the table, so traversal is explicit and cycles (Procreate's layers point back
// at their document) cannot loop.
type keyed struct {
	objects []any
	top     map[string]any
}

func parseKeyed(data []byte) (*keyed, error) {
	_, root, err := parsePlist(data)
	if err != nil {
		return nil, err
	}
	m, ok := root.(map[string]any)
	if !ok {
		return nil, errors.New("archive root is not a dictionary")
	}
	if a, _ := m["$archiver"].(string); a != "NSKeyedArchiver" {
		return nil, fmt.Errorf("unsupported archiver %q", a)
	}
	objs, ok := m["$objects"].([]any)
	if !ok {
		return nil, errors.New("archive has no $objects table")
	}
	top, ok := m["$top"].(map[string]any)
	if !ok {
		return nil, errors.New("archive has no $top entry")
	}
	return &keyed{objects: objs, top: top}, nil
}

// deref resolves a Ref to the object it points at. Reference 0 is the archive's
// canonical nil.
func (k *keyed) deref(v any) any {
	r, ok := v.(Ref)
	if !ok {
		return v
	}
	if r == 0 || uint64(r) >= uint64(len(k.objects)) {
		return nil
	}
	return k.objects[r]
}

// object resolves v to a dictionary, or returns nil when it is absent or nil.
func (k *keyed) object(v any) map[string]any {
	m, _ := k.deref(v).(map[string]any)
	return m
}

// className reports the $classname of an object dictionary.
func (k *keyed) className(obj map[string]any) string {
	cls := k.object(obj["$class"])
	if cls == nil {
		return ""
	}
	n, _ := cls["$classname"].(string)
	return n
}

// list flattens an NSArray/NSMutableArray object into its element references.
func (k *keyed) list(v any) []any {
	obj := k.object(v)
	if obj == nil {
		return nil
	}
	items, _ := obj["NS.objects"].([]any)
	return items
}

func (k *keyed) str(v any) string {
	s, _ := k.deref(v).(string)
	return s
}

func (k *keyed) bool(v any) bool {
	switch t := k.deref(v).(type) {
	case bool:
		return t
	case int64:
		return t != 0
	}
	return false
}

func (k *keyed) float(v any) float64 {
	switch t := k.deref(v).(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	}
	return 0
}

func (k *keyed) int(v any) int {
	switch t := k.deref(v).(type) {
	case int64:
		return int(t)
	case float64:
		return int(t)
	}
	return 0
}

func (k *keyed) data(v any) []byte {
	b, _ := k.deref(v).([]byte)
	return b
}

// parseCGSize reads Procreate's stringified CGSize ("{2048, 1536}"). The size is
// archived as text rather than as a struct, so it has to be parsed rather than
// read out of typed fields.
func parseCGSize(s string) (w, h int, err error) {
	t := strings.TrimSpace(s)
	t = strings.TrimPrefix(t, "{")
	t = strings.TrimSuffix(t, "}")
	parts := strings.SplitN(t, ",", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("cannot read a size from %q", s)
	}
	w, err = parseDim(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("cannot read a size from %q: %w", s, err)
	}
	h, err = parseDim(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("cannot read a size from %q: %w", s, err)
	}
	return w, h, nil
}

func parseDim(s string) (int, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	return int(f), nil
}
