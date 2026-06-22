package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jtb324/ibdf-viewer/pkg/ibdf"
)

type viewMode int

const (
	modeActivePairs viewMode = iota
	modeDeltas
	modeHelp
)

type deltaFilter int

const (
	filterBoth deltaFilter = iota
	filterAddsOnly
	filterDelsOnly
)

// Styles for the TUI
var (
	// Colors
	purple    = lipgloss.Color("#7D56F4")
	darkGray  = lipgloss.Color("#242424")
	lightGray = lipgloss.Color("#D3D3D3")
	dimGray   = lipgloss.Color("#707070")
	green     = lipgloss.Color("#00FF66")
	red       = lipgloss.Color("#FF3366")
	yellow    = lipgloss.Color("#FFCC00")

	// Component Styles
	titleStyle = lipgloss.NewStyle().
			Background(purple).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Background(darkGray).
			Foreground(lightGray).
			Padding(0, 1)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(dimGray)

	columnHeaderStyle = lipgloss.NewStyle().
				Foreground(purple).
				Bold(true)

	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#3C3C3C")).
				Foreground(lipgloss.Color("#FFFFFF"))

	dimStyle = lipgloss.NewStyle().
			Foreground(dimGray)

	addStyle = lipgloss.NewStyle().
			Foreground(green)

	delStyle = lipgloss.NewStyle().
			Foreground(red)

	statusStyle = lipgloss.NewStyle().
			Background(darkGray).
			Foreground(yellow)
)

// Model represents the main UI state
type Model struct {
	reader   *ibdf.Reader
	filePath string
	samples  []string

	// Navigation state
	currIndex     int
	totalIndices  int
	currActiveSet ibdf.ActiveSet
	activePairs   []ibdf.IBDPair   // sorted list of active pairs
	deltaBlock    *ibdf.DeltaBlock // loaded if current is delta

	// Scroll and selection
	cursorIndex  int // index of highlighted row in the list
	scrollOffset int // how many rows scrolled down
	windowWidth  int
	windowHeight int

	// Modes
	mode        viewMode
	deltaFilter deltaFilter

	// Search/Filter state
	searchActive   bool
	searchBuffer   string
	searchFilter   string // active filter text
	compiledFilter ibdf.Filter
	searchError    string

	err error
}

// NewModel initializes the Bubble Tea model
func NewModel(filePath string, reader *ibdf.Reader, samples []string) (*Model, error) {
	m := &Model{
		reader:       reader,
		filePath:     filePath,
		samples:      samples,
		totalIndices: len(reader.Index),
		mode:         modeActivePairs,
	}

	// Load first breakpoint (always a checkpoint)
	if err := m.setIndex(0); err != nil {
		return nil, err
	}

	return m, nil
}

// Init initializes the Bubble Tea loop
func (m *Model) Init() tea.Cmd {
	return nil
}

// sampleName maps ID to string
func (m *Model) sampleName(id uint32) string {
	if int(id) < len(m.samples) {
		return m.samples[id]
	}
	return fmt.Sprintf("Sample_%d", id)
}

// sortedPairs extracts and sorts active pairs from active set
func getSortedPairs(active ibdf.ActiveSet) []ibdf.IBDPair {
	pairs := make([]ibdf.IBDPair, 0, len(active))
	for p := range active {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].P1 != pairs[j].P1 {
			return pairs[i].P1 < pairs[j].P1
		}
		if pairs[i].P2 != pairs[j].P2 {
			return pairs[i].P2 < pairs[j].P2
		}
		return pairs[i].CM < pairs[j].CM
	})
	return pairs
}

