package djson

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	njson "github.com/segmentio/encoding/json"
)

type DynamicJSON struct {
	// map only part
	keys        map[string]uint32 // key -> to index used if it is a map
	ordKeys     []string          // used if it is a map
	iterCounter int               // Frozen if iterCounter < 0

	// map&array part
	values []interface{}
}

func NewMap() *DynamicJSON {
	return &DynamicJSON{
		keys: make(map[string]uint32),
	}
}

func NewArray() *DynamicJSON {
	return &DynamicJSON{}
}

func (self *DynamicJSON) Clear() {

	if self == nil || self.iterCounter < 0 {
		return
	}

	self.values = self.values[:0]

	if self.keys != nil {
		self.keys = make(map[string]uint32)
	}

	if self.ordKeys != nil {
		self.ordKeys = self.ordKeys[:0]
	}
}

func (self *DynamicJSON) Freeze() {

	if self == nil {
		return
	}

	if self.iterCounter < 0 {
		return
	}

	if self.IsArray() {
		for i := 0; i < len(self.values); i++ {
			if d, ok := self.values[i].(*DynamicJSON); ok {
				d.Freeze()
			}
		}
	} else {

		for i := 0; i < len(self.values); i++ {
			if self.values[i] == gDeletedEntry {
				continue
			}

			if d, ok := self.values[i].(*DynamicJSON); ok {
				d.Freeze()
			}
		}
	}

	self.iterCounter = -1000
}

func (self *DynamicJSON) IsArray() bool {
	if self == nil {
		return false
	}
	return self.keys == nil
}

func key2Index(s string) int {
	sLen := len(s)
	if 0 < sLen && sLen < 10 {

		n := 0
		for _, ch := range []byte(s) {
			ch -= '0'
			if ch > 9 {
				return -1
			}
			n = n*10 + int(ch)
		}
		return n
	}
	return -1
}

func (self *DynamicJSON) set(key string, value interface{}) error {

	if self.iterCounter < 0 {
		return fmt.Errorf("Modification attempt of frozen map %s:%v", key, value)
	}

	if self.IsArray() {
		i := key2Index(key)
		if i < 0 {
			return fmt.Errorf("set key(%s) for array", key)
		}

		if i >= len(self.values) {
			self.values = append(self.values, make([]interface{}, i-len(self.values)+1)...)
		}
		self.values[i] = value
	} else {
		if inx, ok := self.keys[key]; ok {
			self.values[inx] = value
			self.ordKeys[inx] = key
			//fmt.Printf("update key %s %d\n", key, inx)
			return nil
		}

		inx := uint32(len(self.values))
		self.keys[key] = inx
		self.values = append(self.values, value)
		self.ordKeys = append(self.ordKeys, key)
		//fmt.Printf("set key %p %s %d\n", self, key, inx)
	}
	return nil
}

func (self *DynamicJSON) SetI(i int, value interface{}) error {

	if self.iterCounter < 0 {
		return fmt.Errorf("Modification attempt of frozen array [%d] = %v", i, value)
	}

	if self.IsArray() {
		if i < 0 {
			return fmt.Errorf("Invalid index: %d", i)
		}

		if i >= len(self.values) {
			self.values = append(self.values, make([]interface{}, i-len(self.values)+1)...)
		}
		self.values[i] = value
		return nil
	}

	return self.set(fmt.Sprint(i), value)
}

func (self *DynamicJSON) get(key string) (interface{}, bool) {
	if self == nil {
		return nil, false
	}

	if self.IsArray() {
		i := key2Index(key)
		if i < 0 {
			return nil, false
		}
		if i < len(self.values) {
			return self.values[i], true
		}
		return nil, false
	}

	inx, ok := self.keys[key]

	if !ok {
		return nil, false
	}

	return self.values[inx], true
}

