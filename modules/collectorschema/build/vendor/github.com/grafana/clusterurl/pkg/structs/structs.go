// Originally from go-gibberish
// Copyright (c) 2015 Rob Renaud
// Licensed under the MIT License. See LICENSES/MIT.txt.
//
// Modifications copyright (c) 2025 Grafana Labs
// Licensed under the Apache License, Version 2.0.

// Package structs contains the definition
// of the structures used.
package structs

// Digraph represents a two-dimensional
// n-gram.
type Digraph struct {
	First  rune
	Second rune
}

// GibberishData contains the data needed
// in order to perform gibberish detection.
type GibberishData struct {
	Occurrences [][]float64
	Positions   map[rune]int
	Threshold   float64
}
