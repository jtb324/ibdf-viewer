package ibdf

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Header represents the 64-byte IBDF v3 file header.
type Header struct {
	Magic              uint32   // 0x33444249 ("IBD3" LE)
	Version            uint16   // 3
	Flags              uint16   // Reserved, must be 0
	NPositions         uint64   // Total number of breakpoints
	IndexOffset        uint64   // Byte offset to position index
	NSamples           uint32   // Number of unique sample IDs
	CheckpointInterval uint32   // Breakpoints between checkpoints
	Reserved           [32]byte // Zero-filled
}

// BlockHeader represents the 12-byte header at the start of each data block.
type BlockHeader struct {
	BlockType uint32 // 0 = DELTA, 1 = CHECKPOINT
	CountA    uint32 // CHECKPOINT: n_pairs, DELTA: n_adds
	CountB    uint32 // CHECKPOINT: 0, DELTA: n_dels
}

// IndexEntry represents a 16-byte entry in the position index at the end of the file.
type IndexEntry struct {
	BpPos      uint64 // Chromosome base-pair position
	DataOffset uint64 // Offset to block (bit 63 is the checkpoint flag)
}

// IBDPair represents a single active Identity-by-Descent (IBD) segment between two samples.
type IBDPair struct {
	CM float32 // centiMorgan length of the segment
	P1 uint32  // Numeric ID of the first sample (p1 < p2 always)
	P2 uint32  // Numeric ID of the second sample
}

// BlockType constants
const (
	BlockTypeDelta      uint32 = 0
	BlockTypeCheckpoint uint32 = 1
)

// Reader provides random-access reading capabilities for IBDF files.
type Reader struct {
	r      io.ReaderAt
	Header Header
	Index  []IndexEntry
}

// We need to make sure that the r io.ReaderAt is closed
// func (r *Reader) Close() {
// 	r.r.Close()
// }

