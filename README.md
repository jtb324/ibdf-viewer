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

#### Method 1: Pre-compiled Binaries (Recommended)
We recommend downloading the compiled binary for your architecture from the GitHub Releases page.

#### Method 2: Installing with Go Toolchain (Alternative)
You can compile and install the application directly into your `$GOPATH/bin` using the Go command-line tool (note that this requires a Go installation on your system):

```bash
go install github.com/jtb324/ibdf-viewer@latest
```

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

Pressing **`/`** activates the search/filter bar at the bottom of the screen. You can input three types of queries:

### 1. Genomic Base-Pair Coordinate
* Enter a number (e.g., `14203500` or `14,203,500`) and press **`Enter`**.
* The viewer will search the index and jump to the breakpoint closest to that position.

### 2. SQL-like Query Filter
You can input SQL-like query conditions to filter the visible table rows. The filter is parsed into an AST and evaluated programmatically in memory (which also prevents any security/SQL injection risks).

#### Supported Columns & Aliases
* **`Row`** (alias: `row`): The 1-indexed row number of the pair in the current active set (e.g., `row <= 10`).
* **`Sample 1`** (aliases: `sample1`, `s1`, `p1`, `"Sample 1"`, `` `Sample 1` ``): The name of the first sample.
* **`Sample 2`** (aliases: `sample2`, `s2`, `p2`, `"Sample 2"`, `` `Sample 2` ``): The name of the second sample.
* **`Sample`** (aliases: `sample`, `s`): A helper that matches **either** `Sample 1` or `Sample 2`.
* **`Length(cM)`** (aliases: `length`, `cm`, `"Length(cM)"`, `` `Length(cM)` ``): The centiMorgan length of the IBD segment.

#### Supported Operators
* **Comparisons**: `=`, `!=`, `>`, `>=`, `<`, `<=`, `LIKE` (case-insensitive substring match using `%` wildcards).
* **Logicals**: `AND`, `OR`, `NOT`.
* **Grouping**: Parentheses `( )` to enforce operator precedence.

#### Example SQL Filters
* Show segments of at least 5 cM: `length >= 5`
* Show segments between 3.5 cM and 10 cM: `cm > 3.5 AND cm < 10`
* Show segments involving sample `HG00096`: `sample = 'HG00096'`
* Show segments involving any sample starting with `NA`: `sample LIKE 'NA%'`
* Show segments where `Sample 1` matches `HG00` and length is greater than 10 cM: `s1 LIKE '%HG00%' AND length > 10`
* Complex grouping: `(s1 = 'HG00096' OR s2 = 'HG00096') AND length >= 12.5`

*Note: If the query has any syntax errors or refers to invalid columns, a helpful validation error will be displayed in the status bar (e.g., `invalid column name "age"`).*

### 3. Simple Sample Name Filter (Fallback)
* If your query does not contain SQL operators (such as `=`, `>`, `like`, etc.), it automatically falls back to matching the sample names as a substring.
* For example, typing `HG001` and pressing **`Enter`** will display all rows where either `Sample 1` or `Sample 2` contains `HG001`.
* Press **`Esc`** while in the main view to clear the filter and display all pairs.

---

## Sample Name Mapping

IBDF v3 binary files optimize space by storing samples as `uint32` indices. To resolve these to human-readable string names, a companion `.samples` file is required. This file contains one sample name per line corresponding to the 0-based index.

### Detection Logic

If you do not specify a path via the `-s`/`--samples` flag, the viewer will attempt to find a companion file in the same directory as the binary:

1. **`<base_name>.samples`** (e.g., if viewing `chr15.ibdf`, it checks for `chr15.samples`).
2. **`<base_name>.ibdf.samples`** (e.g., checks for `chr15.ibdf.samples`).

If no samples file is found:
*   **During Auto-Detection**: The application will terminate immediately with a fatal error message instructing you to place the `.samples` file in the same directory or supply its path manually via the `-s`/`--samples` flag.
*   **During Explicit Load**: If you explicitly supplied a path that cannot be read, the program prints a warning to `stderr` and launches anyway, falling back to displaying sample names as generic placeholders: `Sample_0`, `Sample_1`, `Sample_2`, etc.
