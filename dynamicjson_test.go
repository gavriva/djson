package djson_test

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/gavriva/djson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test1(t *testing.T) {

	djs := djson.NewMap()
	djs.Set("a", 1)
	djs.Set("a/b/c", 1)

	assert.Equal(t, 1, djs.GetInt("a/b/c", 10))
	assert.Equal(t, 1, djs.Nested("a/b").GetInt("c", 10))
}

func TestArray(t *testing.T) {

	djs := djson.NewArray()
	djs.Set("1", 11)
	djs.Set("0", 10)

	assert.Equal(t, 11, djs.GetInt("1", -1))
	assert.Equal(t, 10, djs.GetInt("0", -1))

	var raw string = `[10,11]`
	assert.Equal(t, raw, string(djs.JSONLine()))
}

func TestNumber(t *testing.T) {

	var raw string = `{"x":100}`
	o, err := djson.Parse([]byte(raw))
	assert.NoError(t, err)

	assert.Equal(t, 100, o.GetInt("x", 1))
	assert.Equal(t, raw, string(o.JSONLine()))
}

func TestFloat(t *testing.T) {

	o, err := djson.Parse([]byte(`{"x":117.1}`))
	assert.NoError(t, err)

	assert.Equal(t, 117.1, o.GetFloat("x", 1))
	assert.Equal(t, 117.1, o.Clone().GetFloat("x", 1))
}

func TestReflectSet(t *testing.T) {

	o := djson.NewMap()
	o.Set("a", []int{1, 2, 3})
	o.Set("b", []*djson.DynamicJSON{djson.NewArray(), djson.NewMap()})
	assert.Equal(t, 3, o.Nested("a").Len())
	assert.Equal(t, 2, o.Nested("b").Len())

	o2 := o.Clone()

	r := djson.NewMap()

	r.Set("values", map[string]*djson.DynamicJSON{
		"o1": o,
		"o2": o2,
	})

	assert.Equal(t, 3, r.Nested("values/o1/a").Len())
	assert.Equal(t, 2, r.Nested("values/o2/b").Len())
	assert.Equal(t, 2, r.Nested("values").Len())
}

func TestNesting(t *testing.T) {

	o := djson.NewMap()
	o.Array("a").MapI(0).Map("b")

	assert.EqualValues(t, `{"a":[{"b":{}}]}`, string(o.JSONLine()))

	o = djson.NewMap()
	o.Array("a").SetI(0, 11)
	assert.EqualValues(t, `{"a":[11]}`, string(o.JSONLine()))

	o.Map("a").SetI(0, 12)
	assert.EqualValues(t, `{"a":{"0":12}}`, string(o.JSONLine()))

	o.Array("a").ArrayI(1)
	assert.EqualValues(t, `{"a":[null,[]]}`, string(o.JSONLine()))
}

func TestBasic(t *testing.T) {

	n := djson.NewMap()

	n.Set("History/0/Timestamp", time.Now())
	n.Set("History/0/Approve", true)

	assert.Equal(t, 1, n.Len())
}

func TestVisit(t *testing.T) {

	n := djson.NewMap()

	n.Set("History/0/Timestamp", time.Now())
	n.Set("History/0/Approve", true)
	n.Set("App", true)
	n.Set("History/1/Approve", true)

	var paths []string
	n.Visit(func(path string, _ interface{}) {
		paths = append(paths, path)
	})
	assert.EqualValues(t, 7, len(paths))
}

func roundJS(price float64) float64 {
	return math.Round(price*1000.0) / 1000.0
}

func TestJSONFloats(t *testing.T) {

	n, err := djson.Parse([]byte(`{"v":31167.0}`))
	require.NoError(t, err)

	n.Set("v2", json.Number(fmt.Sprintf("%g", n.GetFloat("v", 0)/100.0)))
	x1 := n.GetFloat("v2", 0)
	x2 := x1 * 10.0
	x2 *= 10.0
	x3 := x1 * 100.0

	n.Set("v3", roundJS(x2))
	n.Set("v4", roundJS(x3))

	assert.EqualValues(t, `{"v":31167.0,"v2":311.67,"v3":31167,"v4":31167}`, string(n.JSONLine()))
}

