# IBDF Viewer

An interactive terminal pager (TUI) for inspecting **Identity-by-Descent Binary Format (IBDF v3)** files. This viewer allows you to easily navigate each genomic breakpoint and inspect pairs sharing IBD pairs, and review individuals who being sharing at the breakpoint or those who stop sharing at the breakpoint (we refer to these as additions/deletions).

---

## Features

- **High-Performance Navigation**: Leverages checkpointed-deltas within the IBDF v3 file structure for fast forward and reverse scrolling.
- **Dual View Modes**:
  - **Active Pairs View**: View the complete set of Identity-by-Descent (IBD) segment relationships active at the current breakpoint.
  - **Delta Details View**: View the specific additions (`+ ADD`) and deletions (`- DEL`) occurring at the current breakpoint.
- **Companion Samples Resolution**: Automatically maps binary numeric sample IDs to human-readable names using companion `.samples` files.
- **Genomic Position Jump**: Quickly jump to the nearest checkpoint or query a specific base-pair position using the search interface.
- **Instant Filtering**: Filter active sets dynamically by sample names.

---

## Getting Started

### Installation

We recommend just download the binary for the correct architecture from GitHub

### Building the Binary

**Prerequisites**

- [Go](https://go.dev/) 1.21 or later.

To compile the viewer from source:

```bash
# Clone the directory
git clone https://github.com/jtb324/ibdf-viewer.git

# move into the directory
cd ibdf-viewer

# Build the executable
go build -o ibdf-viewer main.go
```

This produces an `ibdf-viewer` executable in your current directory.

---

## Usage

Run the viewer by passing an IBDF binary file as an argument:

```bash
./ibdf-viewer [flags] <file>
```

### Arguments

- `<file>`: The path to the IBDF binary file (containing pairwise IBD segments).

### Flags

- `-s`, `--samples <path>`  
  Path to the companion `.samples` file (plain text, one sample name per line). If omitted, the viewer automatically attempts to locate it in the same directory (see [Sample Name Mapping](#sample-name-mapping) below).
- `-p`, `--position <bp>`  
  Genomic base-pair position (bp) to jump to on startup. The viewer will select the breakpoint index closest to this position.
- `-i`, `--index <idx>`  
  0-indexed breakpoint index to start at (takes precedence over `-p` if both are supplied).

---

## User Interface Overview

The interface is divided into four main visual zones:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  IBDF Viewer: chr15.ibdf                           [Help: ?] [Quit: q]      │ <-- 1. Header Bar
├─────────────────────────────────────────────────────────────────────────────┤
│  Breakpoint: 45 / 1,024   | Position: 14,203,500 bp   | Block Type: DELTA   │ <-- 2. Metadata Panel
│  Active Pairs: 128        | Changes: +3 adds, -1 dels                       │
├─────────────────────────────────────────────────────────────────────────────┤
│  Row      Sample 1               Sample 2               Length(cM)          │ <-- 3. Main Body
│  ─────────────────────────────────────────────────────────────────────────  │    (Active Pairs,
│  1        HG00119                HG00124                12.4500             │     Deltas, or
│  2        HG00154                HG00231                8.9000              │     Help Menu)
│  3        HG00120                HG00311                7.1500              │
│  4        HG00199                HG00201                6.5200              │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│  [Left/Right] Bp  [Up/Down] Scroll  [/] Search  [d] Toggle Delta  [?]: Help │ <-- 4. Status Bar
└─────────────────────────────────────────────────────────────────────────────┘
```

1. **Header Bar**: Displays the currently loaded filename along with quick hints on how to access the help menu or quit.
2. **Metadata Panel**: Displays current status information:
   - **Breakpoint**: The current breakpoint number and total count.
   - **Position**: The genomic base-pair coordinate.
   - **Block Type**: Identifies whether the current block is a `CHECKPOINT` (containing the full active set) or a `DELTA` (containing difference changes).
   - **Active Pairs**: Total active IBD segments.
   - **Changes**: Count of additions and deletions at the current breakpoint (only displayed on Delta blocks).
   - **Filter status**: Displays current active sample search filters.
3. **Main Body**: Displays your data according to the current mode:
   - **Active Pairs View (Default)**: A scrollable grid listing all active segments at the current breakpoint.
   - **Delta Details View**: Displays additions (rendered in green) and deletions (rendered in red) at the selected breakpoint.
   - **Help View**: A quick cheat sheet of keyboard commands.
4. **Status & Command Bar**: Displays standard keyboard instructions, search execution errors, or the input prompt when `/` search is active.

---

## Keyboard Controls

Navigate the viewer using the following shortcuts (VIM navigation, such as h,j,k,l, is supported):

| Key | Action |
| :--- | :--- |
| **`Right Arrow`** / **`l`** | Move forward to the next breakpoint position. |
| **`Left Arrow`** / **`h`** | Move backward to the previous breakpoint position. |
| **`Down Arrow`** / **`j`** | Scroll down the list of pairs by one row. |
| **`Up Arrow`** / **`k`** | Scroll up the list of pairs by one row. |
| **`PageDown`** / **`Space`** | Scroll down the list of pairs by a full page. |
| **`PageUp`** / **`b`** | Scroll up the list of pairs by a full page. |
| **`]`** | Jump forward to the next checkpoint block. |
| **`[`** | Jump backward to the nearest checkpoint block. |
| **`d`** | Toggle view between **Active Pairs** and **Delta Details**. |
| **`/`** | Open the search/filter prompt. (See [Search & Filtering](#search--filtering)) |
| **`Esc`** | Clear the active search filter (when in normal mode). |
| **`?`** | Toggle the Keyboard Shortcuts Help screen. |
| **`q`** / **`Ctrl+C`** | Quit the viewer. |

---

## Search & Filtering

Pressing **`/`** activates the search bar at the bottom of the screen. You can input two types of queries:

1. **Genomic Base-Pair Coordinate**:
   - Enter a number (e.g., `14203500` or `14,203,500`) and press **`Enter`**.
   - The viewer will search the index and jump to the breakpoint closest to that position.
2. **Sample Name Filter**:
   - Enter a text string (e.g., `HG001` or `sample_name`) and press **`Enter`**.
   - The view will filter the scrollable list to display only IBD pairs where at least one of the samples matches your search query.
   - Press **`Esc`** while in the main view to clear the filter and display all pairs.

---

## Sample Name Mapping

IBDF v3 binary files optimize space by storing samples as `uint32` indices. To resolve these to human-readable string names, a companion `.samples` file is required. This file contains one sample name per line corresponding to the 0-based index.

### Detection Logic

If you do not specify a path via the `-s`/`--samples` flag, the viewer will attempt to find a companion file in the same directory as the binary:

1. **`<base_name>.samples`** (e.g., if viewing `chr15.ibdf`, it checks for `chr15.samples`).
2. **`<base_name>.ibdf.samples`** (e.g., checks for `chr15.ibdf.samples`).

If no samples file is found
