# MCP 2025-06-18 Implementation Plan

## Overview

This document outlines the implementation plan for supporting the new Model Context Protocol specification version 2025-06-18 in MCP Conductor.

**Specification URL**: https://modelcontextprotocol.io/specification/2025-06-18/changelog

## Key Changes Summary

### Breaking Changes
1. **Remove JSON-RPC Batching** - No longer supported
2. **MCP-Protocol-Version Header** - Required for HTTP transport
3. **Lifecycle Operations** - Changed from SHOULD to MUST

### New Features
1. **Structured Tool Output** - Tools can return structured JSON data
2. **Resource Links** - Tools can reference MCP resources
3. **Elicitation** - Servers can request user input
4. **OAuth2 Resource Server** - Enhanced authorization support

### Enhancements
1. **Enhanced Security** - Better security considerations
2. **Schema Improvements** - `_meta`, `context`, `title` fields
3. **Tool Annotations** - Enhanced tool metadata

## Implementation Phases

### Phase 1: Breaking Changes (Week 1)

#### 1.1 Remove JSON-RPC Batching
**Files**: `internal/mcp/server.go`
- Remove batch request handling
- Remove batch response logic
- Update protocol documentation

#### 1.2 Add MCP-Protocol-Version Header
**Files**: `internal/mcp/server.go`
- Add header validation for HTTP transport
- Ensure version negotiation works correctly
- Add header to outgoing HTTP requests

#### 1.3 Update Lifecycle Operations
**Files**: `internal/mcp/server.go`
- Ensure all lifecycle operations are properly implemented
- Update error handling for mandatory operations

### Phase 2: Core Features (Week 2)

#### 2.1 Structured Tool Output
**Files**: `internal/mcp/tools.go`, agent implementations
- Add `structuredContent` field to tool responses
- Maintain backward compatibility with text content
- Update Kubernetes tools to return structured data

**Example Implementation**:
```go
type ToolResult struct {
    Content           []Content     `json:"content"`
    StructuredContent interface{}   `json:"structuredContent,omitempty"`
    IsError          bool          `json:"isError"`
}
```

#### 2.2 Resource Links Support
**Files**: `internal/mcp/tools.go`
- Add resource link content type
- Enable tools to reference MCP resources
- Update tool response handling

#### 2.3 Output Schema Support
**Files**: `internal/mcp/tools.go`
- Add `outputSchema` field to tool definitions
- Implement schema validation for structured outputs
- Update tool registration

### Phase 3: Advanced Features (Week 3)

#### 3.1 Elicitation Support
**Files**: New `internal/mcp/elicitation.go`
- Implement `elicitation/create` method
- Add capability declaration
- Support structured user input requests

**Use Cases**:
- Interactive Kubernetes operations
- Dynamic configuration requests
- User confirmation for sensitive operations

#### 3.2 OAuth2 Resource Server
**Files**: New `internal/mcp/auth.go`
- Add OAuth2 resource server metadata
- Implement authorization server discovery
- Add Resource Indicators (RFC 8707) support

#### 3.3 Enhanced Security
**Files**: Multiple
- Review and enhance input validation
- Add rate limiting for tool calls
- Improve error handling and logging

### Phase 4: Testing & Documentation (Week 4)

#### 4.1 Protocol Support Matrix
**Files**: `README.md`
- Update support matrix to include 2025-06-18
- Document feature compatibility
- Add migration guide

#### 4.2 Testing
- Test compatibility with both 2025-03-26 and 2025-06-18
- Validate structured tool outputs
- Test elicitation workflows

#### 4.3 Documentation Updates
- Update architecture documentation
- Add examples for new features
- Update quickstart guide

## Compatibility Strategy

### Dual Version Support
- Support both 2025-03-26 and 2025-06-18 simultaneously
- Use capability negotiation to determine feature availability
- Graceful degradation for older clients

### Migration Path
1. **Phase 1**: Deploy breaking changes with backward compatibility
2. **Phase 2**: Add new features with feature flags
3. **Phase 3**: Enable advanced features by default
4. **Phase 4**: Full 2025-06-18 support with 2025-03-26 compatibility

## Risk Assessment

### High Risk
- **JSON-RPC Batching Removal**: May break existing clients
- **Header Requirements**: HTTP transport changes

### Medium Risk
- **Structured Output**: Backward compatibility concerns
- **Elicitation**: New interaction patterns

### Low Risk
- **Resource Links**: Additive feature
- **OAuth2**: Optional enhancement

## Success Criteria

1. ✅ All breaking changes implemented without breaking existing functionality
2. ✅ Structured tool output working with Kubernetes tools
3. ✅ Elicitation support functional for interactive operations
4. ✅ Full test coverage for new features
5. ✅ Documentation updated and comprehensive
6. ✅ Backward compatibility maintained for 2025-03-26

## Next Steps

1. **Create feature branch**: `feature/mcp-2025-06-18` ✅
2. **Start Phase 1**: Remove JSON-RPC batching
3. **Implement breaking changes**: Header support, lifecycle updates
4. **Begin Phase 2**: Structured tool output implementation

## Resources

- [MCP 2025-06-18 Specification](https://modelcontextprotocol.io/specification/2025-06-18)
- [Changelog](https://modelcontextprotocol.io/specification/2025-06-18/changelog)
- [Tools Documentation](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)
- [Elicitation Documentation](https://modelcontextprotocol.io/specification/2025-06-18/client/elicitation)
