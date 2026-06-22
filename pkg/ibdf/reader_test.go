package ibdf

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// Helper to write padding bytes up to next 32-byte boundary
func writeAlignPadding(buf *bytes.Buffer) {
	currentLen := int64(buf.Len())
	nextAligned := AlignUp(currentLen)
	paddingSize := nextAligned - currentLen
	if paddingSize > 0 {
		buf.Write(make([]byte, paddingSize))
	}
}

func TestReader(t *testing.T) {
	buf := new(bytes.Buffer)

	// --- 1. Prepare Header Placeholder ---
	// Magic, Version, Flags, NPositions, IndexOffset, NSamples, CheckpointInterval, Reserved
	headerPlaceholder := make([]byte, 64)
	buf.Write(headerPlaceholder)

	// Block list to build position index later
	type testBlockInfo struct {
		bpPos        uint64
		offset       int64
		isCheckpoint bool
	}
	var blocks []testBlockInfo

	// --- 2. Write Checkpoint Block 0 (Index 0, Pos 1000) ---
	writeAlignPadding(buf)
	cp0Offset := int64(buf.Len())
	blocks = append(blocks, testBlockInfo{bpPos: 1000, offset: cp0Offset, isCheckpoint: true})

	// Header: checkpoint type (1), 3 pairs, 0 dels
	binary.Write(buf, binary.LittleEndian, BlockHeader{BlockType: 1, CountA: 3, CountB: 0})

	// Align to cm array
	writeAlignPadding(buf)
	// Write cm array
	binary.Write(buf, binary.LittleEndian, []float32{1.5, 2.5, 3.5})

	// Align to p1 array
	writeAlignPadding(buf)
	// Write p1 array
	binary.Write(buf, binary.LittleEndian, []uint32{0, 0, 1})

	// Align to p2 array
	writeAlignPadding(buf)
	// Write p2 array
	binary.Write(buf, binary.LittleEndian, []uint32{1, 2, 2})

	// --- 3. Write Delta Block 1 (Index 1, Pos 1050) ---
	writeAlignPadding(buf)
	db1Offset := int64(buf.Len())
	blocks = append(blocks, testBlockInfo{bpPos: 1050, offset: db1Offset, isCheckpoint: false})

	// Header: delta type (0), 1 add, 1 del
	binary.Write(buf, binary.LittleEndian, BlockHeader{BlockType: 0, CountA: 1, CountB: 1})

	// Adds cm, p1, p2
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []float32{4.5})
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []uint32{2})
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []uint32{3})

	// Dels cm, p1, p2
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []float32{2.5}) // Remove (0, 2, 2.5)
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []uint32{0})
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []uint32{2})

	// --- 4. Write Delta Block 2 (Index 2, Pos 1100) ---
	writeAlignPadding(buf)
	db2Offset := int64(buf.Len())
	blocks = append(blocks, testBlockInfo{bpPos: 1100, offset: db2Offset, isCheckpoint: false})

	// Header: delta type (0), 2 adds, 0 dels
	binary.Write(buf, binary.LittleEndian, BlockHeader{BlockType: 0, CountA: 2, CountB: 0})

	// Adds cm, p1, p2
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []float32{5.5, 6.5})
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []uint32{1, 3})
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []uint32{4, 4})

	// Dels cm, p1, p2 (none, but advancement still happens)
	writeAlignPadding(buf) // del_cm
	writeAlignPadding(buf) // del_p1
	writeAlignPadding(buf) // del_p2

	// --- 5. Write Checkpoint Block 3 (Index 3, Pos 1200) ---
	writeAlignPadding(buf)
	cp3Offset := int64(buf.Len())
	blocks = append(blocks, testBlockInfo{bpPos: 1200, offset: cp3Offset, isCheckpoint: true})

	// Header: checkpoint type (1), 2 pairs, 0 dels
	binary.Write(buf, binary.LittleEndian, BlockHeader{BlockType: 1, CountA: 2, CountB: 0})

	// Align and write cm, p1, p2
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []float32{10.0, 11.5})
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []uint32{5, 6})
	writeAlignPadding(buf)
	binary.Write(buf, binary.LittleEndian, []uint32{7, 8})

	// --- 6. Write Position Index ---
	indexOffset := int64(buf.Len())
	for _, b := range blocks {
		dataOffsetVal := uint64(b.offset)
		if b.isCheckpoint {
			dataOffsetVal |= 0x8000000000000000
		}
		binary.Write(buf, binary.LittleEndian, IndexEntry{BpPos: b.bpPos, DataOffset: dataOffsetVal})
	}

	// --- 7. Overwrite File Header ---
	fileData := buf.Bytes()
	hdr := Header{
		Magic:              0x33444249,
		Version:            3,
		Flags:              0,
		NPositions:         uint64(len(blocks)),
		IndexOffset:        uint64(indexOffset),
		NSamples:           10,
		CheckpointInterval: 3,
	}

	headerBuf := new(bytes.Buffer)
	binary.Write(headerBuf, binary.LittleEndian, hdr)
	copy(fileData[0:64], headerBuf.Bytes())

	// --- 8. Run Tests on Reader ---
	tmpFile, err := os.CreateTemp("", "ibdf-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(fileData); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	r, err := NewReader(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	// Validate header contents
	if r.Header.NPositions != 4 {
		t.Errorf("Expected NPositions 4, got %d", r.Header.NPositions)
	}
	if r.Header.IndexOffset != uint64(indexOffset) {
		t.Errorf("Expected IndexOffset %d, got %d", indexOffset, r.Header.IndexOffset)
	}

	// Test ReconstructActiveSet at Index 0 (Checkpoint 1)
	active0, err := r.ReconstructActiveSet(0)
	if err != nil {
		t.Fatalf("ReconstructActiveSet(0) failed: %v", err)
	}
	if len(active0) != 3 {
		t.Errorf("Expected active set size 3, got %d", len(active0))
	}
	expected0 := map[IBDPair]bool{
		{CM: 1.5, P1: 0, P2: 1}: true,
		{CM: 2.5, P1: 0, P2: 2}: true,
		{CM: 3.5, P1: 1, P2: 2}: true,
	}
	for pair := range active0 {
		if !expected0[pair] {
			t.Errorf("Unexpected pair in active set 0: %+v", pair)
		}
	}

	// Test ReconstructActiveSet at Index 1 (Delta 1 applied)
	active1, err := r.ReconstructActiveSet(1)
	if err != nil {
		t.Fatalf("ReconstructActiveSet(1) failed: %v", err)
	}
	if len(active1) != 3 {
		t.Errorf("Expected active set size 3, got %d", len(active1))
	}
	expected1 := map[IBDPair]bool{
		{CM: 1.5, P1: 0, P2: 1}: true,
		{CM: 3.5, P1: 1, P2: 2}: true,
		{CM: 4.5, P1: 2, P2: 3}: true,
	}
	for pair := range active1 {
		if !expected1[pair] {
			t.Errorf("Unexpected pair in active set 1: %+v", pair)
		}
	}

	// Test ReconstructActiveSet at Index 2 (Delta 2 applied)
	active2, err := r.ReconstructActiveSet(2)
	if err != nil {
		t.Fatalf("ReconstructActiveSet(2) failed: %v", err)
	}
	if len(active2) != 5 {
		t.Errorf("Expected active set size 5, got %d", len(active2))
	}
	expected2 := map[IBDPair]bool{
		{CM: 1.5, P1: 0, P2: 1}: true,
		{CM: 3.5, P1: 1, P2: 2}: true,
		{CM: 4.5, P1: 2, P2: 3}: true,
		{CM: 5.5, P1: 1, P2: 4}: true,
		{CM: 6.5, P1: 3, P2: 4}: true,
	}
	for pair := range active2 {
		if !expected2[pair] {
			t.Errorf("Unexpected pair in active set 2: %+v", pair)
		}
	}

	// Test ReconstructActiveSet at Index 3 (Checkpoint 2 loaded directly)
	active3, err := r.ReconstructActiveSet(3)
	if err != nil {
		t.Fatalf("ReconstructActiveSet(3) failed: %v", err)
	}
	if len(active3) != 2 {
		t.Errorf("Expected active set size 2, got %d", len(active3))
	}
	expected3 := map[IBDPair]bool{
		{CM: 10.0, P1: 5, P2: 7}: true,
		{CM: 11.5, P1: 6, P2: 8}: true,
	}
	for pair := range active3 {
		if !expected3[pair] {
			t.Errorf("Unexpected pair in active set 3: %+v", pair)
		}
	}
}