func (self *DynamicJSON) Delete(path string) error {

	if self == nil {
		return nil
	}

	if self.iterCounter < 0 {
		return fmt.Errorf("Modification attempt of frozen djson %s", path)
	}

	i := strings.IndexByte(path, byte('/'))
	if i >= 0 {
		dir := path[:i]
		rest := path[i+1:]
		self.Nested(dir).Delete(rest)
		return nil
	}

	if self.IsArray() {
		i := key2Index(path)
		if i < 0 {
			return nil
		}
		if i < len(self.values) {
			s := self.values
			self.values = append(s[:i], s[i+1:]...)
			s[len(s)-1] = nil
		}
		return nil
	}

	inx, ok := self.keys[path]

	if !ok {
		return nil
	}
	self.values[inx] = gDeletedEntry
	delete(self.keys, path)

	self.packing()
	return nil
}

func (self *DynamicJSON) Has(path string) bool {
	if self == nil {
		return false
	}
	i := strings.IndexByte(path, byte('/'))
	if i >= 0 {
		dir := path[:i]
		rest := path[i+1:]
		return self.Nested(dir).Has(rest)
	}

	if self.IsArray() {
		i := key2Index(path)
		if i < 0 {
			return false
		}
		if i < len(self.values) {
			return true
		}
		return false
	}

	_, ok := self.keys[path]
	return ok
}

func (self *DynamicJSON) Keys() []string {

	if self == nil {
		return nil
	}

	if self.IsArray() {
		r := make([]string, len(self.values))
		for i := 0; i < len(self.values); i++ {
			r[i] = fmt.Sprint(i)
		}
		return r
	}

	var r []string
	for i := 0; i < len(self.values); i++ {

		if self.values[i] == gDeletedEntry {
			continue
		}

		r = append(r, self.ordKeys[i])
	}
	return r
}

func (self *DynamicJSON) Iterate(cb func(key string, value interface{}) bool) {

	if self == nil {
		return
	}

	if self.IsArray() {
		for i := 0; i < len(self.values); i++ {
			if !cb("", self.values[i]) {
				break
			}
		}
		return
	}

	if self.iterCounter >= 0 {
		self.iterCounter++
		defer func() {
			self.iterCounter--
		}()
	}

	for i := 0; i < len(self.values); i++ {

		if self.values[i] == gDeletedEntry {
			continue
		}

		if !cb(self.ordKeys[i], self.values[i]) {
			break
		}
	}
	self.packing()
}

// self must not be changed
func (self *DynamicJSON) Visit(cb func(path string, value interface{})) {
	self.visit("", cb)
}

func (self *DynamicJSON) visit(path string, cb func(fullpath string, value interface{})) {

	if self == nil {
		return
	}

	prefix := path

	if path != "" {
		prefix = path + "/"
	}

	if self.IsArray() {
		for i := 0; i < len(self.values); i++ {

			p := prefix + fmt.Sprint(i)
			cb(p, self.values[i])
			if d, ok := self.values[i].(*DynamicJSON); ok {
				d.visit(p, cb)
			}
		}
		return
	}

	for i := 0; i < len(self.values); i++ {
		if self.values[i] == gDeletedEntry {
			continue
		}

		p := prefix + self.ordKeys[i]
		cb(p, self.values[i])
		if d, ok := self.values[i].(*DynamicJSON); ok {
			d.visit(p, cb)
		}
	}
}

func (self *DynamicJSON) packing() {
	if self == nil || self.iterCounter != 0 || self.IsArray() {
		return
	}

	// Simple stupid algorithm
	if len(self.values) >= 128 && len(self.keys)*3 < len(self.values) {

		values := make([]interface{}, len(self.keys))
		ordKeys := make([]string, len(self.keys))
		var j uint32 = 0
		for i := 0; i < len(self.values); i++ {

			if self.values[i] == gDeletedEntry {
				continue
			}

			values[j] = self.values[i]
			ordKeys[j] = self.ordKeys[i]
			self.keys[ordKeys[j]] = j
			j++
		}
		self.values = values
		self.ordKeys = ordKeys
	}
}

