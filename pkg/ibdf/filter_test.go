package ibdf

import (
	"testing"
)

func TestParseFilterAndMatch(t *testing.T) {
	samples := []string{"HG00096", "HG00097", "NA19240"}

	// Mock active pairs
	pair1 := IBDPair{CM: 12.5, P1: 0, P2: 1} // HG00096, HG00097
	pair2 := IBDPair{CM: 4.2, P1: 1, P2: 2}  // HG00097, NA19240
	pair3 := IBDPair{CM: 8.0, P1: 0, P2: 2}  // HG00096, NA19240

	tests := []struct {
		query    string
		pair     IBDPair
		row      int
		expected bool
		hasError bool
	}{
		// Basic length comparisons
		{query: "length >= 5", pair: pair1, row: 1, expected: true, hasError: false},
		{query: "length >= 5", pair: pair2, row: 2, expected: false, hasError: false},
		{query: "length < 5", pair: pair2, row: 2, expected: true, hasError: false},
		{query: "cm = 8", pair: pair3, row: 3, expected: true, hasError: false},
		{query: "cm != 8.0", pair: pair3, row: 3, expected: false, hasError: false},

		// Sample name comparisons
		{query: "sample1 = 'HG00096'", pair: pair1, row: 1, expected: true, hasError: false},
		{query: "sample1 = 'HG00097'", pair: pair1, row: 1, expected: false, hasError: false},
		{query: "sample2 = 'NA19240'", pair: pair3, row: 3, expected: true, hasError: false},

		// Quoted identifier with spaces
		{query: "`Sample 1` = 'HG00096'", pair: pair1, row: 1, expected: true, hasError: false},
		{query: "\"Sample 2\" = 'NA19240'", pair: pair3, row: 3, expected: true, hasError: false},

		// Sample alias matching either sample1 or sample2
		{query: "sample = 'HG00097'", pair: pair1, row: 1, expected: true, hasError: false},
		{query: "sample = 'HG00097'", pair: pair2, row: 2, expected: true, hasError: false},
		{query: "sample = 'HG00097'", pair: pair3, row: 3, expected: false, hasError: false},

		// SQL LIKE wildcard patterns
		{query: "sample LIKE '%096'", pair: pair1, row: 1, expected: true, hasError: false},
		{query: "sample LIKE 'HG%'", pair: pair2, row: 2, expected: true, hasError: false},
		{query: "sample LIKE '%192%'", pair: pair2, row: 2, expected: true, hasError: false},
		{query: "sample LIKE '%192%'", pair: pair1, row: 1, expected: false, hasError: false},

		// Row filtering
		{query: "row = 1", pair: pair1, row: 1, expected: true, hasError: false},
		{query: "row = 1", pair: pair2, row: 2, expected: false, hasError: false},
		{query: "row > 1", pair: pair2, row: 2, expected: true, hasError: false},

		// Logical compound operators
		{query: "length > 5 AND sample1 = 'HG00096'", pair: pair1, row: 1, expected: true, hasError: false},
		{query: "length > 5 AND sample1 = 'HG00096'", pair: pair3, row: 3, expected: true, hasError: false},
		{query: "length > 10 AND sample1 = 'HG00096'", pair: pair3, row: 3, expected: false, hasError: false},
		{query: "length > 10 OR sample2 = 'NA19240'", pair: pair2, row: 2, expected: true, hasError: false},
		{query: "NOT length >= 5", pair: pair2, row: 2, expected: true, hasError: false},

		// Parentheses grouping
		{query: "(sample = 'HG00096' OR sample = 'NA19240') AND length < 10", pair: pair3, row: 3, expected: true, hasError: false},
		{query: "sample = 'HG00096' OR (sample = 'NA19240' AND length < 5)", pair: pair2, row: 2, expected: true, hasError: false},

		// Validation errors
		{query: "age > 5", pair: pair1, row: 1, expected: false, hasError: true},
		{query: "length", pair: pair1, row: 1, expected: false, hasError: true}, // not a boolean expression
		{query: "length >= 'abc'", pair: pair1, row: 1, expected: false, hasError: true}, // type mismatch during evaluation
	}

	for _, tc := range tests {
		filter, err := ParseFilter(tc.query)
		if tc.hasError {
			if err == nil {
				// Some errors (like type mismatch) happen at evaluation time, so we must evaluate to see the error
				if filter != nil {
					res, evalErr := filter.(*compiledFilter).expr.Eval(tc.pair, tc.row, samples)
					if evalErr != nil {
						continue // expected evaluation error
					}
					t.Errorf("expected error for query %q, got none (eval returned %v)", tc.query, res)
				} else {
					t.Errorf("expected error for query %q, got none", tc.query)
				}
			}
			continue
		}

		if err != nil {
			t.Fatalf("unexpected error parsing query %q: %v", tc.query, err)
		}

		match := filter.Match(tc.pair, tc.row, samples)
		if match != tc.expected {
			t.Errorf("query %q: expected match = %t, got %t (pair cM=%.1f, p1=%d, p2=%d)", tc.query, tc.expected, match, tc.pair.CM, tc.pair.P1, tc.pair.P2)
		}
	}
}
