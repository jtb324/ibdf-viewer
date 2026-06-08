package ibdf

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadSamplesFile reads a plain-text companion file containing sample names.
// Each line corresponds to a sample ID starting from 0.
func ReadSamplesFile(path string) ([]string, error) {
	if path == "" {
		return nil, fmt.Errorf("no file was provided indicating what samples are in the binary ibdf. This file should have been created when the ibdf file was made. Please either provide this file using the --samples flag or make sure a file with the suffix .samples is in the same directory as the ibdf file")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var samples []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Strip carriage returns if present on Windows-formatted files
		line = strings.TrimSuffix(line, "\r")
		samples = append(samples, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}
