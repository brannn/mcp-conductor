# MCP Conductor

A Kubernetes-native orchestration platform for Model Context Protocol (MCP) agents, enabling distributed AI task execution across cloud-native environments with full MCP specification compliance.

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.21+-blue.svg)](https://kubernetes.io)
[![MCP Protocol](https://img.shields.io/badge/MCP-2024--11--05%20%7C%202025--03--26%20%7C%202025--06--18-green.svg)](https://modelcontextprotocol.io)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

## Overview

MCP Conductor extends Kubernetes into an orchestration platform for AI agents. It provides a cloud-native foundation for deploying, managing, and scaling MCP-compatible agents across distributed environments, with complete native support for MCP Protocol versions **2024-11-05**, **2025-03-26**, and **2025-06-18**.

### Key Features

- **Complete MCP Protocol Support**: Full compatibility with MCP 2024-11-05, 2025-03-26, and 2025-06-18
- **Advanced MCP 2025-06-18 Features**: Output schemas, structured content, resource links, elicitation support
- **Multi-Transport**: HTTP (Streamable HTTP) and stdio transports with automatic protocol negotiation
- **Kubernetes-Native**: Built on Custom Resource Definitions (CRDs) with proper RBAC
- **Domain-Specific Orchestration**: Organize agents by domain with intelligent task routing
- **Multi-Language Agent Support**: Any language with Kubernetes client libraries
- **Scalable Architecture**: Horizontal scaling with optimized performance
- **Enhanced Error Handling**: Rich context, recovery suggestions, and validation (MCP 2025-06-18)
- **Interactive Workflows**: Elicitation support for user input during execution (MCP 2025-06-18)
- **Tool Annotations**: Enhanced AI understanding with comprehensive metadata

### MCP Protocol Support Matrix

| Feature | MCP 2024-11-05 | MCP 2025-03-26 | MCP 2025-06-18 | Status |
|---------|----------------|----------------|----------------|---------|
| **Protocol Version Negotiation** | ✅ | ✅ | ✅ | **Complete** |
| **Tool Execution** | ✅ | ✅ | ✅ | **Complete** |
| **Tool Annotations** | ❌ | ✅ | ✅ | **Complete** |
| **Output Schemas** | ❌ | ❌ | ✅ | **Complete** |
| **Structured Content** | ❌ | ❌ | ✅ | **Complete** |
| **Resource Links** | ❌ | ❌ | ✅ | **Complete** |
| **Input Validation** | ❌ | ❌ | ✅ | **Complete** |
| **Enhanced Error Handling** | ❌ | ❌ | ✅ | **Complete** |
| **Elicitation Support** | ❌ | ❌ | ✅ | **Complete** |
| **JSON-RPC Batching** | ✅ | ✅ | ❌ | Deprecated in 2025-06-18 |
| **Streamable HTTP Transport** | ✅ | ✅ | ✅ | **Complete** |
| **stdio Transport** | ✅ | ✅ | ✅ | **Complete** |
| **Backward Compatibility** | ✅ | ✅ | ✅ | **Complete** |

## Architecture

MCP Conductor leverages Kubernetes primitives to eliminate the complexity of traditional MCP server architectures. Instead of managing agent connections, message queues, and state synchronization, we use Kubernetes CRDs as the coordination layer.

```
┌─────────────────────────────────────────────────────────────┐
│                    MCP Conductor                            │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐              ┌─────────────────┐       │
│  │   Controller    │              │   MCP Server    │       │
│  │  (CRD Lifecycle │              │ (HTTP + stdio)  │       │
│  │   Management,   │              │ MCP 2024-2025   │       │
│  │   Scheduling)   │              │                 │       │
│  └─────────────────┘              └─────────────────┘       │
├─────────────────────────────────────────────────────────────┤
│                 Kubernetes API Server                       │
│                   (Coordination Layer)                      │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐ │
│  │   Agent CRDs    │  │   Task CRDs     │  │   Events &   │ │
│  │ (Capabilities,  │  │ (Work Units,    │  │   Metrics    │ │
│  │  Status, Tags)  │  │  Assignment,    │  │ (Observ-     │ │
│  │                 │  │  Results)       │  │  ability)    │ │
│  └─────────────────┘  └─────────────────┘  └──────────────┘ │
├─────────────────────────────────────────────────────────────┤
│                    Agent Ecosystem                          │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐ │
│  │   Any Language  │  │   Any Language  │  │  Any Language│ │
│  │   Agent Pod     │  │   Agent Pod     │  │  Agent Pod   │ │
│  ├─────────────────┤  ├─────────────────┤  ├──────────────┤ │
│  │ CRD Watcher     │  │ CRD Watcher     │  │ CRD Watcher  │ │
│  │ Task Executor   │  │ Task Executor   │  │ Task Executor│ │
│  │ Status Updater  │  │ Status Updater  │  │ Status Update│ │
│  └─────────────────┘  └─────────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Key Benefits of Kubernetes-Native Design

- **No Agent Connections**: Agents register via CRDs, eliminating connection management
- **Built-in Persistence**: Kubernetes API Server handles state storage and consistency
- **Native Scaling**: Leverage Kubernetes deployments, services, and horizontal scaling
- **Language Agnostic**: Any language with Kubernetes client libraries can build agents
- **Operational Simplicity**: Use existing Kubernetes tooling for monitoring and debugging
- **Event-Driven**: Kubernetes watch API provides real-time updates without polling

### MCP 2025-06-18 Advanced Features

MCP Conductor provides complete support for the latest MCP specification with advanced features:

- **🔧 Output Schemas**: Comprehensive schema definitions for all 9 tools enabling better validation and IDE integration
- **📊 Structured Content**: Dual-format responses with both human-readable text and programmatic JSON data
- **🔗 Resource Links**: Kubernetes resource URIs for seamless navigation between related resources
- **✅ Input Validation**: Schema-based parameter validation with detailed error feedback (sub-100ms response)
- **🚨 Enhanced Error Handling**: Rich context with timestamps, recovery suggestions, and debugging information
- **💬 Elicitation Support**: Interactive user input during tool execution with multiple prompt types
- **🔄 Protocol Negotiation**: Automatic feature enablement based on client protocol version
- **⚡ Performance**: Optimized with <100ms validation and <5s task execution times

## Quick Start

> **Want to get started immediately?** See our **[QUICKSTART.md](QUICKSTART.md)** for a 5-minute deployment guide.

### Prerequisites

- **Kubernetes cluster** (1.21+)
- **kubectl** configured
- **Kustomize** (for installation via Makefile)
- **Go 1.21+** (for development)
- **Claude Desktop** (macOS/Windows only) or **Augment Code** (any platform)

### Installation

1. **Install CRDs**:
   ```bash
   make install
   ```

2. **Deploy Controller**:
   ```bash
   make deploy
   ```

3. **Create Sample Agent**:
   ```bash
   kubectl apply -f config/samples/sample_agent.yaml
   ```

4. **Submit Sample Task**:
   ```bash
   kubectl apply -f config/samples/sample_task.yaml
   ```

### MCP Client Integration

MCP Conductor supports two integration modes: **local binary** (for Claude Desktop) and **Kubernetes deployment** (for cloud-based clients like Augment Code).

#### Claude Desktop Integration (Local Binary)

Claude Desktop connects to MCP Conductor running as a **local binary** using stdio transport:

```bash
# 1. Build the MCP server binary
go build -o bin/mcp-server ./cmd/mcp-server

# 2. Copy Claude Desktop configuration
# macOS:
cp examples/configs/claude_desktop_config.json \
  ~/Library/Application\ Support/Claude/claude_desktop_config.json

# Windows:
copy examples/configs/claude_desktop_config.json %APPDATA%\Claude\claude_desktop_config.json

# Alternative: Use Claude Desktop Settings → Developer → Edit Config

# 3. Restart Claude Desktop
# The binary will automatically start when Claude needs it
```

**How it works**: Claude Desktop launches the `mcp-server` binary directly and communicates via stdin/stdout.

#### Augment Code Integration (Kubernetes Deployment)

Augment Code connects to MCP Conductor running as a **Kubernetes service** using HTTP transport:

```bash
# 1. Deploy MCP Conductor to your Kubernetes cluster
kubectl apply -f deploy/kubernetes/

# 2. Make the service accessible locally
kubectl port-forward svc/mcp-server 8080:8080 -n mcp-system

# 3. Configure Augment Code IDE
# Use the configuration from examples/configs/augment_mcp_config.json
# Point it to: http://localhost:8080
```

**How it works**: MCP Conductor runs as a Kubernetes service, and Augment Code connects via HTTP to access your cluster's capabilities.

#### Why Two Different Approaches?

- **Claude Desktop**: Runs locally, needs direct binary access to your kubeconfig
- **Augment Code**: Runs in the cloud/remotely, connects to deployed services via HTTP

#### Quick Troubleshooting

**Claude Desktop not connecting?**
- Ensure `bin/mcp-server` has execute permissions: `chmod +x bin/mcp-server`
- Check your kubeconfig is accessible: `kubectl get nodes`
- Verify config file location:
  - **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
  - **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
- Check Claude Desktop logs for errors:
  - **macOS**: `~/Library/Logs/Claude/mcp*.log`
  - **Windows**: `%APPDATA%\Claude\logs\mcp*.log`

**Augment Code not connecting?**
- Ensure port-forward is running: `kubectl get pods -n mcp-system`
- Test HTTP endpoint: `curl http://localhost:8080/healthz`
- Check firewall/network connectivity

## Development

### Setup Environment

```bash
# Using gvm (recommended)
gvm install go1.21.13
gvm use go1.21.13 --default

# Install dependencies
go mod tidy

# Generate code
make generate manifests
```

### Run Locally

```bash
# Terminal 1: Run controller
make run-controller

# Terminal 2: Run agent
make run-agent

# Terminal 3: Run MCP server
go run ./cmd/mcp-server
```

### Available Tools

The MCP server provides 9 Kubernetes tools:

1. **`list_kubernetes_pods`** - List pods in namespace
2. **`list_kubernetes_nodes`** - List cluster nodes
3. **`get_cluster_summary`** - Get cluster overview
4. **`list_kubernetes_deployments`** - List deployments
5. **`list_kubernetes_services`** - List services
6. **`list_kubernetes_daemonsets`** - List daemonsets
7. **`get_kubernetes_pod`** - Get specific pod details
8. **`get_kubernetes_node_status`** - Get node status
9. **`list_kubernetes_namespaces`** - List all namespaces

## Documentation

- **[Quick Start Guide](QUICKSTART.md)** - 5-minute deployment guide
- **[Building Custom Agents](docs/guides/BUILDING_AGENTS.md)** - Create agents in any language
- **[Configuration Guide](docs/guides/CONFIG_FILES.md)** - MCP client configuration
- **[Architecture Overview](docs/architecture/mcp_k8s_architecture.md)** - Detailed architecture
- **[Implementation Plan](docs/architecture/implementation-plan.md)** - Development roadmap
- **[Motivation](docs/architecture/motivation.md)** - Project background

## Multi-Language Agent Development

Agents can be developed in **any language** that supports Kubernetes client libraries. MCP Conductor uses Kubernetes-native registration through CRDs, making it completely language-agnostic.

### Supported Languages

- **Python** - `kubernetes` library
- **Node.js** - `@kubernetes/client-node`
- **Go** - `sigs.k8s.io/controller-runtime`
- **Java** - `io.kubernetes:client-java`
- **C#** - `KubernetesClient`
- **Rust** - `kube` crate
- **Ruby** - `kubeclient`

### Quick Agent Template

Every agent needs to implement these core functions:

1. **Register** - Create Agent CRD in Kubernetes
2. **Watch** - Monitor for assigned Task CRDs
3. **Execute** - Process tasks based on capabilities
4. **Report** - Update task status and send heartbeats

> **Complete Guide**: See **[Building Custom Agents](docs/guides/BUILDING_AGENTS.md)** for full examples in Python, Node.js, and more languages.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Links

- [Model Context Protocol](https://modelcontextprotocol.io)
- [MCP Specification 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18) (Latest)
- [MCP Specification 2025-03-26](https://modelcontextprotocol.io/specification/2025-03-26)
- [Kubernetes](https://kubernetes.io)
- [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)
