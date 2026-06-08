package ibdf

import (
	"bufio"
	"os"
	"strings"
)

// ReadSamplesFile reads a plain-text companion file containing sample names.
// Each line corresponds to a sample ID starting from 0.
func ReadSamplesFile(path string) ([]string, error) {
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
