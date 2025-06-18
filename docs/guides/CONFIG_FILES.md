# MCP Conductor Configuration Files

This document describes the configuration files for integrating MCP Conductor with various AI tools.

## 📋 Available Configuration Files

### 1. `claude_desktop_config.json` - Claude Desktop Integration

**Purpose**: Configuration for Claude Desktop app using stdio transport

**Usage**:
```bash
# On macOS:
cp examples/configs/claude_desktop_config.json ~/Library/Application\ Support/Claude/claude_desktop_config.json

# On Windows:
copy examples/configs/claude_desktop_config.json %APPDATA%\Claude\claude_desktop_config.json

# Alternative method (all platforms):
# 1. Open Claude Desktop
# 2. Go to Settings → Developer → Edit Config
# 3. Paste the configuration content
```

**Note**: Claude Desktop is currently only available for **macOS and Windows**. Linux is not yet supported.

**Transport**: stdio (standard input/output)
**Protocol Version**: 2025-03-26
**Features**:
- ✅ Local binary execution
- ✅ Direct kubeconfig access
- ✅ JSON-RPC batching support
- ✅ All 9 Kubernetes tools

### 2. `augment_mcp_config.json` - Augment Code Integration

**Purpose**: Configuration for Augment Code IDE plugin using HTTP transport

**Usage**:
```bash
# Use with Augment Code IDE plugin
# Configure in IDE settings or place in project root
```

**Transport**: HTTP (Streamable HTTP)
**Protocol Version**: 2025-03-26
**Endpoint**: `http://127.0.0.1:8080`
**Features**:
- ✅ Streamable HTTP transport (recommended)
- ✅ Kubernetes cluster integration
- ✅ Tool annotations for better AI understanding
- ✅ All 9 Kubernetes tools

## 🚀 Quick Setup

### For Claude Desktop:
1. Build the local binary: `go build -o bin/mcp-server ./cmd/mcp-server`
2. Copy config to the correct location:
   - **macOS**: `cp examples/configs/claude_desktop_config.json ~/Library/Application\ Support/Claude/claude_desktop_config.json`
   - **Windows**: `copy examples/configs/claude_desktop_config.json %APPDATA%\Claude\claude_desktop_config.json`
   - **Alternative**: Open Claude Desktop → Settings → Developer → Edit Config, then paste the contents
3. Restart Claude Desktop

### For Augment Code:
1. Deploy to Kubernetes: `kubectl apply -f deploy/kubernetes/`
2. Port forward: `kubectl port-forward svc/mcp-server 8080:8080 -n mcp-system`
3. Use `examples/configs/augment_mcp_config.json` in IDE settings

## 🛠️ Available Tools

Both configurations provide access to these Kubernetes tools:

1. `list_kubernetes_pods` - List pods in namespace
2. `list_kubernetes_nodes` - List cluster nodes
3. `get_cluster_summary` - Get cluster overview
4. `list_kubernetes_deployments` - List deployments
5. `list_kubernetes_services` - List services
6. `list_kubernetes_daemonsets` - List daemonsets
7. `get_kubernetes_pod` - Get specific pod details
8. `get_kubernetes_node_status` - Get node status
9. `list_kubernetes_namespaces` - List all namespaces

## 📝 Notes

- **Protocol Version**: Both configs use MCP Specification 2025-03-26
- **Security**: HTTP config includes Origin validation and security headers
- **Performance**: Both support JSON-RPC batching for efficient communication
- **Compatibility**: Backward compatible with older MCP clients

## 🔧 Troubleshooting

### Claude Desktop Issues:
- Ensure binary path is correct in config
- Check KUBECONFIG environment variable
- Verify binary has execute permissions
- Check Claude Desktop logs:
  - **macOS**: `~/Library/Logs/Claude/mcp*.log`
  - **Windows**: `%APPDATA%\Claude\logs\mcp*.log`

### Augment Code Issues:
- Ensure port forwarding is active
- Check Kubernetes deployment status
- Verify network connectivity to cluster

## 📚 References

- [MCP Specification 2025-03-26](https://modelcontextprotocol.io/specification/2025-03-26)
- [Claude Desktop MCP Guide](https://docs.anthropic.com/claude/docs/mcp)
- [Augment Code Documentation](https://docs.augmentcode.com/)