func (self *DynamicJSON) Len() int {
	if self == nil {
		return 0
	}
	if self.IsArray() {
		return len(self.values)
	}
	return len(self.keys)
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (self *DynamicJSON) Nested(path string) *DynamicJSON {

	if self == nil {
		return nil
	}

	v, ok := self.doOp(path, false, false, nil)

	if !ok {
		return nil
	}

	if v, ok := v.(*DynamicJSON); ok {
		return v
	}

	return nil
}

func (self *DynamicJSON) NestedI(i int) *DynamicJSON {

	if self == nil {
		return nil
	}

	if !self.IsArray() {
		return nil
	}

	if i < len(self.values) {
		v, ok := self.values[i].(*DynamicJSON)
		if ok {
			return v
		}
	}

	return nil
}

func (self *DynamicJSON) Get(path string) any {
	v, ok := self.doOp(path, false, false, nil)
	if ok {
		return v
	}

	return nil
}

func (self *DynamicJSON) GetI(i int) any {

	if self == nil {
		return nil
	}

	if !self.IsArray() {
		return nil
	}

	if i < len(self.values) {
		return self.values[i]
	}

	return nil
}

func (self *DynamicJSON) Append(v any) {

	if self == nil {
		return
	}

	if self.IsArray() {
		self.values = append(self.values, v)
	}
}

func (self *DynamicJSON) Array(name string) *DynamicJSON {

	if self == nil {
		return nil
	}

	v, ok := self.get(name)

	if ok {
		a, ok := v.(*DynamicJSON)
		if ok && a.IsArray() {
			return a
		}
	}

	x := NewArray()
	self.set(name, x)
	return x
}

func (self *DynamicJSON) ArrayI(i int) *DynamicJSON {

	if self == nil {
		return nil
	}

	if !self.IsArray() {
		return self.Array(fmt.Sprint(i))
	}

	if i < len(self.values) {
		v := self.values[i]
		a, ok := v.(*DynamicJSON)
		if ok && a.IsArray() {
			return a
		}
	}

	x := NewArray()
	self.SetI(i, x)
	return x
}

func (self *DynamicJSON) Map(name string) *DynamicJSON {

	if self == nil {
		return nil
	}

	v, ok := self.get(name)

	if ok {
		m, ok := v.(*DynamicJSON)
		if ok && !m.IsArray() {
			return m
		}
	}
	x := NewMap()
	self.set(name, x)
	return x
}

func (self *DynamicJSON) MapI(i int) *DynamicJSON {

	if self == nil {
		return nil
	}

	if !self.IsArray() {
		return self.Array(fmt.Sprint(i))
	}

	if i < len(self.values) {
		v := self.values[i]
		m, ok := v.(*DynamicJSON)
		if ok && !m.IsArray() {
			return m
		}
	}

	x := NewMap()
	self.SetI(i, x)
	return x
}

func (self *DynamicJSON) Fetch(path string) (any, bool) {
	v, ok := self.doOp(path, false, false, nil)
	if ok {
		return v, true
	}

	return nil, false
}

func createLevelFromNextPath(path string) *DynamicJSON {
	i := strings.IndexByte(path, byte('/'))
	name := path
	if i >= 0 {
		name = path[:i]
	}
	if key2Index(name) >= 0 {
		return NewArray()
	}
	return NewMap()
}

func (self *DynamicJSON) IsFrozen() bool {
	return self.iterCounter < 0
}

func (self *DynamicJSON) doOp(path string, autoCreate bool, setValue bool, value interface{}) (interface{}, bool) {

	if setValue && self.iterCounter < 0 {
		// to Error log ("Modification attempt of frozen map %s:%v", path, value)
		return nil, false
	}

	level := self
	for {
		if len(path) == 0 {
			return level, true
		}
		i := strings.IndexByte(path, byte('/'))
		if i == 0 {
			path = path[1:]
			continue
		}
		var name string
		if i > 0 {
			name = path[:i]
			path = path[i+1:]
			next, ok := level.get(name)
			if ok {
				if nextMap, ok := next.(*DynamicJSON); ok {
					level = nextMap
					continue
				}
			}

			if autoCreate {
				nextMap := createLevelFromNextPath(path)
				level.set(name, nextMap)
				level = nextMap
			} else {
				return nil, false
			}

		} else {
			name = path

			if setValue {
				level.set(name, value) // TODO: add speed optimization
			}
			v, ok := level.get(name)
			if ok {
				return v, true
			}
			return nil, false
		}
	}
}

func (self *DynamicJSON) Set(path string, value interface{}) {
	_, _ = self.doOp(path, true, true, convertToDJ(value))
}

func convertToDJ(v interface{}) interface{} {

	if v == nil {
		return nil
	}

	if _, ok := v.(*DynamicJSON); ok {
		return v
	}

	if a, ok := v.([]interface{}); ok {
		r := NewArray()
		for _, x := range a {
			r.values = append(r.values, convertToDJ(x))
		}
		return r
	}

	if g, ok := v.(map[string]interface{}); ok {
		r := NewMap()
		for nm, x := range g {
			r.Set(nm, convertToDJ(x))
		}
		return r
	}

	t := reflect.TypeOf(v)
	val := reflect.ValueOf(v)

	switch t.Kind() {
	case reflect.Slice | reflect.Array:
		n := val.Len()
		r := NewArray()
		for i := 0; i < n; i++ {
			r.values = append(r.values, convertToDJ(val.Index(i).Interface()))
		}
		return r
	case reflect.Pointer:
		return nil
	case reflect.Map:
		r := NewMap()
		for _, inx := range val.MapKeys() {
			r.Set(inx.String(), val.MapIndex(inx).Interface())
		}
		return r
	}

	return v
}

func (self *DynamicJSON) GetTime(path string) time.Time {

	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return time.Time{}
	}

	if t, ok := v.(time.Time); ok {
		return t
	}

	if s, ok := v.(string); ok {

		if strings.Contains(s, ".") {
			tm, err := time.Parse(time.RFC3339Nano, s)
			if err == nil {
				return tm
			}
		}
		tm, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}
		}
		return tm
	}

	return time.Time{}
}