func TestFreeze(t *testing.T) {

	n := djson.NewMap()

	n.Set("a", 1)
	assert.EqualValues(t, 1, n.Len())
	n.Freeze()
	n.Set("b", 2)
	assert.EqualValues(t, 1, n.Len())

	a := djson.NewArray()
	a.Set("0", -1)
	assert.EqualValues(t, 1, a.Len())
	a.Freeze()
	a.Set("1", -2)
	assert.EqualValues(t, 1, a.Len())

	x := djson.NewMap()
	x.Set("n", n)
	x.Set("a", a)
	x.Set("n/b", 2)
	x.Set("a/1", -2)
	assert.EqualValues(t, 1, n.Len())
	assert.EqualValues(t, 1, a.Len())
	assert.EqualValues(t, 1, x.Nested("n").Len())
	assert.EqualValues(t, 1, x.Nested("a").Len())
}

func TestBasicFeatures3(t *testing.T) {
	n := 100
	om := djson.NewMap()

	for i := 0; i < n; i++ {
		assert.Equal(t, i, om.Len())
		om.Set(fmt.Sprint(i), 2*i)
		assert.Equal(t, i+1, om.Len())
	}

	// get what we just set
	for i := 0; i < n; i++ {
		key := fmt.Sprint(i)
		value, present := om.Fetch(key)

		assert.Equal(t, 2*i, value)
		assert.True(t, present)
		assert.True(t, om.Has(key))
	}

	i := 0
	om.Iterate(func(key string, value interface{}) bool {
		expectedKey := fmt.Sprint(i)
		assert.Equal(t, expectedKey, key)
		assert.Equal(t, 2*i, value)
		i++
		return true
	})
	assert.Equal(t, om.Len(), i)

	i = 0
	om.Iterate(func(_ string, _ interface{}) bool {
		i++
		return false
	})
	assert.Equal(t, 1, i)

	// double values for pairs with even keys

	for j := 0; j < n/2; j++ {
		i = 2 * j
		key := fmt.Sprint(i)
		oldValue, present := om.Fetch(key)
		om.Set(key, 4*i)

		assert.Equal(t, 2*i, oldValue)
		assert.True(t, present)
	}

	// and delete pairs with odd keys

	for j := 0; j < n/2; j++ {
		i = 2*j + 1
		assert.Equal(t, om.Len(), n-j)

		key := fmt.Sprint(i)
		value, present := om.Fetch(key)
		om.Delete(key)
		assert.Equal(t, om.Len(), n-j-1)

		assert.Equal(t, 2*i, value)
		assert.True(t, present)

		// deleting again shouldn't change anything

		value, present = om.Fetch(key)
		om.Delete(key)
		assert.Equal(t, om.Len(), n-j-1)
		assert.Nil(t, value)
		assert.False(t, present)
	}

	// get the whole range

	for j := 0; j < n/2; j++ {
		i = 2 * j
		key := fmt.Sprint(i)
		value, present := om.Fetch(key)
		assert.Equal(t, 4*i, value)
		assert.True(t, present)
		assert.True(t, om.Has(key))

		i = 2*j + 1
		key = fmt.Sprint(i)
		value, present = om.Fetch(key)
		assert.Nil(t, value)
		assert.False(t, present)
		assert.False(t, om.Has(key))
	}

	// check iterations again
	i = 0
	om.Iterate(func(key string, value interface{}) bool {
		expectedKey := fmt.Sprint(i)
		assert.Equal(t, expectedKey, key)
		assert.Equal(t, 4*i, value)
		i += 2
		return true
	})

	assert.Equal(t, om.Len(), i/2)
}

