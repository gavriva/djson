package djson_test

import (
	"testing"

	"github.com/gavriva/djson"

	"github.com/stretchr/testify/assert"
)

func Test1(t *testing.T) {

	djs := make(djson.DynamicJSON)
	djs["a"] = 1
	djs.Set("a/b/c", 1)

	assert.Equal(t, 1, djs.GetInt("a/b/c", 10))
	assert.Equal(t, 1, djs.Nested("a/b", false).GetInt("c", 10))
}

func TestNumber(t *testing.T) {

	json := "{\n  \"x\": 100\n}"
	o, err := djson.Parse([]byte(json))
	assert.NoError(t, err)

	assert.Equal(t, 100, o.GetInt("x", 1))
	assert.Equal(t, json, string(o.JSON()))
}

func TestFloat(t *testing.T) {

	o, err := djson.Parse([]byte(`{"x":117.1}`))
	assert.NoError(t, err)

	assert.Equal(t, 117.1, o.GetFloat("x", 1))
	assert.Equal(t, 117.1, o.Clone().GetFloat("x", 1))
}