func value2int(v interface{}, defaultValue int) int {
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

func (self *DynamicJSON) GetInt(path string, defaultValue int) int {

	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return defaultValue
	}

	return value2int(v, defaultValue)
}

func (self *DynamicJSON) GetFloat(path string, defaultValue float64) float64 {
	v, ok := self.doOp(path, false, false, nil)
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

func (self *DynamicJSON) GetBool(path string, defaultValue bool) bool {

	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return defaultValue
	}

	if n, ok := v.(bool); ok {
		return n
	}
	return defaultValue
}

func value2string(v interface{}, defaultValue string) string {
	if s, ok := v.(string); ok {
		return s
	}
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	if n, ok := v.(int); ok {
		return strconv.Itoa(n)
	}
	if n, ok := v.(int64); ok {
		return fmt.Sprint(n)
	}
	if j, ok := v.(DynamicJSON); ok {
		return string(j.JSONLine())
	}
	if tm, ok := v.(time.Time); ok {
		return tm.Format(time.RFC3339Nano)
	}
	return defaultValue

}

func (self *DynamicJSON) GetString(path string, defaultValue string) string {

	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return defaultValue
	}

	return value2string(v, defaultValue)
}

func (self *DynamicJSON) GetStr(path string) string {

	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}

	return ""
}

func (self *DynamicJSON) GetSlice(path string) []*DynamicJSON {

	if self == nil {
		return nil
	}
	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return nil
	}

	if g, ok := v.(*DynamicJSON); ok {
		if g.IsArray() {
			r := make([]*DynamicJSON, g.Len())

			for i := range g.values {
				r[i], _ = g.values[i].(*DynamicJSON)
			}
			return r
		}
	}
	return nil
}

