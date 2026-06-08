package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jtb324/ibdf-viewer/pkg/ibdf"
	"github.com/jtb324/ibdf-viewer/pkg/tui"
	"github.com/urfave/cli/v3"
)

// Make a sentinel error to represent the samples file not being found
var SamplesFileNotFound = errors.New("Samples file not found")

// detectSamplesFile attempts to locate a companion samples file
func detectSamplesFile(ibdfPath string) (string, error) {
	ext := filepath.Ext(ibdfPath)
	baseWithoutExt := ibdfPath[:len(ibdfPath)-len(ext)]

	// Check 1: <base>.samples
	path1 := baseWithoutExt + ".samples"
	if fileExists(path1) {
		return path1, nil
	}

	// Check 2: <base><ext>.samples (e.g. data.ibdf.samples)
	path2 := ibdfPath + ".samples"
	if fileExists(path2) {
		return path2, nil
	}

	return "", SamplesFileNotFound
}

func main() {
	var ibdfFile string
	cmd := &cli.Command{
		Usage: "TUI to view the ibdf binary files",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "samples",
				Aliases: []string{"s"},
				Usage:   "Path to the samples file that is generated when the ibdf files are formed",
			},
			&cli.Uint64Flag{
				Name:    "position",
				Aliases: []string{"p"},
				Usage:   "Genomic base-pair position to start at",
			},
			&cli.IntFlag{
				Name:    "index",
				Aliases: []string{"i"},
				Value:   -1,
				Usage:   "0-indexeed breakpoint position to start at. ",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "file",
				Destination: &ibdfFile,
				UsageText:   "IBDF binary file containing pairwise ibd segments used for IDBMap.",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			samplesPath := cmd.String("samples")
			startPos := cmd.Uint64("position")
			startIdx := cmd.Int("index")

			reader, err := ibdf.NewReader(ibdfFile)
			if err != nil {
				return fmt.Errorf("failed to parse IBDF headers: %w", err)
			}

			// array to store all of the sample ids in
			// We need to find the samples file if the user doesn't provide one.
			// This approach is assuming that the file is in the same directory
			// as the ibdf file
			if samplesPath == "" {
				samplesPath, err = detectSamplesFile(ibdfFile)

				if errors.Is(err, SamplesFileNotFound) {
					log.Fatal("Failed to detect a .samples file. Make sure that there is a file iwht the suffix .samples either in the same directory as the ibdf file or provide the path to a samples file using the --samples flag")
					os.Exit(1)
				}
			}

			samples, errSamples := ibdf.ReadSamplesFile(samplesPath)
			if errSamples != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to load samples file at %s: %v\nContinuing without sample names.\n", samplesPath, errSamples)
			}

			// Validate sample count if it doesn't match header
			if uint32(len(samples)) < reader.Header.NSamples {
				fmt.Fprintf(os.Stderr, "Warning: Samples file contains %d samples, but IBDF header indicates %d unique samples.\n", len(samples), reader.Header.NSamples)
			}

			// 6. Build the UI Model
			model, err := tui.NewModel(ibdfFile, reader, samples)
			if err != nil {
				return fmt.Errorf("failed to initialize UI: %w", err)
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
					return fmt.Errorf("error setting starting position: %w", err)
				}
			} else if startIdx >= 0 {
				if startIdx >= len(reader.Index) {
					return fmt.Errorf("starting index %d is out of bounds (max %d)", startIdx, len(reader.Index)-1)
				}
				if err := model.SetIndex(startIdx); err != nil {
					return fmt.Errorf("error setting starting index: %w", err)
				}
			}

			// 8. Run Bubble Tea alternate screen app
			p := tea.NewProgram(model, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("terminal viewer failed: %w", err)
			}
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
