package compiler

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	mrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (interp *Interpreter) registerBuiltins() {

	// ===== Section 1: Type Conversion & Inspection =====

	interp.global.Set("print", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = valueToString(a)
			}
			fmt.Println(strings.Join(parts, " "))
			return &NullValue{}
		},
	})

	interp.global.Set("type", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return "null"
			}
			switch args[0].(type) {
			case int64:
				return "int"
			case float64:
				return "float"
			case string:
				return "string"
			case bool:
				return "bool"
			case []Value:
				return "array"
			case map[string]Value:
				return "object"
			case *FunctionValue, *BuiltinFunction, *EnvBuiltinFunction:
				return "function"
			case *NullValue:
				return "null"
			default:
				return "unknown"
			}
		},
	})

	interp.global.Set("str", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return ""
			}
			return valueToString(args[0])
		},
	})

	interp.global.Set("int", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return int64(0)
			}
			switch v := args[0].(type) {
			case int64:
				return v
			case float64:
				return int64(v)
			case bool:
				if v {
					return int64(1)
				}
				return int64(0)
			case string:
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					return n
				}
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					return int64(f)
				}
				return int64(0)
			case *NullValue:
				return int64(0)
			default:
				return int64(0)
			}
		},
	})

	interp.global.Set("float", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return float64(0)
			}
			switch v := args[0].(type) {
			case float64:
				return v
			case int64:
				return float64(v)
			case bool:
				if v {
					return float64(1)
				}
				return float64(0)
			case string:
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					return f
				}
				return float64(0)
			case *NullValue:
				return float64(0)
			default:
				return float64(0)
			}
		},
	})

	interp.global.Set("bool", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return false
			}
			return isTruthy(args[0])
		},
	})

	// ===== Section 2: String Operations =====

	interp.global.Set("len", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return int64(0)
			}
			switch v := args[0].(type) {
			case string:
				return int64(len([]rune(v)))
			case []Value:
				return int64(len(v))
			case map[string]Value:
				return int64(len(v))
			case int64:
				return int64(len(strconv.FormatInt(v, 10)))
			case float64:
				return int64(len(strconv.FormatFloat(v, 'f', -1, 64)))
			case *NullValue:
				return int64(0)
			case bool:
				return int64(len(valueToString(v)))
			default:
				return int64(0)
			}
		},
	})

	containsFn := func(args ...Value) Value {
		if len(args) < 2 {
			return false
		}
		haystack := args[0]
		needle := args[1]
		switch h := haystack.(type) {
		case string:
			return strings.Contains(h, valueToString(needle))
		case []Value:
			needleStr := valueToString(needle)
			for _, item := range h {
				if valueToString(item) == needleStr {
					return true
				}
			}
			return false
		case map[string]Value:
			key := valueToString(needle)
			_, ok := h[key]
			return ok
		default:
			// Coerce haystack to string
			return strings.Contains(valueToString(haystack), valueToString(needle))
		}
	}

	interp.global.Set("contains", &BuiltinFunction{Fn: containsFn})

	interp.global.Set("index_of", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				return int64(-1)
			}
			haystack := args[0]
			needle := args[1]
			switch h := haystack.(type) {
			case string:
				idx := strings.Index(h, valueToString(needle))
				return int64(idx)
			case []Value:
				needleStr := valueToString(needle)
				for i, item := range h {
					if valueToString(item) == needleStr {
						return int64(i)
					}
				}
				return int64(-1)
			default:
				idx := strings.Index(valueToString(haystack), valueToString(needle))
				return int64(idx)
			}
		},
	})

	interp.global.Set("trim", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return ""
			}
			return strings.TrimSpace(valueToString(args[0]))
		},
	})

	interp.global.Set("split", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				if len(args) == 1 {
					// Split into characters
					s := valueToString(args[0])
					chars := []rune(s)
					result := make([]Value, len(chars))
					for i, c := range chars {
						result[i] = string(c)
					}
					return result
				}
				return []Value{}
			}
			sep := valueToString(args[1])
			switch v := args[0].(type) {
			case []Value:
				// Split each element
				var result []Value
				for _, item := range v {
					parts := strings.Split(valueToString(item), sep)
					for _, p := range parts {
						result = append(result, p)
					}
				}
				return result
			default:
				parts := strings.Split(valueToString(v), sep)
				result := make([]Value, len(parts))
				for i, p := range parts {
					result[i] = p
				}
				return result
			}
		},
	})

	interp.global.Set("join", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return ""
			}
			sep := ""
			if len(args) >= 2 {
				sep = valueToString(args[1])
			}
			switch v := args[0].(type) {
			case []Value:
				parts := make([]string, len(v))
				for i, item := range v {
					parts[i] = valueToString(item)
				}
				return strings.Join(parts, sep)
			default:
				// If given multiple string args, join them all
				if len(args) >= 2 {
					// Check if this looks like join(str1, str2, str3...)
					// vs join(arr, sep)
					if _, ok := args[0].(string); ok {
						if len(args) > 2 {
							parts := make([]string, len(args))
							for i, a := range args {
								parts[i] = valueToString(a)
							}
							return strings.Join(parts, "")
						}
					}
				}
				return valueToString(args[0])
			}
		},
	})

	interp.global.Set("upper", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return ""
			}
			return strings.ToUpper(valueToString(args[0]))
		},
	})

	interp.global.Set("lower", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return ""
			}
			return strings.ToLower(valueToString(args[0]))
		},
	})

	interp.global.Set("replace", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 3 {
				if len(args) > 0 {
					return valueToString(args[0])
				}
				return ""
			}
			s := valueToString(args[0])
			old := valueToString(args[1])
			new_ := valueToString(args[2])
			return strings.ReplaceAll(s, old, new_)
		},
	})

	interp.global.Set("starts_with", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				return false
			}
			return strings.HasPrefix(valueToString(args[0]), valueToString(args[1]))
		},
	})

	interp.global.Set("ends_with", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				return false
			}
			return strings.HasSuffix(valueToString(args[0]), valueToString(args[1]))
		},
	})

	interp.global.Set("slice", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				if len(args) == 1 {
					return args[0]
				}
				return &NullValue{}
			}
			start := toInt64(args[1])

			switch v := args[0].(type) {
			case string:
				runes := []rune(v)
				length := int64(len(runes))
				start = normalizeIndex(start, length)
				end := length
				if len(args) >= 3 {
					end = normalizeIndex(toInt64(args[2]), length)
				}
				if start > end {
					return ""
				}
				return string(runes[start:end])
			case []Value:
				length := int64(len(v))
				start = normalizeIndex(start, length)
				end := length
				if len(args) >= 3 {
					end = normalizeIndex(toInt64(args[2]), length)
				}
				if start > end {
					return []Value{}
				}
				result := make([]Value, end-start)
				copy(result, v[start:end])
				return result
			default:
				// Coerce to string
				s := valueToString(v)
				runes := []rune(s)
				length := int64(len(runes))
				start = normalizeIndex(start, length)
				end := length
				if len(args) >= 3 {
					end = normalizeIndex(toInt64(args[2]), length)
				}
				if start > end {
					return ""
				}
				return string(runes[start:end])
			}
		},
	})

	interp.global.Set("match", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				if len(args) == 1 {
					// Single arg: treat as pattern, return whether it compiles
					_, err := regexp.Compile(valueToString(args[0]))
					return err == nil
				}
				return &NullValue{}
			}
			s := valueToString(args[0])
			pattern := valueToString(args[1])
			re, err := regexp.Compile(pattern)
			if err != nil {
				return &NullValue{}
			}
			matches := re.FindStringSubmatch(s)
			if matches == nil {
				return &NullValue{}
			}
			result := make([]Value, len(matches))
			for i, m := range matches {
				result[i] = m
			}
			return result
		},
	})

	interp.global.Set("repeat", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				if len(args) == 1 {
					return valueToString(args[0])
				}
				return ""
			}
			s := valueToString(args[0])
			n := int(toInt64(args[1]))
			if n < 0 {
				n = 0
			}
			return strings.Repeat(s, n)
		},
	})

	// ===== Section 3: Collection Operations =====

	interp.global.Set("append", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return []Value{}
			}
			switch v := args[0].(type) {
			case []Value:
				result := make([]Value, len(v))
				copy(result, v)
				result = append(result, args[1:]...)
				return result
			case string:
				// String concatenation
				var sb strings.Builder
				sb.WriteString(v)
				for _, a := range args[1:] {
					sb.WriteString(valueToString(a))
				}
				return sb.String()
			default:
				// Wrap in array
				result := []Value{args[0]}
				result = append(result, args[1:]...)
				return result
			}
		},
	})

	interp.global.Set("keys", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return []Value{}
			}
			switch v := args[0].(type) {
			case map[string]Value:
				result := make([]Value, 0, len(v))
				for k := range v {
					result = append(result, k)
				}
				// Sort keys for deterministic output
				sort.Slice(result, func(i, j int) bool {
					return result[i].(string) < result[j].(string)
				})
				return result
			case []Value:
				result := make([]Value, len(v))
				for i := range v {
					result[i] = int64(i)
				}
				return result
			case string:
				runes := []rune(v)
				result := make([]Value, len(runes))
				for i := range runes {
					result[i] = int64(i)
				}
				return result
			default:
				return []Value{}
			}
		},
	})

	interp.global.Set("values", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return []Value{}
			}
			switch v := args[0].(type) {
			case map[string]Value:
				// Get keys sorted for deterministic order
				keys := make([]string, 0, len(v))
				for k := range v {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				result := make([]Value, len(keys))
				for i, k := range keys {
					result[i] = v[k]
				}
				return result
			case []Value:
				result := make([]Value, len(v))
				copy(result, v)
				return result
			case string:
				runes := []rune(v)
				result := make([]Value, len(runes))
				for i, c := range runes {
					result[i] = string(c)
				}
				return result
			default:
				return []Value{}
			}
		},
	})

	interp.global.Set("has", &BuiltinFunction{Fn: containsFn})
	interp.global.Set("includes", &BuiltinFunction{Fn: containsFn})

	interp.global.Set("merge", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return &NullValue{}
			}
			switch args[0].(type) {
			case map[string]Value:
				result := make(map[string]Value)
				for _, arg := range args {
					if obj, ok := arg.(map[string]Value); ok {
						for k, v := range obj {
							result[k] = v
						}
					}
				}
				return result
			case []Value:
				var result []Value
				for _, arg := range args {
					if arr, ok := arg.([]Value); ok {
						result = append(result, arr...)
					} else {
						result = append(result, arg)
					}
				}
				return result
			case string:
				var sb strings.Builder
				for _, arg := range args {
					sb.WriteString(valueToString(arg))
				}
				return sb.String()
			default:
				return args[0]
			}
		},
	})

	interp.global.Set("delete", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				if len(args) == 1 {
					return args[0]
				}
				return &NullValue{}
			}
			switch v := args[0].(type) {
			case map[string]Value:
				key := valueToString(args[1])
				result := make(map[string]Value)
				for k, val := range v {
					if k != key {
						result[k] = val
					}
				}
				return result
			case []Value:
				idx := int(toInt64(args[1]))
				if idx < 0 || idx >= len(v) {
					// Return copy of original
					result := make([]Value, len(v))
					copy(result, v)
					return result
				}
				result := make([]Value, 0, len(v)-1)
				result = append(result, v[:idx]...)
				result = append(result, v[idx+1:]...)
				return result
			default:
				return args[0]
			}
		},
	})

	interp.global.Set("sort", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return []Value{}
			}
			arr, ok := args[0].([]Value)
			if !ok {
				return args[0]
			}
			result := make([]Value, len(arr))
			copy(result, arr)

			// Check for custom comparator
			if len(args) >= 2 {
				comparator := args[1]
				sort.SliceStable(result, func(i, j int) bool {
					cmpResult := interp.callFunction(comparator, []Value{result[i], result[j]})
					switch v := cmpResult.(type) {
					case int64:
						return v < 0
					case float64:
						return v < 0
					case bool:
						return v
					default:
						return false
					}
				})
				return result
			}

			// Auto-detect type and sort
			sort.SliceStable(result, func(i, j int) bool {
				return compareValues(result[i], result[j]) < 0
			})
			return result
		},
	})

	interp.global.Set("reverse", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return &NullValue{}
			}
			switch v := args[0].(type) {
			case []Value:
				result := make([]Value, len(v))
				for i, item := range v {
					result[len(v)-1-i] = item
				}
				return result
			case string:
				runes := []rune(v)
				for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
					runes[i], runes[j] = runes[j], runes[i]
				}
				return string(runes)
			default:
				return args[0]
			}
		},
	})

	interp.global.Set("unique", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return []Value{}
			}
			arr, ok := args[0].([]Value)
			if !ok {
				return []Value{args[0]}
			}
			seen := make(map[string]bool)
			var result []Value
			for _, item := range arr {
				key := valueToString(item)
				if !seen[key] {
					seen[key] = true
					result = append(result, item)
				}
			}
			if result == nil {
				return []Value{}
			}
			return result
		},
	})

	interp.global.Set("flat", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return []Value{}
			}
			arr, ok := args[0].([]Value)
			if !ok {
				return []Value{args[0]}
			}
			var result []Value
			for _, item := range arr {
				if inner, ok := item.([]Value); ok {
					result = append(result, inner...)
				} else {
					result = append(result, item)
				}
			}
			if result == nil {
				return []Value{}
			}
			return result
		},
	})

	interp.global.Set("find", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				return &NullValue{}
			}
			arr, ok := args[0].([]Value)
			if !ok {
				return &NullValue{}
			}
			fn := args[1]
			for i, item := range arr {
				result := interp.callFunction(fn, []Value{item, int64(i)})
				if isTruthy(result) {
					return item
				}
			}
			return &NullValue{}
		},
	})

	interp.global.Set("filter", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				if len(args) == 1 {
					if arr, ok := args[0].([]Value); ok {
						// Filter with no fn: remove falsy values
						var result []Value
						for _, item := range arr {
							if isTruthy(item) {
								result = append(result, item)
							}
						}
						if result == nil {
							return []Value{}
						}
						return result
					}
				}
				return []Value{}
			}
			arr, ok := args[0].([]Value)
			if !ok {
				return []Value{}
			}
			fn := args[1]
			var result []Value
			for i, item := range arr {
				callResult := interp.callFunction(fn, []Value{item, int64(i)})
				if isTruthy(callResult) {
					result = append(result, item)
				}
			}
			if result == nil {
				return []Value{}
			}
			return result
		},
	})

	interp.global.Set("map", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				if len(args) == 1 {
					if arr, ok := args[0].([]Value); ok {
						result := make([]Value, len(arr))
						copy(result, arr)
						return result
					}
				}
				return []Value{}
			}
			arr, ok := args[0].([]Value)
			if !ok {
				// Map over a single value
				return []Value{interp.callFunction(args[1], []Value{args[0], int64(0)})}
			}
			fn := args[1]
			result := make([]Value, len(arr))
			for i, item := range arr {
				result[i] = interp.callFunction(fn, []Value{item, int64(i)})
			}
			return result
		},
	})

	// ===== Section 4: Math =====

	interp.global.Set("min", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return &NullValue{}
			}
			// If single arg is an array, find min in array
			if len(args) == 1 {
				if arr, ok := args[0].([]Value); ok {
					args = arr
				}
			}
			if len(args) == 0 {
				return &NullValue{}
			}
			minVal := args[0]
			for _, arg := range args[1:] {
				if compareValues(arg, minVal) < 0 {
					minVal = arg
				}
			}
			return minVal
		},
	})

	interp.global.Set("max", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return &NullValue{}
			}
			// If single arg is an array, find max in array
			if len(args) == 1 {
				if arr, ok := args[0].([]Value); ok {
					args = arr
				}
			}
			if len(args) == 0 {
				return &NullValue{}
			}
			maxVal := args[0]
			for _, arg := range args[1:] {
				if compareValues(arg, maxVal) > 0 {
					maxVal = arg
				}
			}
			return maxVal
		},
	})

	interp.global.Set("abs", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return int64(0)
			}
			switch v := args[0].(type) {
			case int64:
				if v < 0 {
					return -v
				}
				return v
			case float64:
				return math.Abs(v)
			default:
				f := toFloat64(args[0])
				return math.Abs(f)
			}
		},
	})

	interp.global.Set("floor", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return int64(0)
			}
			switch v := args[0].(type) {
			case int64:
				return v
			case float64:
				return int64(math.Floor(v))
			default:
				f := toFloat64(args[0])
				return int64(math.Floor(f))
			}
		},
	})

	interp.global.Set("ceil", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return int64(0)
			}
			switch v := args[0].(type) {
			case int64:
				return v
			case float64:
				return int64(math.Ceil(v))
			default:
				f := toFloat64(args[0])
				return int64(math.Ceil(f))
			}
		},
	})

	interp.global.Set("round", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) == 0 {
				return int64(0)
			}
			f := toFloat64(args[0])
			if len(args) >= 2 {
				// Round to N decimal places
				places := toInt64(args[1])
				mul := math.Pow(10, float64(places))
				return math.Round(f*mul) / mul
			}
			// No decimal places: return int
			switch args[0].(type) {
			case int64:
				return args[0]
			default:
				return int64(math.Round(f))
			}
		},
	})

	interp.global.Set("random", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			return mrand.Float64()
		},
	})

	interp.global.Set("random_int", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 2 {
				if len(args) == 1 {
					// random_int(max) → [0, max]
					max := toInt64(args[0])
					if max <= 0 {
						return int64(0)
					}
					return mrand.Int63n(max + 1)
				}
				return int64(0)
			}
			minV := toInt64(args[0])
			maxV := toInt64(args[1])
			if minV > maxV {
				minV, maxV = maxV, minV
			}
			if minV == maxV {
				return minV
			}
			return minV + mrand.Int63n(maxV-minV+1)
		},
	})

	interp.global.Set("clamp", &BuiltinFunction{
		Fn: func(args ...Value) Value {
			if len(args) < 3 {
				if len(args) > 0 {
					return args[0]
				}
				return int64(0)
			}
			// Determine if we should work in int or float
			_, valIsInt := args[0].(int64)
			_, minIsInt := args[1].(int64)
			_, maxIsInt := args[2].(int64)
			if valIsInt && minIsInt && maxIsInt {
				v := args[0].(int64)
				lo := args[1].(int64)
				hi := args[2].(int64)
				if v < lo {
					return lo
				}
				if v > hi {
					return hi
				}
				return v
			}
			v := toFloat64(args[0])
			lo := toFloat64(args[1])
			hi := toFloat64(args[2])
			if v < lo {
				return lo
			}
			if v > hi {
				return hi
			}
			return v
		},
	})

	// ===== Section 5: Time =====

	interp.global.Set("now", &BuiltinFunction{Fn: func(args ...Value) Value {
		return int64(time.Now().Unix())
	}})

	interp.global.Set("now_ms", &BuiltinFunction{Fn: func(args ...Value) Value {
		return int64(time.Now().UnixMilli())
	}})

	interp.global.Set("time_format", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) == 0 {
			return time.Now().UTC().Format(time.RFC3339)
		}
		var ts time.Time
		switch v := args[0].(type) {
		case int64:
			ts = time.Unix(v, 0).UTC()
		case float64:
			ts = time.Unix(int64(v), 0).UTC()
		default:
			ts = time.Now().UTC()
		}
		if len(args) >= 2 {
			if fmtStr, ok := args[1].(string); ok {
				// Map common format names to Go layouts
				switch fmtStr {
				case "iso", "ISO", "iso8601":
					return ts.Format(time.RFC3339)
				case "date":
					return ts.Format("2006-01-02")
				case "time":
					return ts.Format("15:04:05")
				case "datetime":
					return ts.Format("2006-01-02 15:04:05")
				case "rfc2822":
					return ts.Format(time.RFC1123Z)
				case "unix":
					return int64(ts.Unix())
				default:
					// Try as Go time layout
					return ts.Format(fmtStr)
				}
			}
		}
		return ts.Format(time.RFC3339)
	}})

	interp.global.Set("sleep", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) >= 1 {
			switch v := args[0].(type) {
			case int64:
				time.Sleep(time.Duration(v) * time.Millisecond)
			case float64:
				time.Sleep(time.Duration(v*1000) * time.Microsecond)
			}
		}
		return &NullValue{}
	}})

	// ===== Section 6: Encoding =====

	interp.global.Set("base64_encode", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return ""
		}
		s := valueToString(args[0])
		return base64.StdEncoding.EncodeToString([]byte(s))
	}})

	interp.global.Set("base64_decode", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return ""
		}
		s := valueToString(args[0])
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			// Try URL-safe base64
			b, err = base64.URLEncoding.DecodeString(s)
			if err != nil {
				// Try without padding
				b, err = base64.RawURLEncoding.DecodeString(s)
				if err != nil {
					return &NullValue{}
				}
			}
		}
		return string(b)
	}})

	interp.global.Set("url_encode", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return ""
		}
		return url.QueryEscape(valueToString(args[0]))
	}})

	interp.global.Set("url_decode", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return ""
		}
		s, err := url.QueryUnescape(valueToString(args[0]))
		if err != nil {
			return valueToString(args[0])
		}
		return s
	}})

	// ===== Section 7: Crypto =====

	interp.global.Set("sha256", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return ""
		}
		h := sha256.Sum256([]byte(valueToString(args[0])))
		return hex.EncodeToString(h[:])
	}})

	interp.global.Set("md5", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return ""
		}
		h := md5.Sum([]byte(valueToString(args[0])))
		return hex.EncodeToString(h[:])
	}})

	interp.global.Set("hmac", &BuiltinFunction{Fn: func(args ...Value) Value {
		// hmac(algorithm, key, data) or hmac(key, data) defaults to sha256
		var algo, key, data string
		if len(args) == 3 {
			algo = valueToString(args[0])
			key = valueToString(args[1])
			data = valueToString(args[2])
		} else if len(args) == 2 {
			algo = "sha256"
			key = valueToString(args[0])
			data = valueToString(args[1])
		} else {
			return ""
		}
		switch strings.ToLower(algo) {
		case "sha256":
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write([]byte(data))
			return hex.EncodeToString(mac.Sum(nil))
		default:
			// Default to sha256
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write([]byte(data))
			return hex.EncodeToString(mac.Sum(nil))
		}
	}})

	interp.global.Set("uuid", &BuiltinFunction{Fn: func(args ...Value) Value {
		b := make([]byte, 16)
		rand.Read(b)
		b[6] = (b[6] & 0x0f) | 0x40 // version 4
		b[8] = (b[8] & 0x3f) | 0x80 // variant 10
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	}})

	interp.global.Set("bcrypt_hash", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return &NullValue{}
		}
		password := valueToString(args[0])
		cost := bcrypt.DefaultCost
		if len(args) >= 2 {
			if c, ok := args[1].(int64); ok {
				cost = int(c)
			}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
		if err != nil {
			return &NullValue{}
		}
		return string(hash)
	}})

	interp.global.Set("bcrypt_verify", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 2 {
			return false
		}
		password := valueToString(args[0])
		hash := valueToString(args[1])
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
		return err == nil
	}})

	// JWT (HS256)
	interp.global.Set("jwt_sign", &BuiltinFunction{Fn: func(args ...Value) Value {
		// jwt_sign(payload_obj, secret_string)
		if len(args) < 2 {
			return &NullValue{}
		}
		payload := valueToGo(args[0])
		secret := valueToString(args[1])

		headerJSON := []byte(`{"alg":"HS256","typ":"JWT"}`)
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return &NullValue{}
		}

		headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
		payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
		signingInput := headerB64 + "." + payloadB64

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(signingInput))
		signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

		return signingInput + "." + signature
	}})

	interp.global.Set("jwt_verify", &BuiltinFunction{Fn: func(args ...Value) Value {
		// jwt_verify(token_string, secret_string) → payload object or null
		if len(args) < 2 {
			return &NullValue{}
		}
		token := valueToString(args[0])
		secret := valueToString(args[1])

		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			return &NullValue{}
		}

		// Verify signature
		signingInput := parts[0] + "." + parts[1]
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(signingInput))
		expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
			return &NullValue{}
		}

		// Decode payload
		payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return &NullValue{}
		}
		var raw interface{}
		if err := json.Unmarshal(payloadJSON, &raw); err != nil {
			return &NullValue{}
		}
		return goToValue(raw)
	}})

	// ===== Section 8: HTTP Client =====

	interp.global.Set("http_get", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return &NullValue{}
		}
		rawURL := valueToString(args[0])

		// Optional headers object as 2nd arg
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return map[string]Value{"error": err.Error(), "status": int64(0)}
		}
		if len(args) >= 2 {
			if hdrs, ok := args[1].(map[string]Value); ok {
				for k, v := range hdrs {
					req.Header.Set(k, valueToString(v))
				}
			}
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return map[string]Value{"error": err.Error(), "status": int64(0)}
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		result := map[string]Value{
			"status":  int64(resp.StatusCode),
			"body":    string(body),
			"headers": httpHeadersToValue(resp.Header),
		}

		// Auto-parse JSON body if content-type is JSON
		ct := resp.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			var raw interface{}
			if err := json.Unmarshal(body, &raw); err == nil {
				result["data"] = goToValue(raw)
			}
		}

		return result
	}})

	interp.global.Set("http_post", &BuiltinFunction{Fn: func(args ...Value) Value {
		// http_post(url, body, headers?)
		if len(args) < 1 {
			return &NullValue{}
		}
		rawURL := valueToString(args[0])

		var bodyStr string
		contentType := "application/json"
		if len(args) >= 2 {
			switch b := args[1].(type) {
			case string:
				bodyStr = b
			case map[string]Value, []Value:
				// Auto-serialize to JSON
				data := valueToGo(b)
				jsonBytes, _ := json.Marshal(data)
				bodyStr = string(jsonBytes)
			default:
				bodyStr = valueToString(b)
			}
		}

		req, err := http.NewRequest("POST", rawURL, strings.NewReader(bodyStr))
		if err != nil {
			return map[string]Value{"error": err.Error(), "status": int64(0)}
		}
		req.Header.Set("Content-Type", contentType)

		if len(args) >= 3 {
			if hdrs, ok := args[2].(map[string]Value); ok {
				for k, v := range hdrs {
					req.Header.Set(k, valueToString(v))
				}
			}
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return map[string]Value{"error": err.Error(), "status": int64(0)}
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		result := map[string]Value{
			"status":  int64(resp.StatusCode),
			"body":    string(respBody),
			"headers": httpHeadersToValue(resp.Header),
		}

		ct := resp.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			var raw interface{}
			if err := json.Unmarshal(respBody, &raw); err == nil {
				result["data"] = goToValue(raw)
			}
		}

		return result
	}})

	// ===== Section 9: HTTP Response (env-aware) =====

	interp.global.Set("header", &EnvBuiltinFunction{Fn: func(env *Environment, args ...Value) Value {
		if len(args) < 2 {
			return &NullValue{}
		}
		name := valueToString(args[0])
		val := valueToString(args[1])

		// Walk up to find _response_headers
		if hdrs, ok := env.Get("_response_headers"); ok {
			if hmap, ok := hdrs.(map[string]Value); ok {
				hmap[name] = val
			}
		}
		return &NullValue{}
	}})

	interp.global.Set("cookie", &EnvBuiltinFunction{Fn: func(env *Environment, args ...Value) Value {
		// cookie(name, value) or cookie(name, value, options_object)
		if len(args) < 2 {
			return &NullValue{}
		}
		name := valueToString(args[0])
		val := valueToString(args[1])

		c := &http.Cookie{Name: name, Value: val, Path: "/"}

		if len(args) >= 3 {
			if opts, ok := args[2].(map[string]Value); ok {
				if p, ok := opts["path"]; ok {
					c.Path = valueToString(p)
				}
				if d, ok := opts["domain"]; ok {
					c.Domain = valueToString(d)
				}
				if ma, ok := opts["maxAge"]; ok {
					c.MaxAge = int(toInt64(ma))
				}
				if ma, ok := opts["max_age"]; ok {
					c.MaxAge = int(toInt64(ma))
				}
				if s, ok := opts["secure"]; ok {
					if b, ok := s.(bool); ok {
						c.Secure = b
					}
				}
				if h, ok := opts["httpOnly"]; ok {
					if b, ok := h.(bool); ok {
						c.HttpOnly = b
					}
				}
				if h, ok := opts["http_only"]; ok {
					if b, ok := h.(bool); ok {
						c.HttpOnly = b
					}
				}
				if ss, ok := opts["sameSite"]; ok {
					switch strings.ToLower(valueToString(ss)) {
					case "lax":
						c.SameSite = http.SameSiteLaxMode
					case "strict":
						c.SameSite = http.SameSiteStrictMode
					case "none":
						c.SameSite = http.SameSiteNoneMode
					}
				}
			}
		}

		// Store cookie string in _response_cookies
		if cookies, ok := env.Get("_response_cookies"); ok {
			if carr, ok := cookies.([]Value); ok {
				carr = append(carr, c.String())
				env.SetExisting("_response_cookies", carr)
			}
		}
		return &NullValue{}
	}})

	interp.global.Set("redirect", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return &NullValue{}
		}
		redirURL := valueToString(args[0])
		code := 302
		if len(args) >= 2 {
			if c, ok := args[1].(int64); ok {
				code = int(c)
			}
		}
		return &RedirectValue{URL: redirURL, StatusCode: code}
	}})

	// ===== Section 10: Logging =====

	interp.global.Set("log", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) == 0 {
			return &NullValue{}
		}
		level := "INFO"
		var parts []string
		if len(args) >= 2 {
			level = strings.ToUpper(valueToString(args[0]))
			for _, a := range args[1:] {
				parts = append(parts, valueToString(a))
			}
		} else {
			parts = append(parts, valueToString(args[0]))
		}
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] %s %s\n", timestamp, level, strings.Join(parts, " "))
		return &NullValue{}
	}})

	interp.global.Set("log_info", &BuiltinFunction{Fn: func(args ...Value) Value {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = valueToString(a)
		}
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] \033[36mINFO\033[0m %s\n", timestamp, strings.Join(parts, " "))
		return &NullValue{}
	}})

	interp.global.Set("log_warn", &BuiltinFunction{Fn: func(args ...Value) Value {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = valueToString(a)
		}
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] \033[33mWARN\033[0m %s\n", timestamp, strings.Join(parts, " "))
		return &NullValue{}
	}})

	interp.global.Set("log_error", &BuiltinFunction{Fn: func(args ...Value) Value {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = valueToString(a)
		}
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] \033[31mERROR\033[0m %s\n", timestamp, strings.Join(parts, " "))
		return &NullValue{}
	}})

	// ===== Section 11: Environment =====

	interp.global.Set("env", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 1 {
			return ""
		}
		name := valueToString(args[0])
		val := os.Getenv(name)
		// Optional default value as 2nd arg
		if val == "" && len(args) >= 2 {
			return args[1]
		}
		return val
	}})

	// ===== Section 12: JSON namespace =====

	jsonObj := map[string]Value{
		"parse": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) < 1 {
				return &NullValue{}
			}
			// Accept string or auto-coerce
			s := valueToString(args[0])
			var raw interface{}
			if err := json.Unmarshal([]byte(s), &raw); err != nil {
				return &NullValue{}
			}
			return goToValue(raw)
		}},
		"stringify": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) < 1 {
				return ""
			}
			data := valueToGo(args[0])
			// Optional indent argument
			if len(args) >= 2 {
				if indent, ok := args[1].(int64); ok {
					b, _ := json.MarshalIndent(data, "", strings.Repeat(" ", int(indent)))
					return string(b)
				}
			}
			b, _ := json.Marshal(data)
			return string(b)
		}},
	}
	interp.global.Set("json", jsonObj)
}