func TestUpdatingDoesntChangePairsOrder3(t *testing.T) {
	om := djson.NewMap()
	om.Set("foo", "bar")
	om.Set("12", 28)
	om.Set("78", 100)
	om.Set("bar", "baz")

	oldValue, present := om.Fetch("78")
	om.Set("78", 102)
	assert.Equal(t, 100, oldValue)
	assert.True(t, present)

	assertOrderedPairsEqual3(t, om,
		[]string{"foo", "12", "78", "bar"},
		[]interface{}{"bar", 28, 102, "baz"})
}

func TestDeletingAndReinsertingChangesPairsOrder3(t *testing.T) {
	om := djson.NewMap()
	om.Set("foo", "bar")
	om.Set("12", 28)
	om.Set("78", 100)
	om.Set("bar", "baz")

	// delete a pair
	oldValue, present := om.Fetch("78")
	om.Delete("78")
	assert.Equal(t, 100, oldValue)
	assert.True(t, present)

	// re-insert the same pair
	oldValue, present = om.Fetch("78")
	om.Set("78", 100)
	assert.Nil(t, oldValue)
	assert.False(t, present)

	assertOrderedPairsEqual3(t, om,
		[]string{"foo", "12", "bar", "78"},
		[]interface{}{"bar", 28, "baz", 100})
}

func TestEmptyMapOperations3(t *testing.T) {
	om := djson.NewMap()

	oldValue, present := om.Fetch("foo")
	assert.Nil(t, oldValue)
	assert.False(t, present)

	oldValue, present = om.Fetch("bar")
	om.Delete("bar")
	assert.Nil(t, oldValue)
	assert.False(t, present)

	assert.Equal(t, om.Len(), 0)

	i := 0
	om.Iterate(func(_ string, _ interface{}) bool {
		i++
		return true
	})

	assert.Zero(t, i)
}

func TestNilMapOperations3(t *testing.T) {
	var om *djson.DynamicJSON

	oldValue, present := om.Fetch("foo")
	assert.Nil(t, oldValue)
	assert.False(t, present)

	om.Delete("bar")
	oldValue, present = om.Fetch("bar")
	assert.Nil(t, oldValue)
	assert.False(t, present)

	assert.False(t, om.Has("bar"))

	assert.Equal(t, om.Len(), 0)

	i := 0
	om.Iterate(func(_ string, _ interface{}) bool {
		i++
		return true
	})

	assert.Zero(t, i)
}

func TestShuffle3(t *testing.T) {
	ranLen := 100

	for _, n := range []int{0, 10, 20, 100, 1000, 10000} {
		t.Run(fmt.Sprintf("shuffle test with %d items", n), func(t *testing.T) {
			om := djson.NewMap()

			keys := make([]string, n)
			values := make([]interface{}, n)

			for i := 0; i < n; i++ {
				// we prefix with the number to ensure that we don't get any duplicates
				keys[i] = fmt.Sprintf("%d_%s", i, randomHexString(t, ranLen))
				values[i] = randomHexString(t, ranLen)

				value, present := om.Fetch(keys[i])
				om.Set(keys[i], values[i])
				assert.Nil(t, value)
				assert.False(t, present)
				assert.True(t, om.Has(keys[i]))
			}

			assertOrderedPairsEqual3(t, om, keys, values)
		})
	}
}

func randomHexString(t *testing.T, length int) string {
	b := length / 2
	randBytes := make([]byte, b)

	if n, err := rand.Read(randBytes); err != nil || n != b {
		if err == nil {
			err = fmt.Errorf("only got %v random bytes, expected %v", n, b)
		}
		t.Fatal(err)
	}

	return hex.EncodeToString(randBytes)
}

func assertOrderedPairsEqual3(t *testing.T, om *djson.DynamicJSON, expectedKeys []string, expectedValues []interface{}) {

	if assert.Equal(t, len(expectedKeys), len(expectedValues)) && assert.Equal(t, len(expectedKeys), om.Len()) {
		i := 0

		om.Iterate(func(key string, value interface{}) bool {
			assert.Equal(t, expectedKeys[i], key)
			assert.Equal(t, expectedValues[i], value)
			i++
			return true
		})
	}
}
