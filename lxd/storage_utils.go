package main

import (
	"fmt"
	"regexp"
)

func storageValidName(value string) error {
	// Validate the character set
	match, _ := regexp.MatchString("^[-a-zA-Z0-9]*$", value)
	if !match {
		return fmt.Errorf("Interface name contains invalid characters")
	}

	return nil
}
