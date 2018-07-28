package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptionMap(t *testing.T) {
	assert := assert.New(t)

	m := make(optionMap)

	assert.EqualError(m.Set("foo"), "invalid \"code,value\" pair")
	assert.EqualError(m.Set(","), "option code \"\" is invalid")
	assert.EqualError(m.Set("0x12,foo"), "option code \"0x12\" is invalid")

	assert.NoError(m.Set("65,foo"))
	assert.Equal("65,foo", m.String())
}