func (self *DynamicJSON) Each(path string) iter.Seq[*DynamicJSON] {

	return func(yield func(*DynamicJSON) bool) {
		if self == nil {
			return
		}
		v, ok := self.doOp(path, false, false, nil)
		if !ok {
			return
		}

		if g, ok := v.(*DynamicJSON); ok {
			if g.IsArray() {
				for _, x := range g.values {
					if !yield(x.(*DynamicJSON)) {
						return
					}
				}
				return
			}
		}
	}
}

func (self *DynamicJSON) EachPair(path string) iter.Seq2[string, any] {

	return func(yield func(string, any) bool) {
		if self == nil {
			return
		}
		v, ok := self.doOp(path, false, false, nil)
		if !ok {
			return
		}

		if g, ok := v.(*DynamicJSON); ok {
			if g.IsArray() {
				for _, x := range g.values {
					if !yield("", x) {
						return
					}
				}
				return
			}

			if g.iterCounter >= 0 {
				g.iterCounter++
				defer func() {
					g.iterCounter--
				}()
			}

			for i := 0; i < len(g.values); i++ {

				if g.values[i] == gDeletedEntry {
					continue
				}

				if !yield(g.ordKeys[i], g.values[i]) {
					break
				}
			}
			g.packing()
		}
	}
}

func (self *DynamicJSON) GetStringsSlice(path string) []string {

	if self == nil {
		return nil
	}

	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return nil
	}

	if g, ok := v.(*DynamicJSON); ok {
		if g.IsArray() {
			r := make([]string, g.Len())

			for i := range g.values {
				r[i] = value2string(g.values[i], "")
			}
			return r
		}
	}
	return nil
}

func (self *DynamicJSON) GetAnySlice(path string) []any {

	if self == nil {
		return nil
	}

	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return nil
	}

	if g, ok := v.(*DynamicJSON); ok {
		if g.IsArray() {
			r := make([]any, g.Len())

			for i := range g.values {
				r[i] = g.values[i]
			}
			return r
		}
	}
	return nil
}

func (self *DynamicJSON) GetIntsSlice(path string) []int {

	if self == nil {
		return nil
	}

	v, ok := self.doOp(path, false, false, nil)
	if !ok {
		return nil
	}

	if g, ok := v.(*DynamicJSON); ok {
		if g.IsArray() {
			r := make([]int, g.Len())

			for i := range g.values {
				r[i] = value2int(g.values[i], 0)
			}
			return r
		}
	}
	return nil
}

func (self *DynamicJSON) JSON() []byte {
	var identBuf [64]byte
	buf := &bytes.Buffer{}
	self.writeTo(buf, true, identBuf[:0])
	return buf.Bytes()
}

func (self *DynamicJSON) JSONLine() []byte {
	var identBuf [64]byte
	buf := &bytes.Buffer{}
	self.writeTo(buf, false, identBuf[:0])
	return buf.Bytes()
}

var gPrettyIdent = []byte{' ', ' '}
var gEndLine = []byte{'\n'}
var gMapBegin = []byte{'{'}
var gMapEnd = []byte{'}'}
var gEmptyMap = []byte{'{', '}'}
var gArrayBegin = []byte{'['}
var gArrayEnd = []byte{']'}
var gEmptyArray = []byte{'[', ']'}
var gComma = []byte{','}
var gPrettyKVSep = []byte{':', ' '}
var gKVSep = []byte{':'}

