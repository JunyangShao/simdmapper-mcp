// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type simdMapperParams struct {
	Asm string `json:"asm" jsonschema:"the assembly instruction to be mapped to simd intrinsics."`
}

type simdOperand struct {
	kind string
	name string
}

type simdAsm struct {
	name     string
	operands []simdOperand
}

type goAPISignature struct {
	Name       string
	Shape      string
	ArgTypes   []string // The last one is the return type
	CPUFeature string
	ConstImm   string
	Doc        string
	ResInArg0 bool
}

var regMap = map[string]byte{
	"Int8x16":    'X',
	"Uint8x16":   'X',
	"Int16x8":    'X',
	"Uint16x8":   'X',
	"Int32x4":    'X',
	"Uint32x4":   'X',
	"Int64x2":    'X',
	"Uint64x2":   'X',
	"Float32x4":  'X',
	"Float64x2":  'X',
	"Int8x32":    'Y',
	"Uint8x32":   'Y',
	"Int16x16":   'Y',
	"Uint16x16":  'Y',
	"Int32x8":    'Y',
	"Uint32x8":   'Y',
	"Int64x4":    'Y',
	"Uint64x4":   'Y',
	"Float32x8":  'Y',
	"Float64x4":  'Y',
	"Int8x64":    'Z',
	"Uint8x64":   'Z',
	"Int16x32":   'Z',
	"Uint16x32":  'Z',
	"Int32x16":   'Z',
	"Uint32x16":  'Z',
	"Int64x8":    'Z',
	"Uint64x8":   'Z',
	"Float32x16": 'Z',
	"Float64x8":  'Z',
}

type mapper struct {
	name           string
	vectorComments []string
	cpuFeature     string
	ok             bool
	ops            []string
	maskReg        string
	resultType     string
}

func (m *mapper) parseReg(opr simdOperand, goname string) {
	if opr.kind != "Reg" ||
		((opr.name[0] == 'X' ||
			opr.name[0] == 'Y' ||
			opr.name[0] == 'Z') &&
			opr.name[0] != regMap[goname]) {
		m.parseLoad(opr, goname)
		return
	}
	m.vectorComments = append(m.vectorComments, fmt.Sprintf("%s is of type %s", opr.name, goname))
	m.resultType = goname
	m.ops = append(m.ops, opr.name)
}

func (m *mapper) parseVecRegAsScalar(opr simdOperand, goname string) {
	if opr.kind != "Reg" ||
		((opr.name[0] == 'X' ||
			opr.name[0] == 'Y' ||
			opr.name[0] == 'Z') &&
			opr.name[0] != regMap[goname]) {
		m.parseLoad(opr, goname)
		return
	}
	m.ops = append(m.ops, fmt.Sprintf("%s.AsUint8x16().GetElem(0)", opr.name))
	m.vectorComments = append(m.vectorComments, fmt.Sprintf("%s must be an 128-bit vector", opr.name))
}

func (m *mapper) parseImm(opr simdOperand, goname string) {
	if opr.kind != "Imm" || goname != "uint8" {
		m.ok = false
		return
	}
	m.ops = append(m.ops, opr.name)
}

func (m *mapper) parseImmSplit2(opr simdOperand, goname1, goname2 string) {
	if opr.kind != "Imm" || goname1 != "uint8" || goname2 != "uint8" {
		m.ok = false
		return
	}
	m.ops = append(m.ops, fmt.Sprintf("%s&0b11", opr.name))
	m.ops = append(m.ops, fmt.Sprintf("%s>>4&0b11", opr.name))
}

func (m *mapper) parseLoad(opr simdOperand, goname string) {
	if opr.kind != "Load" {
		m.ok = false
		return
	}
	if _, ok := regMap[goname]; ok {
		m.ops = append(m.ops, fmt.Sprintf("archsimd.Load%s(*%s)", goname, opr.name))
		return
	}
	// Scalar load, just dereference the pointer.
	m.ops = append(m.ops, fmt.Sprintf("*%s", opr.name))
}

func (m *mapper) emitGoCode() string {
	if !m.ok || m.cpuFeature == "" {
		return ""
	}
	mask := ""
	if m.maskReg != "" {
		mask = fmt.Sprintf(".Masked(%s)", m.maskReg)
		if !strings.HasPrefix(m.cpuFeature, "AVX512") {
			m.cpuFeature = "AVX512"
		}
		m.vectorComments = append(m.vectorComments, fmt.Sprintf("%s is of type %s", m.maskReg,
			strings.ReplaceAll(
				strings.ReplaceAll(
					strings.ReplaceAll(
						m.resultType,
						"Float", "Mask"),
					"Uint", "Mask"),
				"Int", "Mask")))
	}
	return fmt.Sprintf("if archsimd.X86.%s() {\n\t%s = %s.%s(%s)%s // %s\n}",
		m.cpuFeature, m.ops[len(m.ops)-1], m.ops[0], m.name,
		strings.Join(m.ops[1:len(m.ops)-1], ", "), mask, strings.Join(m.vectorComments, ", "))
}

