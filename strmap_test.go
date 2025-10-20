package djson

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Tests are shamelessly stolen from https://github.com/wk8/go-ordered-map/blob/v1/orderedmap_test.go

func TestBasicFeatures(t *testing.T) {
	n := 100
	om := NewStrMap()

	for i := 0; i < n; i++ {
		assert.Equal(t, i, om.Len())
		om.Set(fmt.Sprint(i), 2*i)
		assert.Equal(t, i+1, om.Len())
	}

	// get what we just set
	for i := 0; i < n; i++ {
		key := fmt.Sprint(i)
		value, present := om.Get(key)

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
		oldValue, present := om.Get(key)
		om.Set(key, 4*i)

		assert.Equal(t, 2*i, oldValue)
		assert.True(t, present)
	}

	// and delete pairs with odd keys

	for j := 0; j < n/2; j++ {
		i = 2*j + 1
		assert.Equal(t, om.Len(), n-j)

		key := fmt.Sprint(i)
		value, present := om.Get(key)
		om.Delete(key)
		assert.Equal(t, om.Len(), n-j-1)

		assert.Equal(t, 2*i, value)
		assert.True(t, present)

		// deleting again shouldn't change anything

		value, present = om.Get(key)
		om.Delete(key)
		assert.Equal(t, om.Len(), n-j-1)
		assert.Nil(t, value)
		assert.False(t, present)
	}

	// get the whole range

	for j := 0; j < n/2; j++ {
		i = 2 * j
		key := fmt.Sprint(i)
		value, present := om.Get(key)
		assert.Equal(t, 4*i, value)
		assert.True(t, present)
		assert.True(t, om.Has(key))

		i = 2*j + 1
		key = fmt.Sprint(i)
		value, present = om.Get(key)
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

func TestUpdatingDoesntChangePairsOrder(t *testing.T) {
	om := NewStrMap()
	om.Set("foo", "bar")
	om.Set("12", 28)
	om.Set("78", 100)
	om.Set("bar", "baz")

	oldValue, present := om.Get("78")
	om.Set("78", 102)
	assert.Equal(t, 100, oldValue)
	assert.True(t, present)

	assertOrderedPairsEqual(t, om,
		[]string{"foo", "12", "78", "bar"},
		[]interface{}{"bar", 28, 102, "baz"})
}

func TestDeletingAndReinsertingChangesPairsOrder(t *testing.T) {
	om := NewStrMap()
	om.Set("foo", "bar")
	om.Set("12", 28)
	om.Set("78", 100)
	om.Set("bar", "baz")

	// delete a pair
	oldValue, present := om.Get("78")
	om.Delete("78")
	assert.Equal(t, 100, oldValue)
	assert.True(t, present)

	// re-insert the same pair
	oldValue, present = om.Get("78")
	om.Set("78", 100)
	assert.Nil(t, oldValue)
	assert.False(t, present)

	assertOrderedPairsEqual(t, om,
		[]string{"foo", "12", "bar", "78"},
		[]interface{}{"bar", 28, "baz", 100})
}

func TestEmptyMapOperations(t *testing.T) {
	om := NewStrMap()

	oldValue, present := om.Get("foo")
	assert.Nil(t, oldValue)
	assert.False(t, present)

	oldValue, present = om.Get("bar")
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

func TestNilMapOperations(t *testing.T) {
	var om *StrMap

	oldValue, present := om.Get("foo")
	assert.Nil(t, oldValue)
	assert.False(t, present)

	om.Delete("bar")
	oldValue, present = om.Get("bar")
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

// shamelessly stolen from https://github.com/python/cpython/blob/e19a91e45fd54a56e39c2d12e6aaf4757030507f/Lib/test/test_ordered_dict.py#L55-L61
func TestShuffle(t *testing.T) {
	ranLen := 100

	for _, n := range []int{0, 10, 20, 100, 1000, 10000} {
		t.Run(fmt.Sprintf("shuffle test with %d items", n), func(t *testing.T) {
			om := NewStrMap()

			keys := make([]string, n)
			values := make([]interface{}, n)

			for i := 0; i < n; i++ {
				// we prefix with the number to ensure that we don't get any duplicates
				keys[i] = fmt.Sprintf("%d_%s", i, randomHexString(t, ranLen))
				values[i] = randomHexString(t, ranLen)

				value, present := om.Get(keys[i])
				om.Set(keys[i], values[i])
				assert.Nil(t, value)
				assert.False(t, present)
				assert.True(t, om.Has(keys[i]))
			}

			assertOrderedPairsEqual(t, om, keys, values)
		})
	}
}

func TestPacking(t *testing.T) {
	n := 10000
	om := NewStrMap()

	for i := 0; i < n; i++ {
		om.Set(fmt.Sprint(i), 2*i)
	}

	d := 0
	for i := 0; i < n; i++ {
		if mrand.Intn(5) == 1 {
			continue
		}
		d++
		om.Delete(fmt.Sprint(i))
	}
	assert.Less(t, len(om.values), 10000)
	assert.Greater(t, d, 7000)
	assert.Equal(t, n-d, om.Len())
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

func assertOrderedPairsEqual(t *testing.T, om *StrMap, expectedKeys []string, expectedValues []interface{}) {

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
