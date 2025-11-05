// Originally from go-gibberish
// Copyright (c) 2015 Rob Renaud
// Licensed under the MIT License. See LICENSES/MIT.txt.
//
// Modifications copyright (c) 2025 Grafana Labs
// Licensed under the Apache License, Version 2.0.

// Package gibberish contains methods to tell whether
// the input is gibberish or not.
package gibberish

import (
	"github.com/grafana/clusterurl/pkg/analysis"
	"github.com/grafana/clusterurl/pkg/structs"
)

// IsGibberish returns true if the input string is likely
// to be gibberish
func IsGibberish(input string, data *structs.GibberishData) bool {
	value, err := analysis.AverageTransitionProbability(input, data.Occurrences, data.Positions)
	return value <= data.Threshold && err == nil
}