// NewReader validates the header of the IBDF file, reads the position index, and returns a new Reader.
func NewReader(filepath string) (*Reader, error) {
	// open the initial reader
	r, err := os.Open(filepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to open file %s: %v\n", filepath, err)
		os.Exit(1)
	}

	// 1. Read and parse file header (64 bytes)
	var hdr Header
	headerReader := io.NewSectionReader(r, 0, 64)
	if err := binary.Read(headerReader, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// 2. Validate header metadata
	if hdr.Magic != 0x33444249 {
		return nil, fmt.Errorf("invalid magic number: expected 0x33444249, got 0x%08X", hdr.Magic)
	}
	if hdr.Version != 3 {
		return nil, fmt.Errorf("unsupported version: expected 3, got %d", hdr.Version)
	}
	if hdr.Flags != 0 {
		return nil, fmt.Errorf("unsupported flags set in header: %d (expected 0)", hdr.Flags)
	}

	// 3. Read position index (n_positions * 16 bytes)
	indexOffset := int64(hdr.IndexOffset)
	indexBytes := int64(hdr.NPositions) * 16

	indexEntries := make([]IndexEntry, hdr.NPositions)
	indexReader := io.NewSectionReader(r, indexOffset, indexBytes)
	if err := binary.Read(indexReader, binary.LittleEndian, indexEntries); err != nil {
		return nil, fmt.Errorf("failed to read position index: %w", err)
	}

	return &Reader{
		r:      r,
		Header: hdr,
		Index:  indexEntries,
	}, nil
}

// AlignUp rounds offset up to the next 32-byte alignment boundary.
func AlignUp(offset int64) int64 {
	return (offset + 31) &^ 31
}

// IsCheckpoint reports whether the index entry offset indicates a checkpoint block.
func (e IndexEntry) IsCheckpoint() bool {
	return (e.DataOffset & 0x8000000000000000) != 0
}

// RealOffset returns the actual byte offset to the block data by masking off the checkpoint bit.
func (e IndexEntry) RealOffset() int64 {
	return int64(e.DataOffset & 0x7FFFFFFFFFFFFFFF)
}

// ReadBlockHeader reads the BlockHeader metadata at a specific index.
func (r *Reader) ReadBlockHeader(idx int) (BlockHeader, error) {
	if idx < 0 || idx >= len(r.Index) {
		return BlockHeader{}, fmt.Errorf("block index %d out of bounds (0..%d)", idx, len(r.Index)-1)
	}
	offset := r.Index[idx].RealOffset()
	var bh BlockHeader
	if err := binary.Read(io.NewSectionReader(r.r, offset, 12), binary.LittleEndian, &bh); err != nil {
		return BlockHeader{}, err
	}
	return bh, nil
}

// CheckpointBlock contains the full set of IBD pairs at a checkpoint breakpoint.
type CheckpointBlock struct {
	Header BlockHeader
	Pairs  []IBDPair
}

// ReadCheckpointBlock decodes a Checkpoint block at a given index.
func (r *Reader) ReadCheckpointBlock(idx int) (*CheckpointBlock, error) {
	if idx < 0 || idx >= len(r.Index) {
		return nil, fmt.Errorf("block index %d out of bounds", idx)
	}
	entry := r.Index[idx]
	offset := entry.RealOffset()

	var bh BlockHeader
	if err := binary.Read(io.NewSectionReader(r.r, offset, 12), binary.LittleEndian, &bh); err != nil {
		return nil, fmt.Errorf("failed to read checkpoint block header: %w", err)
	}
	if bh.BlockType != BlockTypeCheckpoint {
		return nil, fmt.Errorf("block at index %d is not a checkpoint block (type = %d)", idx, bh.BlockType)
	}

	nPairs := bh.CountA
	cmOffset := AlignUp(offset + 12)
	p1Offset := AlignUp(cmOffset + int64(nPairs)*4)
	p2Offset := AlignUp(p1Offset + int64(nPairs)*4)

	cm := make([]float32, nPairs)
	p1 := make([]uint32, nPairs)
	p2 := make([]uint32, nPairs)

	if nPairs > 0 {
		if err := binary.Read(io.NewSectionReader(r.r, cmOffset, int64(nPairs)*4), binary.LittleEndian, cm); err != nil {
			return nil, fmt.Errorf("failed to read cm array: %w", err)
		}
		if err := binary.Read(io.NewSectionReader(r.r, p1Offset, int64(nPairs)*4), binary.LittleEndian, p1); err != nil {
			return nil, fmt.Errorf("failed to read p1 array: %w", err)
		}
		if err := binary.Read(io.NewSectionReader(r.r, p2Offset, int64(nPairs)*4), binary.LittleEndian, p2); err != nil {
			return nil, fmt.Errorf("failed to read p2 array: %w", err)
		}
	}

	pairs := make([]IBDPair, nPairs)
	for i := range pairs {
		pairs[i] = IBDPair{
			CM: cm[i],
			P1: p1[i],
			P2: p2[i],
		}
	}

	return &CheckpointBlock{
		Header: bh,
		Pairs:  pairs,
	}, nil
}

// DeltaBlock contains segments added and removed at a delta breakpoint.
type DeltaBlock struct {
	Header BlockHeader
	Adds   []IBDPair
	Dels   []IBDPair
}

// ReadDeltaBlock decodes a Delta block at a given index.
func (r *Reader) ReadDeltaBlock(idx int) (*DeltaBlock, error) {
	if idx < 0 || idx >= len(r.Index) {
		return nil, fmt.Errorf("block index %d out of bounds", idx)
	}
	entry := r.Index[idx]
	offset := entry.RealOffset()

	var bh BlockHeader
	if err := binary.Read(io.NewSectionReader(r.r, offset, 12), binary.LittleEndian, &bh); err != nil {
		return nil, fmt.Errorf("failed to read delta block header: %w", err)
	}
	if bh.BlockType != BlockTypeDelta {
		return nil, fmt.Errorf("block at index %d is not a delta block (type = %d)", idx, bh.BlockType)
	}

	nAdds := bh.CountA
	nDels := bh.CountB

	addCMOffset := AlignUp(offset + 12)
	addP1Offset := AlignUp(addCMOffset + int64(nAdds)*4)
	addP2Offset := AlignUp(addP1Offset + int64(nAdds)*4)

	delCMOffset := AlignUp(addP2Offset + int64(nAdds)*4)
	delP1Offset := AlignUp(delCMOffset + int64(nDels)*4)
	delP2Offset := AlignUp(delP1Offset + int64(nDels)*4)

	addCM := make([]float32, nAdds)
	addP1 := make([]uint32, nAdds)
	addP2 := make([]uint32, nAdds)

	delCM := make([]float32, nDels)
	delP1 := make([]uint32, nDels)
	delP2 := make([]uint32, nDels)

	if nAdds > 0 {
		if err := binary.Read(io.NewSectionReader(r.r, addCMOffset, int64(nAdds)*4), binary.LittleEndian, addCM); err != nil {
			return nil, fmt.Errorf("failed to read add_cm array: %w", err)
		}
		if err := binary.Read(io.NewSectionReader(r.r, addP1Offset, int64(nAdds)*4), binary.LittleEndian, addP1); err != nil {
			return nil, fmt.Errorf("failed to read add_p1 array: %w", err)
		}
		if err := binary.Read(io.NewSectionReader(r.r, addP2Offset, int64(nAdds)*4), binary.LittleEndian, addP2); err != nil {
			return nil, fmt.Errorf("failed to read add_p2 array: %w", err)
		}
	}

	if nDels > 0 {
		if err := binary.Read(io.NewSectionReader(r.r, delCMOffset, int64(nDels)*4), binary.LittleEndian, delCM); err != nil {
			return nil, fmt.Errorf("failed to read del_cm array: %w", err)
		}
		if err := binary.Read(io.NewSectionReader(r.r, delP1Offset, int64(nDels)*4), binary.LittleEndian, delP1); err != nil {
			return nil, fmt.Errorf("failed to read del_p1 array: %w", err)
		}
		if err := binary.Read(io.NewSectionReader(r.r, delP2Offset, int64(nDels)*4), binary.LittleEndian, delP2); err != nil {
			return nil, fmt.Errorf("failed to read del_p2 array: %w", err)
		}
	}

	adds := make([]IBDPair, nAdds)
	for i := range adds {
		adds[i] = IBDPair{CM: addCM[i], P1: addP1[i], P2: addP2[i]}
	}

	dels := make([]IBDPair, nDels)
	for i := range dels {
		dels[i] = IBDPair{CM: delCM[i], P1: delP1[i], P2: delP2[i]}
	}

	return &DeltaBlock{
		Header: bh,
		Adds:   adds,
		Dels:   dels,
	}, nil
}

// ActiveSet represents a set of active IBD pairs.
type ActiveSet map[IBDPair]struct{}

// Copy returns a copy of the ActiveSet.
func (s ActiveSet) Copy() ActiveSet {
	c := make(ActiveSet, len(s))
	for k := range s {
		c[k] = struct{}{}
	}
	return c
}

// ReconstructActiveSet performs backward-replay to reconstruct the active set at target index T.
func (r *Reader) ReconstructActiveSet(T int) (ActiveSet, error) {
	if T < 0 || T >= len(r.Index) {
		return nil, fmt.Errorf("index out of bounds: %d", T)
	}

	// 1. Scan backward from T to find the nearest checkpoint block
	checkpointIdx := -1
	for i := T; i >= 0; i-- {
		if r.Index[i].IsCheckpoint() {
			checkpointIdx = i
			break
		}
	}
	if checkpointIdx == -1 {
		return nil, fmt.Errorf("no checkpoint found prior to or at index %d", T)
	}

	// 2. Load the checkpoint block
	cb, err := r.ReadCheckpointBlock(checkpointIdx)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint at index %d: %w", checkpointIdx, err)
	}

	active := make(ActiveSet, len(cb.Pairs))
	for _, pair := range cb.Pairs {
		active[pair] = struct{}{}
	}

	// 3. Play forward through deltas from checkpointIdx + 1 up to T
	for i := checkpointIdx + 1; i <= T; i++ {
		entry := r.Index[i]
		if entry.IsCheckpoint() {
			// If we hit a checkpoint anyway, replace the active set entirely
			cbNext, err := r.ReadCheckpointBlock(i)
			if err != nil {
				return nil, fmt.Errorf("failed to load checkpoint block at index %d: %w", i, err)
			}
			active = make(ActiveSet, len(cbNext.Pairs))
			for _, pair := range cbNext.Pairs {
				active[pair] = struct{}{}
			}
		} else {
			// Apply delta additions and deletions
			db, err := r.ReadDeltaBlock(i)
			if err != nil {
				return nil, fmt.Errorf("failed to load delta block at index %d: %w", i, err)
			}
			for _, del := range db.Dels {
				delete(active, del)
			}
			for _, add := range db.Adds {
				active[add] = struct{}{}
			}
		}
	}

	return active, nil
}
