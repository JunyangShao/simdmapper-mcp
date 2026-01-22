# Go SimdMapper MCP Server

This tool provides a mapping from Go assembly SIMD instructions to `archsimd` Go API calls. It can be used directly as a CLI tool or as a Model Context Protocol (MCP) server.

## Installation

### From Source
```bash
go build -o simdmcp main.go
```

## Usage

### CLI Mode

You can run the tool directly from the command line by providing the assembly instruction as an argument.

**Example:**
```bash
./simdmcp "VPSUBD X2, X9, X2"
```

**Output:**
```go
if archsimd.X86.AVX() {
        X2 = X9.Sub(X2) \\ X9 is of type Int32x4, X2 is of type Int32x4, X2 is of type Int32x4
}
// Or
if archsimd.X86.AVX() {
        X2 = X9.Sub(X2) \\ X9 is of type Uint32x4, X2 is of type Uint32x4, X2 is of type Uint32x4
}
```

### MCP Server Mode (gemini-cli)

To use this tool with `gemini-cli` (or other MCP clients), configure the client to run this binary. The server communicates via standard input/output (stdio).

**configuration:**

If your `gemini-cli` uses a configuration file (typically `~/.gemini/settings.json` or similar), add this server to the `mcpServers` section:

```json
  "mcpServers": {
    "simd_mapper": {
        "command": "/absolute/to/simdmcp-bin"
    }
  }
```

Once configured, the agent will have access to the `go_simdmapper` tool and can use it to verify SIMD mappings.

Consider using the [gopls MCP](https://go.dev/gopls/features/mcp) in synergy!