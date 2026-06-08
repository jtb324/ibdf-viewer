package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jtb324/ibdf-viewer/pkg/ibdf"
	"github.com/jtb324/ibdf-viewer/pkg/tui"
)

func main() {
	// 1. Define command line flags
	samplesFlag := flag.String("samples", "", "Path to the companion .samples file (defaults to auto-detecting)")
	samplesShort := flag.String("s", "", "Path to the companion .samples file (shorthand)")

	posFlag := flag.Uint64("position", 0, "Genomic base-pair position to start at")
	posShort := flag.Uint64("p", 0, "Genomic base-pair position to start at (shorthand)")

	idxFlag := flag.Int("index", -1, "0-indexed breakpoint position to start at")
	idxShort := flag.Int("i", -1, "0-indexed breakpoint position to start at (shorthand)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <file.ibdf>\n\nFlags:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	// Resolve shorthands
	samplesPath := *samplesFlag
	if samplesPath == "" {
		samplesPath = *samplesShort
	}
	startPos := *posFlag
	if startPos == 0 {
		startPos = *posShort
	}
	startIdx := *idxFlag
	if startIdx == -1 {
		startIdx = *idxShort
	}

	// 2. Validate positional arguments
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: Missing required argument <file.ibdf>")
		flag.Usage()
		os.Exit(1)
	}
	ibdfPath := args[0]

	// 3. Open the IBDF file
	file, err := os.Open(ibdfPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to open file %s: %v\n", ibdfPath, err)
		os.Exit(1)
	}
	defer file.Close()

	// 4. Create the IBDF reader
	reader, err := ibdf.NewReader(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to parse IBDF headers: %v\n", err)
		os.Exit(1)
	}

	// 5. Auto-detect or load the samples companion file
	var samples []string
	if samplesPath == "" {
		samplesPath = detectSamplesFile(ibdfPath)
	}

	if samplesPath != "" {
		var errSamples error
		samples, errSamples = ibdf.ReadSamplesFile(samplesPath)
		if errSamples != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to load samples file at %s: %v\nContinuing without sample names.\n", samplesPath, errSamples)
		} else {
			// Validate sample count if it doesn't match header
			if uint32(len(samples)) < reader.Header.NSamples {
				fmt.Fprintf(os.Stderr, "Warning: Samples file contains %d samples, but IBDF header indicates %d unique samples.\n", len(samples), reader.Header.NSamples)
			}
		}
	}

	// 6. Build the UI Model
	model, err := tui.NewModel(ibdfPath, reader, samples)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize UI: %v\n", err)
		os.Exit(1)
	}

	// 7. Apply start position or index flags
	if startPos > 0 {
		// Jump to closest base-pair position
		bestIdx := 0
		minDiff := uint64(math.MaxUint64)
		for i, entry := range reader.Index {
			diff := uint64(0)
			if entry.BpPos > startPos {
				diff = entry.BpPos - startPos
			} else {
				diff = startPos - entry.BpPos
			}
			if diff < minDiff {
				minDiff = diff
				bestIdx = i
			}
		}
		if err := model.SetIndex(bestIdx); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting starting position: %v\n", err)
			os.Exit(1)
		}
	} else if startIdx >= 0 {
		if startIdx >= len(reader.Index) {
			fmt.Fprintf(os.Stderr, "Error: Starting index %d is out of bounds (max %d)\n", startIdx, len(reader.Index)-1)
			os.Exit(1)
		}
		if err := model.SetIndex(startIdx); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting starting index: %v\n", err)
			os.Exit(1)
		}
	}

	// 8. Run Bubble Tea alternate screen app
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Terminal viewer failed: %v\n", err)
		os.Exit(1)
	}
}

// detectSamplesFile attempts to locate a companion samples file
func detectSamplesFile(ibdfPath string) string {
	ext := filepath.Ext(ibdfPath)
	baseWithoutExt := ibdfPath[:len(ibdfPath)-len(ext)]

	// Check 1: <base>.samples
	path1 := baseWithoutExt + ".samples"
	if fileExists(path1) {
		return path1
	}

	// Check 2: <base><ext>.samples (e.g. data.ibdf.samples)
	path2 := ibdfPath + ".samples"
	if fileExists(path2) {
		return path2
	}

	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