// setIndex updates the current position, using optimizations for step navigation
func (m *Model) setIndex(newIdx int) error {
	if newIdx < 0 || newIdx >= m.totalIndices {
		return nil
	}

	var active ibdf.ActiveSet
	var err error

	// Apply delta forward/backward optimization
	if newIdx == m.currIndex+1 && m.currActiveSet != nil {
		entry := m.reader.Index[newIdx]
		if entry.IsCheckpoint() {
			active, err = m.reader.ReconstructActiveSet(newIdx)
		} else {
			delta, errRead := m.reader.ReadDeltaBlock(newIdx)
			if errRead != nil {
				return errRead
			}
			// Clone and update in place
			active = m.currActiveSet.Copy()
			for _, del := range delta.Dels {
				delete(active, del)
			}
			for _, add := range delta.Adds {
				active[add] = struct{}{}
			}
		}
	} else if newIdx == m.currIndex-1 && m.currActiveSet != nil {
		currEntry := m.reader.Index[m.currIndex]
		if currEntry.IsCheckpoint() {
			// We cannot reverse from a checkpoint block without re-evaluating
			active, err = m.reader.ReconstructActiveSet(newIdx)
		} else {
			delta, errRead := m.reader.ReadDeltaBlock(m.currIndex)
			if errRead != nil {
				return errRead
			}
			active = m.currActiveSet.Copy()
			// Reverse delta operations
			for _, del := range delta.Dels {
				active[del] = struct{}{}
			}
			for _, add := range delta.Adds {
				delete(active, add)
			}
		}
	} else {
		// Full reconstruct
		active, err = m.reader.ReconstructActiveSet(newIdx)
	}

	if err != nil {
		return err
	}

	m.currActiveSet = active
	m.activePairs = getSortedPairs(active)
	m.currIndex = newIdx
	m.cursorIndex = 0
	m.scrollOffset = 0

	// Load delta details if applicable
	if !m.reader.Index[newIdx].IsCheckpoint() {
		db, errRead := m.reader.ReadDeltaBlock(newIdx)
		if errRead == nil {
			m.deltaBlock = db
		}
	} else {
		m.deltaBlock = nil
	}

	return nil
}

type deltaItem struct {
	IsAdd bool
	Pair  ibdf.IBDPair
}

