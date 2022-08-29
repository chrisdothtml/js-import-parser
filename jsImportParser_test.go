package jsImportParser

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"testing"
)

func TestGetImportsFromFile(t *testing.T) {
	result := parseFixture("main.js")

	expectedLength := 21
	actualLength := len(result)
	if actualLength != expectedLength {
		t.Errorf("Expected result length of %d, got %d.", expectedLength, actualLength)
	}

	for _, importString := range result {
		if strings.HasPrefix(importString, "EXCLUDED") {
			t.Errorf("Unexpected import was included in result: %s.", importString)
		}
	}
}

func TestGetImportsFromFileNoImports(t *testing.T) {
	result := parseFixture("no-imports.js")

	expectedLength := 0
	actualLength := len(result)
	if actualLength != expectedLength {
		t.Errorf("Expected result length of %d, got %d.", expectedLength, actualLength)
	}
}

func TestGetImportsFromFileTypes(t *testing.T) {
	result := parseFixture("types.js")

	expectedLength := 2
	actualLength := len(result)
	if actualLength != expectedLength {
		t.Errorf("Expected result length of %d, got %d.", expectedLength, actualLength)
	}

	for _, importString := range result {
		if strings.HasPrefix(importString, "EXCLUDED") {
			t.Errorf("Unexpected import was included in result: %s.", importString)
		}
	}
}

func parseFixture(fileName string) []string {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("Error getting cwd: ", err)
	}

	filePath := path.Join(cwd, "__fixtures__", fileName)
	var content []byte
	content, err = ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatal("Error reading file: ", err)
	}

	return GetImportsFromFile(string(content), filePath)
}