// === Helper functions for builtins ===

// toInt64 converts a Value to int64
func toInt64(v Value) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case bool:
		if val {
			return 1
		}
		return 0
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return int64(f)
		}
		return 0
	default:
		return 0
	}
}

// toFloat64 converts a Value to float64
func toFloat64(v Value) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case bool:
		if val {
			return 1
		}
		return 0
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}


// normalizeIndex handles negative indices (from end) and clamps to bounds
func normalizeIndex(idx, length int64) int64 {
	if idx < 0 {
		idx = length + idx
	}
	if idx < 0 {
		idx = 0
	}
	if idx > length {
		idx = length
	}
	return idx
}

// compareValues compares two values for sorting. Returns -1, 0, or 1.
func compareValues(a, b Value) int {
	// Try numeric comparison first
	aFloat, aIsNum := toNumeric(a)
	bFloat, bIsNum := toNumeric(b)
	if aIsNum && bIsNum {
		if aFloat < bFloat {
			return -1
		}
		if aFloat > bFloat {
			return 1
		}
		return 0
	}
	// Fall back to string comparison
	aStr := valueToString(a)
	bStr := valueToString(b)
	if aStr < bStr {
		return -1
	}
	if aStr > bStr {
		return 1
	}
	return 0
}

// toNumeric attempts to convert a value to float64, returning whether it was numeric
func toNumeric(v Value) (float64, bool) {
	switch val := v.(type) {
	case int64:
		return float64(val), true
	case float64:
		return val, true
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// httpHeadersToValue converts HTTP headers to a map[string]Value
func httpHeadersToValue(headers http.Header) map[string]Value {
	result := make(map[string]Value)
	for k, v := range headers {
		if len(v) == 1 {
			result[k] = v[0]
		} else {
			arr := make([]Value, len(v))
			for i, s := range v {
				arr[i] = s
			}
			result[k] = arr
		}
	}
	return result
}