func (m *Model) getFilteredDeltas() []deltaItem {
	if m.deltaBlock == nil {
		return nil
	}

	// Gather all items according to m.deltaFilter
	allItems := make([]deltaItem, 0)
	if m.deltaFilter == filterBoth || m.deltaFilter == filterAddsOnly {
		for _, p := range m.deltaBlock.Adds {
			allItems = append(allItems, deltaItem{IsAdd: true, Pair: p})
		}
	}
	if m.deltaFilter == filterBoth || m.deltaFilter == filterDelsOnly {
		for _, p := range m.deltaBlock.Dels {
			allItems = append(allItems, deltaItem{IsAdd: false, Pair: p})
		}
	}

	if m.searchFilter == "" {
		return allItems
	}

	filtered := make([]deltaItem, 0)
	if m.compiledFilter != nil {
		for i, item := range allItems {
			if m.compiledFilter.Match(item.Pair, i+1, m.samples) {
				filtered = append(filtered, item)
			}
		}
		return filtered
	}

	filterLower := strings.ToLower(m.searchFilter)
	for _, item := range allItems {
		s1 := strings.ToLower(m.sampleName(item.Pair.P1))
		s2 := strings.ToLower(m.sampleName(item.Pair.P2))
		if strings.Contains(s1, filterLower) || strings.Contains(s2, filterLower) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// getFilteredPairs returns pairs matching search filter
func (m *Model) getFilteredPairs() []ibdf.IBDPair {
	if m.searchFilter == "" {
		return m.activePairs
	}

	if m.compiledFilter != nil {
		filtered := make([]ibdf.IBDPair, 0)
		for i, p := range m.activePairs {
			if m.compiledFilter.Match(p, i+1, m.samples) {
				filtered = append(filtered, p)
			}
		}
		return filtered
	}

	filterLower := strings.ToLower(m.searchFilter)
	filtered := make([]ibdf.IBDPair, 0)
	for _, p := range m.activePairs {
		s1 := strings.ToLower(m.sampleName(p.P1))
		s2 := strings.ToLower(m.sampleName(p.P2))
		if strings.Contains(s1, filterLower) || strings.Contains(s2, filterLower) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// Update handles UI signals and keypresses
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Search mode input intercept
		if m.searchActive {
			switch msg.String() {
			case "enter":
				m.searchActive = false
				m.executeSearch()
				return m, nil
			case "esc":
				m.searchActive = false
				m.searchBuffer = ""
				return m, nil
			case "backspace":
				if len(m.searchBuffer) > 0 {
					m.searchBuffer = m.searchBuffer[:len(m.searchBuffer)-1]
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.searchBuffer += msg.String()
				}
				return m, nil
			}
		}

		// Normal key bindings
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "right", "l":
			if err := m.setIndex(m.currIndex + 1); err != nil {
				m.err = err
			}

		case "left", "h":
			if err := m.setIndex(m.currIndex - 1); err != nil {
				m.err = err
			}

		case "]":
			// Jump to next checkpoint
			for i := m.currIndex + 1; i < m.totalIndices; i++ {
				if m.reader.Index[i].IsCheckpoint() {
					if err := m.setIndex(i); err != nil {
						m.err = err
					}
					break
				}
			}

		case "[":
			// Jump to previous checkpoint
			for i := m.currIndex - 1; i >= 0; i-- {
				if m.reader.Index[i].IsCheckpoint() {
					if err := m.setIndex(i); err != nil {
						m.err = err
					}
					break
				}
			}

		case "down", "j":
			m.scrollDown(1)

		case "up", "k":
			m.scrollUp(1)

		case "t":
			if m.mode == modeDeltas {
				m.deltaFilter = (m.deltaFilter + 1) % 3
				m.cursorIndex = 0
				m.scrollOffset = 0
			}

		case "pgdown", " ", "d": // wait, 'd' is toggle delta view. Let's make Space / PageDown scroll by page
			if msg.String() == "d" {
				if m.mode == modeActivePairs {
					m.mode = modeDeltas
				} else {
					m.mode = modeActivePairs
				}
				m.cursorIndex = 0
				m.scrollOffset = 0
			} else {
				m.scrollDown(m.pageHeight())
			}

		case "pgup", "b":
			m.scrollUp(m.pageHeight())

		case "/":
			m.searchActive = true
			m.searchBuffer = ""
			m.searchError = ""

		case "esc":
			// Clear filter
			m.searchFilter = ""
			m.compiledFilter = nil
			m.searchError = ""

		case "?":
			if m.mode == modeHelp {
				m.mode = modeActivePairs
			} else {
				m.mode = modeHelp
			}
		}

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
	}

	return m, nil
}

// pageHeight returns the height of the data display window
func (m *Model) pageHeight() int {
	// 3 lines header, 4 lines metadata/table header, 2 lines status bar
	h := m.windowHeight - 9
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) scrollDown(amount int) {
	listLen := m.getListLength()
	if listLen == 0 {
		return
	}
	m.cursorIndex += amount
	if m.cursorIndex >= listLen {
		m.cursorIndex = listLen - 1
	}

	ph := m.pageHeight()
	if m.cursorIndex >= m.scrollOffset+ph {
		m.scrollOffset = m.cursorIndex - ph + 1
	}
}

func (m *Model) scrollUp(amount int) {
	m.cursorIndex -= amount
	if m.cursorIndex < 0 {
		m.cursorIndex = 0
	}

	if m.cursorIndex < m.scrollOffset {
		m.scrollOffset = m.cursorIndex
	}
}

func (m *Model) getListLength() int {
	if m.mode == modeDeltas {
		return len(m.getFilteredDeltas())
	}
	return len(m.getFilteredPairs())
}

// executeSearch parses input and updates position or search query
func (m *Model) executeSearch() {
	query := strings.TrimSpace(m.searchBuffer)
	if query == "" {
		m.searchFilter = ""
		m.compiledFilter = nil
		m.searchError = ""
		return
	}

	// Try to parse as base-pair position
	cleanedQuery := strings.ReplaceAll(query, ",", "")
	if bpPos, err := strconv.ParseUint(cleanedQuery, 10, 64); err == nil {
		// Binary search index for closest bp position
		targetIdx := m.binarySearchBpPos(bpPos)
		if err := m.setIndex(targetIdx); err != nil {
			m.searchError = fmt.Sprintf("Error jumping to position: %v", err)
		}
		return
	}

	// Try parsing as SQL expression first
	filter, err := ibdf.ParseFilter(query)
	if err == nil {
		m.searchFilter = query
		m.compiledFilter = filter
		m.searchError = ""
		return
	}

	// If there's an error, check if they intended to write a query, if so display the syntax/validation error
	lowerQuery := strings.ToLower(query)
	hasSQLSig := strings.ContainsAny(query, "=><!") ||
		strings.Contains(lowerQuery, "like") ||
		strings.Contains(lowerQuery, "and") ||
		strings.Contains(lowerQuery, "or") ||
		strings.Contains(lowerQuery, "not")

	if hasSQLSig {
		m.searchError = err.Error()
		return
	}

	// Otherwise, fallback to basic substring match
	m.searchFilter = query
	m.compiledFilter = nil
	m.searchError = ""
}

func (m *Model) binarySearchBpPos(pos uint64) int {
	idx := sort.Search(len(m.reader.Index), func(i int) bool {
		return m.reader.Index[i].BpPos >= pos
	})
	if idx >= len(m.reader.Index) {
		return len(m.reader.Index) - 1
	}
	if idx > 0 {
		// Choose closest between idx and idx-1
		diffCurrent := m.reader.Index[idx].BpPos - pos
		diffPrev := pos - m.reader.Index[idx-1].BpPos
		if diffPrev < diffCurrent {
			return idx - 1
		}
	}
	return idx
}

// View renders the terminal output
func (m *Model) View() string {
	if m.windowHeight == 0 || m.windowWidth == 0 {
		return "Initializing viewer..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\nPress Ctrl+C or Q to exit.", m.err)
	}

	var s strings.Builder

	// 1. Header Bar
	headerTitle := fmt.Sprintf(" IBDF Viewer: %s ", filepath.Base(m.filePath))
	headerBar := titleStyle.Render(headerTitle)
	spacer := strings.Repeat(" ", m.max(0, m.windowWidth-lipgloss.Width(headerBar)-20))
	helpHint := dimStyle.Render("[?: Help] [q: Quit] ")
	s.WriteString(borderStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, headerBar, spacer, helpHint)) + "\n")

	// 2. Metadata Panel
	entry := m.reader.Index[m.currIndex]
	blockTypeStr := "DELTA"
	if entry.IsCheckpoint() {
		blockTypeStr = "CHECKPOINT"
	}

	activeCount := len(m.activePairs)
	metaLine1 := fmt.Sprintf("Breakpoint: %d / %d | Position: %s bp | Block Type: %s",
		m.currIndex+1, m.totalIndices, m.formatNumber(entry.BpPos), blockTypeStr)

	var metaLine2 string
	if entry.IsCheckpoint() {
		metaLine2 = fmt.Sprintf("Active Pairs: %d", activeCount)
	} else {
		addsCount := 0
		delsCount := 0
		if m.deltaBlock != nil {
			addsCount = len(m.deltaBlock.Adds)
			delsCount = len(m.deltaBlock.Dels)
		}
		metaLine2 = fmt.Sprintf("Active Pairs: %d | Changes: +%d adds, -%d dels", activeCount, addsCount, delsCount)
	}
	if m.searchFilter != "" {
		metaLine2 += fmt.Sprintf(" (Filter: \"%s\")", m.searchFilter)
	}
	if m.mode == modeDeltas {
		switch m.deltaFilter {
		case filterAddsOnly:
			metaLine2 += " (Type: ADDs only)"
		case filterDelsOnly:
			metaLine2 += " (Type: DELs only)"
		}
	}

	s.WriteString(headerStyle.Render(metaLine1) + "\n")
	s.WriteString(headerStyle.Render(metaLine2) + "\n\n")

	// 3. Main Body
	ph := m.pageHeight()
	switch m.mode {
	case modeHelp:
		s.WriteString(m.renderHelpView(ph))
	case modeDeltas:
		s.WriteString(m.renderDeltaView(ph))
	default:
		s.WriteString(m.renderActivePairsView(ph))
	}

	// 4. Status and Command Bar
	s.WriteString("\n" + m.renderStatusBar())

	return s.String()
}

