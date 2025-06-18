# MCP Conductor Quick Start

Get MCP Conductor running in your Kubernetes cluster in **5 minutes**.

## Prerequisites

- Kubernetes cluster (local or remote)
- `kubectl` configured
- Docker (for building images)
- Go 1.21+ (for building)

## 🚀 Quick Deploy (6 Steps)

### 1. Clone and Build
```bash
git clone https://github.com/your-org/mcp-conductor.git
cd mcp-conductor
go mod tidy
make build
```

### 2. Build and Load Images

**Option A: Local Development (Recommended)**
```bash
# Build all container images
docker build -t mcp-conductor/controller:latest -f Dockerfile.controller .
docker build -t mcp-conductor/agent:latest -f Dockerfile.agent .
docker build -t mcp-conductor/mcp-server:latest -f Dockerfile.mcp-server .

# Load images into your local cluster
# Choose the command for your environment:

# Minikube:
minikube image load mcp-conductor/controller:latest mcp-conductor/agent:latest mcp-conductor/mcp-server:latest

# Kind:
kind load docker-image mcp-conductor/controller:latest mcp-conductor/agent:latest mcp-conductor/mcp-server:latest

# Docker Desktop Kubernetes: No additional step needed
```

**Option B: Production**
```bash
# Build and push to your registry
docker build -t your-registry/mcp-conductor/controller:latest -f Dockerfile.controller .
docker push your-registry/mcp-conductor/controller:latest
# (repeat for agent and mcp-server)

# Update image references in deploy/kubernetes/deployment.yaml
```

### 3. Deploy to Kubernetes
```bash
# Install CRDs
kubectl apply -f config/crd/bases/

# Deploy controller and MCP server
kubectl apply -f deploy/kubernetes/
```

### 4. Verify Deployment
```bash
# Check all components are running
kubectl get pods -n mcp-system

# Should see:
# mcp-controller-xxx     1/1 Running
# mcp-server-xxx         1/1 Running
```

### 5. Deploy Infrastructure Agent
```bash
# Deploy the working infrastructure agent
kubectl apply -f config/samples/sample_agent.yaml

# Verify agent is registered and ready
kubectl get agents
```

**What this creates:**
- **Infrastructure Agent**: Provides Kubernetes cluster introspection capabilities (list pods, namespaces, nodes, deployments, etc.)

*Note: Additional agent examples are available in `docs/examples/agent-definitions.yaml` but require custom implementations.*

### 6. Connect MCP Client

#### Option A: Augment Code (HTTP Transport)
```bash
# Port forward MCP server
kubectl port-forward svc/mcp-server 8080:8080 -n mcp-system

# Configure Augment Code with:
# URL: http://localhost:8080
# Protocol: 2025-03-26
# Transport: HTTP
```

**Why HTTP works**: Augment Code supports HTTP Streamable transport and can connect to remote MCP servers.

#### Option B: Claude Desktop (Local Binary - stdio Transport)
```bash
# Build local binary
go build -o bin/mcp-server ./cmd/mcp-server

# Copy config (macOS)
cp examples/configs/claude_desktop_config.json \
  ~/Library/Application\ Support/Claude/claude_desktop_config.json

# Restart Claude Desktop
```

**Why local binary is required**: Claude Desktop only supports stdio transport (stdin/stdout) and cannot connect to HTTP MCP servers. It must launch the MCP server as a local process.

## ✅ Test Your Setup

### Option 1: Test with kubectl (Quick Verification)
```bash
# Create a test task to verify the system works
kubectl apply -f config/samples/test_task.yaml

# Watch the task get assigned and completed
kubectl get tasks -w

# Check the results
kubectl get task test-list-namespaces -o yaml
```

### Option 2: Test with MCP Host (Full Experience)
1. Open your MCP host (Augment Code IDE or Claude Desktop)
2. Try these example queries to test the sample infrastructure agent:
   - "List all Kubernetes namespaces in my cluster"
   - "What pods are running in the default namespace?"
   - "Show me the status of my cluster nodes"
   - "What deployments do I have running?"

**Expected behavior:**
- Your AI assistant will use the sample agent's capabilities to query your cluster
- You'll see MCP Conductor orchestrating tasks between the MCP server and agents
- The infrastructure agent will return real-time cluster information

## 🛠️ Available Tools

Once connected, you'll have access to 9 Kubernetes tools:

- `list_kubernetes_namespaces` - List all namespaces
- `list_kubernetes_pods` - List pods in namespace
- `list_kubernetes_nodes` - List cluster nodes
- `get_cluster_summary` - Get cluster overview
- `list_kubernetes_deployments` - List deployments
- `list_kubernetes_services` - List services
- `list_kubernetes_daemonsets` - List daemonsets
- `get_kubernetes_pod` - Get specific pod details
- `get_kubernetes_node_status` - Get node status

## 🔧 Quick Troubleshooting

### Pods not starting?
```bash
# Check controller logs
kubectl logs -n mcp-conductor-system deployment/mcp-conductor-controller-manager

# Check MCP server logs
kubectl logs -n mcp-system deployment/mcp-server
```

### MCP client not connecting?
```bash
# Test HTTP endpoint
curl http://localhost:8080/healthz

# Check port forward
kubectl get svc -n mcp-system

# Verify MCP protocol
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "initialize", "id": 1, "params": {}}'
```

### Agent not registering?
```bash
# Check agent status
kubectl get agents -o wide

# Check agent logs (if running as pod)
kubectl logs -l app=mcp-conductor,component=agent
```

## 🎯 What's Next?

- **[Full Documentation](README.md)** - Complete setup guide
- **[Configuration Guide](docs/guides/CONFIG_FILES.md)** - MCP client configs
- **[Architecture Overview](docs/architecture/mcp_k8s_architecture.md)** - How it works
- **[Build Custom Agents](docs/guides/BUILDING_AGENTS.md)** - Create agents in any language

## 💡 Quick Tips

- **Local Development**: Use `make run-controller` and `make run-agent` for local testing
- **Custom Agents**: Check `internal/agent/examples/` for agent templates
- **Multiple Domains**: Deploy separate instances for different use cases
- **Production**: Use proper RBAC and resource limits in production

---

**🎉 You're now running MCP Conductor! Start asking your AI assistant about your Kubernetes cluster.**
