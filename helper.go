package main

import (
	"errors"
	"strings"
)

func simplifyError(err error) error {
	str := err.Error()
	index := strings.LastIndex(str, ": ")

	if index != -1 {
		str = str[index+2 : len(str)]
	}

	return errors.New(str)
}