func (m *Model) renderActivePairsView(height int) string {
	pairs := m.getFilteredPairs()
	if len(pairs) == 0 {
		return "  No active pairs at this position (or matches for filter).\n" + strings.Repeat("\n", height-1)
	}

	var s strings.Builder
	col1Width := 8
	col2Width := m.max(15, (m.windowWidth-24)/2)
	col3Width := m.max(15, (m.windowWidth-24)/2)
	col4Width := 10

	// Column Headers
	headerRow := fmt.Sprintf("  %-*s %-*s %-*s %*s",
		col1Width, columnHeaderStyle.Render("Row"),
		col2Width, columnHeaderStyle.Render("Sample 1"),
		col3Width, columnHeaderStyle.Render("Sample 2"),
		col4Width, columnHeaderStyle.Render("Length(cM)"))
	s.WriteString(headerRow + "\n")

	// Adjust boundary for viewport
	endIdx := m.scrollOffset + height
	if endIdx > len(pairs) {
		endIdx = len(pairs)
	}

	for i := m.scrollOffset; i < endIdx; i++ {
		p := pairs[i]
		s1 := m.sampleName(p.P1)
		s2 := m.sampleName(p.P2)
		cmStr := fmt.Sprintf("%.4f", p.CM)
		rowStr := fmt.Sprintf("  %-*d %-*s %-*s %*s",
			col1Width, i+1,
			col2Width, s1,
			col3Width, s2,
			col4Width, cmStr)

		if i == m.cursorIndex {
			s.WriteString(selectedRowStyle.Render(rowStr) + "\n")
		} else {
			s.WriteString(rowStr + "\n")
		}
	}

	// Pad empty lines if list is shorter than viewport height
	for i := endIdx - m.scrollOffset; i < height; i++ {
		s.WriteString("\n")
	}

	return s.String()
}

