package symfony

import "fmt"

// mergeValue merges src onto dst following Symfony's configuration semantics and
// returns the merged result:
//
//   - When both dst and src are maps, keys are merged recursively. Keys present
//     only in dst are kept; keys present in src override or extend dst.
//   - For any other combination (scalar, sequence, or a type mismatch) src wins
//     and replaces dst entirely. In particular sequences are NOT concatenated,
//     matching Symfony where a list defined in a higher-precedence file replaces
//     the lower-precedence list.
//
// dst is treated as immutable: a new map is allocated when merging maps so the
// caller's lower-precedence data is never mutated in place.
func mergeValue(dst, src any) any {
	srcMap, srcIsMap := asStringMap(src)
	if !srcIsMap {
		return src
	}

	dstMap, dstIsMap := asStringMap(dst)
	if !dstIsMap {
		// dst is not a map; src (a map) replaces it. Copy so later merges that
		// reach into this subtree cannot mutate src.
		return cloneMap(srcMap)
	}

	out := make(map[string]any, len(dstMap)+len(srcMap))
	for k, v := range dstMap {
		out[k] = v
	}

	for k, v := range srcMap {
		if existing, ok := out[k]; ok {
			out[k] = mergeValue(existing, v)
		} else {
			out[k] = v
		}
	}

	return out
}

// asStringMap normalises the two map shapes yaml.v3 can produce
// (map[string]any and map[any]any) into a map[string]any. The bool reports
// whether value was a map at all.
func asStringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[stringifyKey(k)] = v
		}

		return out, true
	default:
		return nil, false
	}
}

// cloneMap returns a shallow-then-recursive copy of m so the result shares no
// mutable map nodes with the input.
func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if nested, ok := asStringMap(v); ok {
			out[k] = cloneMap(nested)
		} else {
			out[k] = v
		}
	}

	return out
}

// stringifyKey converts an arbitrary YAML map key into the string form used by
// the merged representation.
func stringifyKey(key any) string {
	if s, ok := key.(string); ok {
		return s
	}

	return fmt.Sprintf("%v", key)
}
