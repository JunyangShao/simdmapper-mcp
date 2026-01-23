package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	simdmcp "github.com/JunyangShao/simdmapper-mcp/mcp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type simdMapperParams struct {
	Asm string `json:"asm" jsonschema:"the assembly instruction to be mapped to simd intrinsics."`
}

func SimdMapperHandler(ctx context.Context, req *mcp.CallToolRequest, params simdMapperParams) (*mcp.CallToolResult, any, error) {
	query := params.Asm
	if len(query) == 0 {
		return nil, nil, fmt.Errorf("empty query")
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: simdmcp.SimdMapper(query)}},
	}, nil, nil
}


func main() {
	if len(os.Args) > 1 {
		query := strings.Join(os.Args[1:], " ")
		fmt.Println(simdmcp.SimdMapper(query))
		return
	}

	// Create a server with a single tool.
	server := mcp.NewServer(&mcp.Implementation{Name: "greeter", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name: "go_simdmapper",
		Description: ` This tool is useful when you are uncertain about the mappings
of Go assembly *vector* instructions to the archsimd intrinsic APIs. When the user
ask you to lift an Go assembly that involves vector instructions, you should use
this tool to verify your mappings are right.

Given a Go assembly vector instruction, this
tool will return the archsimd API form of it and its CPU feature requirement.
For example, given argument {"asm": "VPADDD X2, X9, X2"}, the tool will return
"""
	if archsimd.X86.AVX() {
		X2 = X2.Add(X9)
	}
"""
This tool only supports amd64 instructions at this moment.

This tool might miss instructions because some archsimd intrinsics are not exactly mapped
to one instruction (e.g. emulated), or are in other files that's not in the knowledge
of this tool. In that case you should read the package doc of archsimd to find the
right APIs.`,
	}, SimdMapperHandler)
	// Run the server over stdin/stdout, until the client disconnects.
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