// SimdMapper parses the assembly instruction and returns the corresponding Go code.
func SimdMapper(query string) string {
	var asm simdAsm
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return "Illegal input"
	}
	asm.name = parts[0]
	asm.operands = make([]simdOperand, len(parts)-1)
	for i, op := range parts[1:] {
		if strings.HasPrefix(op, "$") {
			asm.operands[i] = simdOperand{kind: "Imm", name: strings.Trim(op[1:], ",")}
		} else if strings.Contains(op, "(") {
			addrParts := regexp.MustCompile(`(-?\d+)?(\(\w+\))?(\(\w+\))`).FindStringSubmatch(strings.Trim(op, ","))
			if len(addrParts) != 4 {
				return "Illegal input"
			}
			addr := ""
			if addrParts[2] != "" {
				addr += strings.Trim(strings.Trim(addrParts[2], "("), ")")
			}
			if addrParts[3] != "" {
				addr += "+" + strings.Trim(strings.Trim(addrParts[3], "("), ")")
			}
			if addrParts[1] != "" {
				addr += addrParts[1]
			}
			asm.operands[i] = simdOperand{kind: "Load", name: strings.TrimPrefix(addr, "+")}
		} else {
			asm.operands[i] = simdOperand{kind: "Reg", name: strings.Trim(op, ",")}
		}
	}
	candidates := []string{}
	if sigs, ok := opMap[asm.name]; ok {
		// TODO: if no exact match, try to find a fuzzy match?
		for _, sig := range sigs {
			mapper := &mapper{
				name:       sig.Name,
				cpuFeature: sig.CPUFeature,
				ok:         true,
			}
			operands := make([]simdOperand, len(asm.operands))
			processLastArg := func(arg0idx int) {
				if !sig.ResInArg0 {
					mapper.parseReg(operands[len(operands)-1], sig.ArgTypes[len(sig.ArgTypes)-1])
				} else {
					mapper.parseReg(operands[arg0idx], sig.ArgTypes[len(sig.ArgTypes)-1])
				}
			}
			copy(operands, asm.operands)
			// Check const imm
			if sig.ConstImm != "" {
				if operands[0].kind != "Imm" {
					continue
				}
				// compare operands[0].name with sig.ConstImm mathematically
				imm, err := strconv.ParseUint(operands[0].name, 0, 64)
				if err != nil {
					continue
				}
				constImm, err := strconv.ParseUint(sig.ConstImm, 0, 64)
				if err != nil {
					continue
				}
				if imm != constImm {
					continue
				}
				operands = operands[1:]
			}
			// Check if it's a masked operation
			no := len(operands)
			if no > 1 && operands[no-2].kind == "Reg" && operands[no-2].name[0] == 'K' {
				// Check that this is the only mask register
				kcnt := 0
				for _, op := range operands {
					if op.kind == "Reg" && op.name[0] == 'K' {
						kcnt++
					}
				}
				if kcnt != 1 {
					continue
				}
				mapper.maskReg = operands[no-2].name
				operands = append(operands[:no-2], operands[no-1])
			}
			switch sig.Shape {
			case "op1":
				mapper.parseReg(operands[0], sig.ArgTypes[0])
				processLastArg(0)
			case "op2":
				mapper.parseReg(operands[1], sig.ArgTypes[0])
				mapper.parseReg(operands[0], sig.ArgTypes[1])
				processLastArg(1)
			case "op2_21", "op2_21Type1":
				mapper.parseReg(operands[0], sig.ArgTypes[0])
				mapper.parseReg(operands[1], sig.ArgTypes[1])
				processLastArg(0)
			case "op3":
				mapper.parseReg(operands[2], sig.ArgTypes[0])
				mapper.parseReg(operands[1], sig.ArgTypes[1])
				mapper.parseReg(operands[0], sig.ArgTypes[2])
				processLastArg(2)
			case "op3_21", "op3_21Type1":
				mapper.parseReg(operands[1], sig.ArgTypes[0])
				mapper.parseReg(operands[2], sig.ArgTypes[1])
				mapper.parseReg(operands[0], sig.ArgTypes[2])
				processLastArg(1)
			case "op3_231Type1":
				mapper.parseReg(operands[1], sig.ArgTypes[0])
				mapper.parseReg(operands[0], sig.ArgTypes[1])
				mapper.parseReg(operands[2], sig.ArgTypes[2])
				processLastArg(1)
			case "op2VecAsScalar":
				mapper.parseReg(operands[1], sig.ArgTypes[0])
				mapper.parseVecRegAsScalar(operands[0], sig.ArgTypes[1])
				processLastArg(1)
			case "op3VecAsScalar":
				mapper.parseReg(operands[2], sig.ArgTypes[0])
				mapper.parseVecRegAsScalar(operands[1], sig.ArgTypes[1])
				mapper.parseReg(operands[0], sig.ArgTypes[2])
				processLastArg(2)
			case "op4":
				mapper.parseReg(operands[3], sig.ArgTypes[0])
				mapper.parseReg(operands[2], sig.ArgTypes[1])
				mapper.parseReg(operands[1], sig.ArgTypes[2])
				mapper.parseReg(operands[0], sig.ArgTypes[3])
				processLastArg(3)
			case "op4_231Type1":
				mapper.parseReg(operands[2], sig.ArgTypes[0])
				mapper.parseReg(operands[1], sig.ArgTypes[1])
				mapper.parseReg(operands[3], sig.ArgTypes[2])
				mapper.parseReg(operands[0], sig.ArgTypes[3])
				processLastArg(2)
			case "op4_31":
				mapper.parseReg(operands[1], sig.ArgTypes[0])
				mapper.parseReg(operands[2], sig.ArgTypes[1])
				mapper.parseReg(operands[3], sig.ArgTypes[2])
				mapper.parseReg(operands[0], sig.ArgTypes[3])
				processLastArg(1)
			case "op1Imm8":
				mapper.parseReg(operands[1], sig.ArgTypes[0])
				mapper.parseImm(operands[0], sig.ArgTypes[1])
				processLastArg(1)
			case "op2Imm8", "op2Imm8_SHA1RNDS4":
				mapper.parseReg(operands[2], sig.ArgTypes[0])
				mapper.parseImm(operands[0], sig.ArgTypes[1])
				mapper.parseReg(operands[1], sig.ArgTypes[2])
				processLastArg(2)
			case "op2Imm8_2I":
				mapper.parseReg(operands[2], sig.ArgTypes[0])
				mapper.parseReg(operands[1], sig.ArgTypes[1])
				mapper.parseImm(operands[0], sig.ArgTypes[2])
				processLastArg(2)
			case "op2Imm8_II":
				mapper.parseReg(operands[2], sig.ArgTypes[0])
				mapper.parseImmSplit2(operands[0], sig.ArgTypes[1], sig.ArgTypes[2])
				mapper.parseReg(operands[1], sig.ArgTypes[3])
				processLastArg(2)
			case "op3Imm8":
				mapper.parseReg(operands[3], sig.ArgTypes[0])
				mapper.parseImm(operands[0], sig.ArgTypes[1])
				mapper.parseReg(operands[2], sig.ArgTypes[2])
				mapper.parseReg(operands[1], sig.ArgTypes[3])
				processLastArg(3)
			case "op3Imm8_2I":
				mapper.parseReg(operands[3], sig.ArgTypes[0])
				mapper.parseReg(operands[2], sig.ArgTypes[1])
				mapper.parseImm(operands[0], sig.ArgTypes[2])
				mapper.parseReg(operands[1], sig.ArgTypes[3])
				processLastArg(3)
			case "op4Imm8":
				mapper.parseReg(operands[4], sig.ArgTypes[0])
				mapper.parseImm(operands[0], sig.ArgTypes[1])
				mapper.parseReg(operands[3], sig.ArgTypes[2])
				mapper.parseReg(operands[2], sig.ArgTypes[3])
				mapper.parseReg(operands[1], sig.ArgTypes[4])
				processLastArg(4)
			default:
				continue
			}
			candidates = append(candidates, mapper.emitGoCode())
		}
	}
	validCandidates := []string{}
	for _, c := range candidates {
		if c != "" {
			validCandidates = append(validCandidates, c)
		}
	}
	if len(validCandidates) == 0 {
		return "Missing a direct translation for this instruction, but similar instructions might be available. Please check the documentation at: https://pkg.go.dev/simd/archsimd"
	}
	return strings.Join(validCandidates, "\n// Or\n")
}

func SimdMapperHandler(ctx context.Context, req *mcp.CallToolRequest, params simdMapperParams) (*mcp.CallToolResult, any, error) {
	query := params.Asm
	if len(query) == 0 {
		return nil, nil, fmt.Errorf("empty query")
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: SimdMapper(query)}},
	}, nil, nil
}