func (m *Model) renderDeltaView(height int) string {
	if m.deltaBlock == nil {
		return "  Checkpoint block contains full active set. No delta changes here.\n" + strings.Repeat("\n", height-1)
	}

	filteredDeltas := m.getFilteredDeltas()
	if len(filteredDeltas) == 0 {
		return "  No delta changes match the current filter.\n" + strings.Repeat("\n", height-1)
	}

	var s strings.Builder
	col1Width := 8
	col2Width := 6
	col3Width := m.max(15, (m.windowWidth-30)/2)
	col4Width := m.max(15, (m.windowWidth-30)/2)
	col5Width := 10

	// Column Headers
	headerRow := fmt.Sprintf("  %-*s %-*s %-*s %-*s %*s",
		col1Width, columnHeaderStyle.Render("Row"),
		col2Width, columnHeaderStyle.Render("Type"),
		col3Width, columnHeaderStyle.Render("Sample 1"),
		col4Width, columnHeaderStyle.Render("Sample 2"),
		col5Width, columnHeaderStyle.Render("Length(cM)"))
	s.WriteString(headerRow + "\n")

	// Adjust boundary for viewport
	endIdx := m.scrollOffset + height
	if endIdx > len(filteredDeltas) {
		endIdx = len(filteredDeltas)
	}

	for i := m.scrollOffset; i < endIdx; i++ {
		item := filteredDeltas[i]
		isAdd := item.IsAdd
		p := item.Pair

		s1 := m.sampleName(p.P1)
		s2 := m.sampleName(p.P2)
		cmStr := fmt.Sprintf("%.4f", p.CM)

		typeStr := "- DEL"
		typeStyle := delStyle
		if isAdd {
			typeStr = "+ ADD"
			typeStyle = addStyle
		}

		rowStr := fmt.Sprintf("  %-*d %-*s %-*s %-*s %*s",
			col1Width, i+1,
			col2Width, typeStr,
			col3Width, s1,
			col4Width, s2,
			col5Width, cmStr)

		if i == m.cursorIndex {
			s.WriteString(selectedRowStyle.Render(rowStr) + "\n")
		} else {
			// Apply text colors to types
			typeRendered := typeStyle.Render(typeStr)
			styledRowStr := fmt.Sprintf("  %-*d %-*s %-*s %-*s %*s",
				col1Width, i+1,
				col2Width, typeRendered,
				col3Width, s1,
				col4Width, s2,
				col5Width, cmStr)
			s.WriteString(styledRowStr + "\n")
		}
	}

	// Pad empty lines
	for i := endIdx - m.scrollOffset; i < height; i++ {
		s.WriteString("\n")
	}

	return s.String()
}

