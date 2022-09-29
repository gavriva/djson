package djson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type DynamicJSON map[string]interface{}

func (self DynamicJSON) Nested(path string, create bool) DynamicJSON {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	v, err := self.doOp(path, create, false, nil)

	if err != nil {
		return nil
	}

	if v, ok := v.(map[string]interface{}); ok {
		return v
	}

	if v, ok := v.(DynamicJSON); ok {
		return v
	}

	return nil
}

func (self DynamicJSON) doOp(path string, autoCreate bool, setValue bool, value interface{}) (interface{}, error) {
	orgPath := path
	path = strings.TrimLeft(path, "/")
	level := self
	for {
		if len(path) == 0 {
			return level, nil
		}
		i := strings.IndexByte(path, byte('/'))
		var name string
		if i >= 0 {
			// must be container
			name = path[:i]
			path = path[i+1:]
			next, ok := level[name]
			if ok {
				if nextMap, ok := next.(DynamicJSON); ok {
					level = nextMap
					continue
				}
			}

			if autoCreate {
				nextMap := make(DynamicJSON)
				level[name] = nextMap
				level = nextMap
			} else {
				return nil, fmt.Errorf("not found %s", orgPath)
			}

		} else {
			name = path

			if setValue {
				level[name] = value
			}
			v, ok := level[name]
			if ok {
				return v, nil
			}
			return nil, fmt.Errorf("not found %s", orgPath)
		}
	}
}

// allows index in path
func (self DynamicJSON) Get(path string) (interface{}, bool) {
	v, err := self.doOp(path, false, false, nil)
	if err == nil {
		return v, true
	}
	return nil, false
}

// allows index in path
func (self DynamicJSON) Set(path string, value interface{}) {
	_, _ = self.doOp(path, true, true, deserialize(value))
}

func (self DynamicJSON) GetInt(path string, defaultValue int) int {
	v, ok := self.Get(path)
	if !ok {
		return defaultValue
	}
	if n, ok := v.(json.Number); ok {
		n = json.Number(strings.TrimSuffix(n.String(), ".0"))
		i, err := n.Int64()
		if err == nil {
			return int(i)
		}
	}
	if f, ok := v.(float64); ok {
		return int(math.Round(f))
	}
	if i, ok := v.(int); ok {
		return i
	}
	if i, ok := v.(int64); ok {
		return int(i)
	}
	if s, ok := v.(string); ok {
		i, err := strconv.Atoi(s)
		if err == nil {
			return i
		}
	}
	return defaultValue
}

func (self DynamicJSON) GetFloat(path string, defaultValue float64) float64 {
	v, ok := self.Get(path)
	if !ok {
		return defaultValue
	}
	if n, ok := v.(json.Number); ok {
		f, err := n.Float64()
		if err == nil {
			return f
		}
	}
	if f, ok := v.(float64); ok {
		return f
	}
	if s, ok := v.(string); ok {
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return f
		}
	}
	return defaultValue
}

func (self DynamicJSON) GetBool(path string, defaultValue bool) bool {
	v, ok := self.Get(path)
	if !ok {
		return defaultValue
	}
	if n, ok := v.(bool); ok {
		return n
	}
	return defaultValue
}

func (self DynamicJSON) GetString(path string, defaultValue string) string {
	v, ok := self.Get(path)
	if !ok {
		return defaultValue
	}
	if s, ok := v.(string); ok {
		return s
	}
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	return defaultValue
}

func (self DynamicJSON) IsArray() bool {
	for nm := range self {
		i := toIndex(nm)
		if i < 0 {
			return false
		}
	}
	return len(self) > 0
}

// O.E: get slice of primitive types
func (self DynamicJSON) GetSlice(path string) []DynamicJSON {
	v, ok := self.Get(path)
	if !ok {
		return nil
	}
	if g, ok := v.(DynamicJSON); ok {

		result := make([]DynamicJSON, len(g))

		indices := make([]string, 0, len(g))
		for nm := range g {
			indices = append(indices, nm)
		}

		sort.Slice(indices, func(i int, j int) bool {
			num1, err1 := strconv.Atoi(indices[i])
			num2, err2 := strconv.Atoi(indices[j])
			if err1 == nil && err2 == nil {
				return num1 < num2
			}
			return strings.Compare(indices[i], indices[j]) < 0
		})

		for i, nm := range indices {
			result[i], _ = g[nm].(DynamicJSON)
		}

		return result
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var rNumber = regexp.MustCompile(`^[0-9]+$`)

func toIndex(nm string) int {
	if !rNumber.MatchString(nm) {
		return -1
	}

	i, _ := strconv.Atoi(nm)
	return i
}

func serialize(self DynamicJSON) interface{} {
	var indices []int
	for nm := range self {
		i := toIndex(nm)
		if i < 0 {
			break
		}
		indices = append(indices, i)
	}
	if len(indices) != len(self) {
		// leave as map
		r := make(DynamicJSON)
		for nm, v := range self {
			if g, ok := v.(DynamicJSON); ok {
				r[nm] = serialize(g)
			} else {
				r[nm] = v
			}
		}
		return r
	}

	// convert to array
	sort.Ints(indices)
	r := make([]interface{}, 0, len(indices))
	for _, nm := range indices {
		v := self[strconv.Itoa(nm)]
		if g, ok := v.(DynamicJSON); ok {
			r = append(r, serialize(g))
		} else {
			r = append(r, v)
		}
	}

	return r
}

func (self DynamicJSON) JSON() []byte {
	b, err := json.Marshal(serialize(self))
	if err == nil {
		var dst bytes.Buffer
		err = json.Indent(&dst, b, "", "  ")
		if err != nil {
			panic(err)
		}
		return dst.Bytes()
	} else {
		panic(err)
	}
}

func (self DynamicJSON) Clone() DynamicJSON {
	v, err := Parse(self.JSON())
	if err != nil {
		panic(err)
	}
	return v
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func deserialize(v interface{}) interface{} {

	if a, ok := v.([]interface{}); ok {
		r := make(DynamicJSON)
		for i := range a {
			r[strconv.Itoa(i)] = deserialize(a[i])
		}
		return r
	}

	if g, ok := v.(map[string]interface{}); ok {
		r := make(DynamicJSON)
		for nm := range g {
			r[nm] = deserialize(g[nm])
		}
		return r
	}

	return v
}

func Parse(data []byte) (DynamicJSON, error) {
	var raw interface{}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	err := dec.Decode(&raw)
	if err != nil {
		return nil, err
	}

	return deserialize(raw).(DynamicJSON), nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func FromResponse(resp *http.Response, err0 error) (response DynamicJSON, err error) {
	if err0 != nil {
		return nil, err0
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return nil, err
	}

	return Parse(body)
}