func (self *DynamicJSON) writeTo(w *bytes.Buffer, pretty bool, ident []byte) {

	var bufStorage [256]byte

	nestedIdent := ident

	if self.IsArray() {
		if self.Len() == 0 {
			w.Write(gEmptyArray)
			return
		}
		w.Write(gArrayBegin)

		if pretty {
			nestedIdent = append(ident, gPrettyIdent...)
			w.Write(gEndLine)
			w.Write(nestedIdent)

		}
		for idx, v := range self.values {
			if idx != 0 {
				w.Write(gComma)
				if pretty {
					w.Write(gEndLine)
					w.Write(nestedIdent)
				}
			}
			if container, ok := v.(*DynamicJSON); ok {
				container.writeTo(w, pretty, nestedIdent)
			} else {

				if tm, ok := v.(time.Time); ok {
					v = tm.Format(time.RFC3339Nano)
				}

				b, _ := njson.Append(bufStorage[:0], v, njson.EscapeHTML)
				w.Write(b)
			}
		}
		if pretty {
			w.Write(gEndLine)
			w.Write(ident)
		}
		w.Write(gArrayEnd)
	} else { // map

		if self.Len() == 0 {
			w.Write(gEmptyMap)
			return
		}

		w.Write(gMapBegin)

		if pretty {
			nestedIdent = append(ident, gPrettyIdent...)
			w.Write(gEndLine)
			w.Write(nestedIdent)
		}
		idx := 0
		for i, v := range self.values {

			if self.values[i] == gDeletedEntry {
				continue
			}

			if idx != 0 {
				w.Write(gComma)
				if pretty {
					w.Write(gEndLine)
					w.Write(nestedIdent)
				}
			}

			encodeString(w, self.ordKeys[i])

			if pretty {
				w.Write(gPrettyKVSep)
			} else {
				w.Write(gKVSep)
			}

			if container, ok := v.(*DynamicJSON); ok {
				container.writeTo(w, pretty, nestedIdent)
			} else {

				if tm, ok := v.(time.Time); ok {
					v = tm.Format(time.RFC3339Nano)
				}

				b, _ := njson.Append(bufStorage[:0], v, njson.EscapeHTML)
				w.Write(b)
			}
			idx++
		}
		if pretty {
			w.Write(gEndLine)
			w.Write(ident)
		}
		w.Write(gMapEnd)
	}
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func token2value(tok *njson.Tokenizer, strData string) any {

	switch tok.Kind() {
	case njson.Array:
		r := NewArray()

		for tok.Next() {
			if tok.Err != nil {
				return r
			}

			if tok.Delim == ']' {
				return r
			}

			if tok.Delim == ',' {
				if !tok.Next() {
					return r
				}
			}

			r.values = append(r.values, token2value(tok, strData))
		}
		return r
	case njson.Object:
		r := NewMap()

		for tok.Next() {
			if tok.Err != nil {
				return r
			}

			if tok.Delim == ',' {
				if !tok.Next() {
					return r
				}
			}

			if tok.Delim == '}' {
				return r
			}

			var key string

			if tok.Kind() == njson.Unescaped && len(tok.Value) > 1 {
				end := len(strData) - tok.Remaining()
				start := end - len(tok.Value)
				key = strData[start+1 : end-1]
			} else {
				key = string(tok.String())
			}

			tok.Next()

			if !tok.Next() {
				return r
			}

			value := token2value(tok, strData)
			r.set(key, value)
		}
		return r

	case njson.False:
		return false
	case njson.True:
		return true
	case njson.Null:
		return nil
	case njson.Unescaped:
		end := len(strData) - tok.Remaining()
		start := end - len(tok.Value)
		return strData[start+1 : end-1]
	default:

		if tok.Kind()&njson.String != 0 {
			return string(tok.String())
		}

		if tok.Kind()&njson.Num != 0 {
			end := len(strData) - tok.Remaining()
			start := end - len(tok.Value)
			return json.Number(strData[start:end])
		}
	}

	tok.Err = fmt.Errorf("unexpected kind %T %v", tok.Kind(), tok.Kind())
	return nil
}

func Parse(data []byte) (*DynamicJSON, error) {

	strData := string(data)
	tok := njson.NewTokenizer(data)

	tok.Next()
	if tok.Delim != '{' && tok.Delim != '[' {
		part := data
		if len(part) > 50 {
			part = part[:50]
		}
		return nil, fmt.Errorf("is not a map or array: %s", string(part))
	}
	v := token2value(tok, strData)

	if tok.Err != nil {
		return nil, tok.Err
	}
	if r, ok := v.(*DynamicJSON); ok {
		return r, nil
	}
	return nil, fmt.Errorf("Internal error %T %v", v, v)
}

func cloneValue(value any) any {
	if v, ok := value.(*DynamicJSON); ok {
		return v.Clone()
	}
	return value
}

func (self *DynamicJSON) Clone() *DynamicJSON {
	if self == nil {
		return nil
	}

	if self.IsArray() {
		x := NewArray()
		for _, v := range self.values {
			x.values = append(x.values, cloneValue(v))
		}
		return x
	}

	x := NewMap()
	for i, v := range self.values {

		if self.values[i] == gDeletedEntry {
			continue
		}

		x.Set(self.ordKeys[i], cloneValue(v))
	}
	return x
}

func (self *DynamicJSON) IsEqual(o *DynamicJSON) bool {
	if self == o {
		return true
	}

	if self == nil {
		return false
	}

	array1 := self.IsArray()
	array2 := o.IsArray()

	if array1 != array2 {
		return false
	}

	if array1 {

		if len(self.values) != len(o.values) {
			return false
		}
		for i, value1 := range self.values {

			value2 := o.values[i]

			d1, ok1 := value1.(*DynamicJSON)
			d2, ok2 := value2.(*DynamicJSON)

			if ok1 && ok2 {
				if !d1.IsEqual(d2) {
					return false
				}
			} else {
				if !reflect.DeepEqual(value1, value2) {
					return false
				}
			}
		}
		return true
	}

	// map & map

	if len(self.keys) != len(o.keys) {
		return false
	}

	for i := 0; i < len(self.values); i++ {

		if self.values[i] == gDeletedEntry {
			continue
		}

		key1 := self.ordKeys[i]
		value1 := self.values[i]

		inx, ok := o.keys[key1]

		if !ok {
			return false
		}

		value2 := o.values[inx]

		d1, ok1 := value1.(*DynamicJSON)
		d2, ok2 := value2.(*DynamicJSON)

		if ok1 && ok2 {
			if !d1.IsEqual(d2) {
				return false
			}
		} else {
			if !reflect.DeepEqual(value1, value2) {
				return false
			}
		}
	}
	return true
}

func scalar2str(value interface{}) string {
	var bufStorage [256]byte

	if tm, ok := value.(time.Time); ok {
		value = tm.Format(time.RFC3339Nano)
	}

	b, _ := njson.Append(bufStorage[:0], value, njson.EscapeHTML)
	return string(b)
}

func (self *DynamicJSON) IsEqualAsString(o *DynamicJSON) bool {
	if self == o {
		return true
	}

	if self == nil {
		return false
	}

	array1 := self.IsArray()
	array2 := o.IsArray()

	if array1 != array2 {
		return false
	}

	if array1 {

		if len(self.values) != len(o.values) {
			return false
		}
		for i, value1 := range self.values {

			value2 := o.values[i]

			d1, ok1 := value1.(*DynamicJSON)
			d2, ok2 := value2.(*DynamicJSON)

			if ok1 && ok2 {
				if !d1.IsEqualAsString(d2) {
					return false
				}
			} else {
				if scalar2str(value1) != scalar2str(value2) {
					return false
				}
			}
		}
		return true
	}

	// map & map

	if len(self.keys) != len(o.keys) {
		return false
	}

	for i := 0; i < len(self.values); i++ {

		if self.values[i] == gDeletedEntry {
			continue
		}

		key1 := self.ordKeys[i]
		value1 := self.values[i]

		inx, ok := o.keys[key1]

		if !ok {
			return false
		}

		value2 := o.values[inx]

		d1, ok1 := value1.(*DynamicJSON)
		d2, ok2 := value2.(*DynamicJSON)

		if ok1 && ok2 {
			if !d1.IsEqualAsString(d2) {
				return false
			}
		} else {
			x1 := scalar2str(value1)
			x2 := scalar2str(value2)
			//if scalar2str(value1) != scalar2str(value2) {
			if x1 != x2 {
				return false
			}
		}
	}
	return true
}

func (self *DynamicJSON) IsEqualCheck(o *DynamicJSON) bool {
	if self == o {
		return true
	}

	if self == nil {
		// Debugf("diff: self is nil")
		return false
	}

	array1 := self.IsArray()
	array2 := o.IsArray()

	if array1 != array2 {
		// Debugf("diff: array is not array")
		return false
	}

	if array1 {

		if len(self.values) != len(o.values) {
			// Debugf("diff: array(%d) != array(%d)", len(self.values), len(o.values))
			return false
		}
		for i, value1 := range self.values {

			value2 := o.values[i]

			d1, ok1 := value1.(*DynamicJSON)
			d2, ok2 := value2.(*DynamicJSON)

			if ok1 && ok2 {
				if !d1.IsEqualCheck(d2) {
					// Debugf("diff nested[%d]", i)
					return false
				}
			} else {
				if scalar2str(value1) != scalar2str(value2) {
					// Debugf("scalar diff %d %s %s", i, scalar2str(value1), scalar2str(value2))
					return false
				}
			}
		}
		return true
	}

	// map & map

	if len(self.keys) != len(o.keys) {
		for k := range self.keys {
			if _, ok := o.keys[k]; !ok {
				// Debugf("ddiff maps < %s", k)
			}
		}
		for k := range o.keys {
			if _, ok := self.keys[k]; !ok {
				// Debugf("ddiff maps > %s", k)
			}
		}
		return false
	}

	for i := 0; i < len(self.values); i++ {

		if self.values[i] == gDeletedEntry {
			continue
		}

		key1 := self.ordKeys[i]
		value1 := self.values[i]

		inx, ok := o.keys[key1]

		if !ok {
			// Debugf("diff >%s", key1)
			return false
		}

		value2 := o.values[inx]

		d1, ok1 := value1.(*DynamicJSON)
		d2, ok2 := value2.(*DynamicJSON)

		if ok1 && ok2 {
			if !d1.IsEqualCheck(d2) {
				// Debugf("diff tested[%s]", key1)
				return false
			}
		} else {
			x1 := scalar2str(value1)
			x2 := scalar2str(value2)
			if x1 != x2 {
				// Debugf("scalar ddiff %s %s %s", key1, x1, x2)
				return false
			}
		}
	}
	return true
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func FromResponse(resp *http.Response, err0 error) (response *DynamicJSON, err error) {
	if err0 != nil {
		return nil, err0
	}

	defer resp.Body.Close()

	reader := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		defer reader.Close()
	}

	body, err := io.ReadAll(reader)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("%s", resp.Status)
	}

	r, err2 := Parse(body)
	if err != nil {
		err2 = err
	}
	return r, err2
}

func FromResponse200(resp *http.Response, err0 error) (response *DynamicJSON, err error) {
	if err0 != nil {
		return nil, err0
	}

	defer resp.Body.Close()

	reader := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		defer reader.Close()
	}

	body, err := io.ReadAll(reader)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("StatusCode %v", resp.StatusCode)
	}

	if len(body) <= 2 {
		return nil, fmt.Errorf("response '%s'", body)
	}

	return Parse(body)
}

func FromFile(filepath string) (r *DynamicJSON, err error) {

	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	r, err = Parse(data)
	return
}

func FromFolder(path string) ([]*DynamicJSON, error) {

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var objects []*DynamicJSON

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r, err := FromFile(filepath.Join(path, e.Name()))

		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}

		objects = append(objects, r)
	}
	return objects, nil
}