func (m *Model) renderHelpView(height int) string {
	helpLines := []string{
		"  KEYBOARD NAVIGATION:",
		"  ─────────────────────────────────────────────────────────────",
		"  Right Arrow / l   - Move to next breakpoint position",
		"  Left Arrow / h    - Move to previous breakpoint position",
		"  Down Arrow / j    - Scroll list of pairs down",
		"  Up Arrow / k      - Scroll list of pairs up",
		"  PageDown / Space  - Page list down",
		"  PageUp / b        - Page list up",
		"  [                 - Jump backward to nearest checkpoint",
		"  ]                 - Jump forward to next checkpoint",
		"  d                 - Toggle view: Active Pairs vs Delta Details",
		"  t                 - Cycle Delta Type filter: BOTH -> ADD -> DEL (Delta view only)",
		"  /                 - Open search prompt",
		"                      (Enter bp position to jump, or text to filter pairs)",
		"  Esc               - Clear active text filter",
		"  ?                 - Toggle help menu",
		"  q / Ctrl+C        - Quit viewer",
	}

	var s strings.Builder
	for i := 0; i < height; i++ {
		if i < len(helpLines) {
			s.WriteString(helpLines[i] + "\n")
		} else {
			s.WriteString("\n")
		}
	}
	return s.String()
}

func (m *Model) renderStatusBar() string {
	if m.searchActive {
		return statusStyle.Render(fmt.Sprintf(" Search (number to jump, text to filter): /%s█", m.searchBuffer))
	}

	var s string
	if m.searchError != "" {
		s = lipgloss.NewStyle().Background(red).Foreground(lipgloss.Color("#FFFFFF")).Render(" " + m.searchError + " ")
	} else {
		// Navigation tips
		tip := "[Left/Right] Bp  [Up/Down] Scroll  [/] Search  [d] Toggle Delta  [?]: Help"
		if m.mode == modeDeltas {
			tip = "[Left/Right] Bp  [Up/Down] Scroll  [/] Search  [d] Toggle Delta  [t] Cycle Type  [?]: Help"
		}
		s = statusStyle.Render(" " + tip + " ")
	}
	return s
}

func (m *Model) max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *Model) formatNumber(n uint64) string {
	s := strconv.FormatUint(n, 10)
	if len(s) <= 3 {
		return s
	}
	var res []string
	for len(s) > 3 {
		res = append(res, s[len(s)-3:])
		s = s[:len(s)-3]
	}
	res = append(res, s)
	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}
	return strings.Join(res, ",")
}

// SetIndex updates the current position from an external source (like main.go)
func (m *Model) SetIndex(newIdx int) error {
	return m.setIndex(newIdx)
}
