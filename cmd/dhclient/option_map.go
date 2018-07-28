package main

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type optionMap map[uint8]string

func (m optionMap) Set(value string) error {
	i := strings.Index(value, ",")
	if i < 0 {
		return errors.New("invalid \"code,value\" pair")
	}

	code, err := strconv.Atoi(value[:i])
	if err != nil {
		return fmt.Errorf("option code \"%s\" is invalid", value[:i])
	}

	m[uint8(code)] = value[i+1:]

	return nil
}

func (m optionMap) String() string {
	b := new(bytes.Buffer)
	for key, value := range m {
		if b.Len() != 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(b, "%d,%s", key, value)
	}
	return b.String()
}
