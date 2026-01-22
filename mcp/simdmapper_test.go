// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mcp_test

import (
	"testing"

	"github.com/JunyangShao/simdmapper-mcp/mcp"
)

// TODO: add more test data.
var testData = []string{
	"VPINSRD $7, (R11), X9, X11",
	"VPADDD (BX), X9, X2",
	"VHADDPS X0, X0, X0",
	"VEXTRACTF128 $0x01, Y0, X1",
	"VDIVPD Z9, Z21, K3, Z14",
}

// test simdMapper
func TestSimdMapper(t *testing.T) {
	notWant := "Missing a direct translation for this instruction, but similar instructions might be available. Please check the documentation at: https://pkg.go.dev/simd/archsimd"
	notWant2 := "Illegal input"
	for _, data := range testData {
		got := mcp.SimdMapper(data)
		if got == notWant || got == notWant2 {
			t.Errorf("SimdMapper doesn't support this instruction: %s\ngot: %s", data, got)
		}
	}
}
