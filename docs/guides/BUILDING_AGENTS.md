# Building Custom MCP Agents

Learn how to build your own MCP agents in any programming language that can register with MCP Conductor.

## 🎯 Overview

MCP Conductor uses **Kubernetes-native registration** through Custom Resource Definitions (CRDs). This means agents can be written in **any language** that can interact with the Kubernetes API.

## 🏗️ Agent Architecture

Every MCP agent needs to implement these core functions:

1. **Register** - Create an Agent CRD in Kubernetes
2. **Watch** - Monitor for assigned Task CRDs
3. **Execute** - Process tasks based on capabilities
4. **Report** - Update task status and send heartbeats

## 🚀 Quick Agent Template

### Core Interface (Language Agnostic)

```yaml
# Required Agent Capabilities:
1. Create/Update Agent CRD
2. Watch Task CRDs with field selector
3. Execute tasks based on declared capabilities
4. Update Task status (Running → Completed/Failed)
5. Send periodic heartbeats
```

### Agent CRD Structure

```yaml
apiVersion: mcp.io/v1
kind: Agent
metadata:
  name: my-custom-agent
  namespace: default
spec:
  domain: "my-domain"                    # Domain grouping
  capabilities:                         # What this agent can do
    - "my-capability-1"
    - "my-capability-2"
  resources:                           # Resource requirements
    cpu: "500m"
    memory: "1Gi"
  tags:                               # Additional metadata
    - "my-tag"
  maxConcurrentTasks: 5               # Concurrency limit
  heartbeatInterval: "30s"            # Health check frequency
```

## 🌍 Language Examples

### Python Agent

```python
import kubernetes
from kubernetes import client, config, watch
import json
import time
import threading

class MCPAgent:
    def __init__(self, name, domain, capabilities):
        self.name = name
        self.domain = domain
        self.capabilities = capabilities
        
        # Load Kubernetes config
        try:
            config.load_incluster_config()  # In-cluster
        except:
            config.load_kube_config()       # Local development
            
        self.k8s_client = client.CustomObjectsApi()
        self.core_client = client.CoreV1Api()
        
    def register(self):
        """Register this agent with MCP Conductor"""
        agent_spec = {
            "apiVersion": "mcp.io/v1",
            "kind": "Agent",
            "metadata": {
                "name": self.name,
                "namespace": "default"
            },
            "spec": {
                "domain": self.domain,
                "capabilities": self.capabilities,
                "resources": {"cpu": "500m", "memory": "1Gi"},
                "maxConcurrentTasks": 5,
                "heartbeatInterval": "30s"
            }
        }
        
        try:
            self.k8s_client.create_namespaced_custom_object(
                group="mcp.io", version="v1", namespace="default",
                plural="agents", body=agent_spec
            )
            print(f"Agent {self.name} registered successfully")
        except Exception as e:
            print(f"Registration failed: {e}")
    
    def watch_tasks(self):
        """Watch for assigned tasks"""
        w = watch.Watch()
        for event in w.stream(
            self.k8s_client.list_namespaced_custom_object,
            group="mcp.io", version="v1", namespace="default",
            plural="tasks", field_selector=f"status.assignedAgent={self.name}"
        ):
            task = event['object']
            if event['type'] == 'ADDED' or event['type'] == 'MODIFIED':
                if task['status']['phase'] == 'Pending':
                    threading.Thread(target=self.execute_task, args=(task,)).start()
    
    def execute_task(self, task):
        """Execute a task based on capabilities"""
        task_name = task['metadata']['name']
        capabilities = task['spec']['requiredCapabilities']
        
        # Update status to Running
        self.update_task_status(task_name, "Running", "Task started")
        
        try:
            # Execute based on capability
            result = None
            for capability in capabilities:
                if capability in self.capabilities:
                    result = self.handle_capability(capability, task['spec']['payload'])
                    break
            
            if result:
                self.update_task_status(task_name, "Completed", "Task completed", result)
            else:
                self.update_task_status(task_name, "Failed", "No matching capability")
                
        except Exception as e:
            self.update_task_status(task_name, "Failed", str(e))
    
    def handle_capability(self, capability, payload):
        """Handle specific capability - implement your logic here"""
        if capability == "my-capability-1":
            # Your custom logic here
            return {"result": "capability-1 executed", "data": payload}
        elif capability == "my-capability-2":
            # Your custom logic here
            return {"result": "capability-2 executed", "data": payload}
        return None
    
    def update_task_status(self, task_name, phase, message, result=None):
        """Update task status"""
        status_update = {
            "status": {
                "phase": phase,
                "message": message,
                "lastUpdated": time.strftime("%Y-%m-%dT%H:%M:%SZ")
            }
        }
        if result:
            status_update["status"]["result"] = result
            
        self.k8s_client.patch_namespaced_custom_object_status(
            group="mcp.io", version="v1", namespace="default",
            plural="tasks", name=task_name, body=status_update
        )
    
    def start_heartbeat(self):
        """Send periodic heartbeats"""
        def heartbeat():
            while True:
                try:
                    self.k8s_client.patch_namespaced_custom_object_status(
                        group="mcp.io", version="v1", namespace="default",
                        plural="agents", name=self.name,
                        body={"status": {"lastHeartbeat": time.strftime("%Y-%m-%dT%H:%M:%SZ")}}
                    )
                except Exception as e:
                    print(f"Heartbeat failed: {e}")
                time.sleep(30)
        
        threading.Thread(target=heartbeat, daemon=True).start()
    
    def run(self):
        """Start the agent"""
        self.register()
        self.start_heartbeat()
        print(f"Agent {self.name} starting task watcher...")
        self.watch_tasks()

# Usage
if __name__ == "__main__":
    agent = MCPAgent(
        name="python-agent",
        domain="data-processing", 
        capabilities=["my-capability-1", "my-capability-2"]
    )
    agent.run()
```

### Node.js Agent

```javascript
const k8s = require('@kubernetes/client-node');

class MCPAgent {
    constructor(name, domain, capabilities) {
        this.name = name;
        this.domain = domain;
        this.capabilities = capabilities;
        
        const kc = new k8s.KubeConfig();
        kc.loadFromDefault();
        this.k8sApi = kc.makeApiClient(k8s.CustomObjectsApi);
    }
    
    async register() {
        const agentSpec = {
            apiVersion: 'mcp.io/v1',
            kind: 'Agent',
            metadata: { name: this.name, namespace: 'default' },
            spec: {
                domain: this.domain,
                capabilities: this.capabilities,
                resources: { cpu: '500m', memory: '1Gi' },
                maxConcurrentTasks: 5,
                heartbeatInterval: '30s'
            }
        };
        
        try {
            await this.k8sApi.createNamespacedCustomObject(
                'mcp.io', 'v1', 'default', 'agents', agentSpec
            );
            console.log(`Agent ${this.name} registered`);
        } catch (error) {
            console.error('Registration failed:', error);
        }
    }
    
    async watchTasks() {
        // Implementation similar to Python example
        // Use k8s.Watch to monitor tasks
    }
    
    async executeTask(task) {
        // Implementation similar to Python example
    }
    
    async run() {
        await this.register();
        this.startHeartbeat();
        console.log(`Agent ${this.name} starting...`);
        await this.watchTasks();
    }
}

// Usage
const agent = new MCPAgent(
    'nodejs-agent',
    'integration',
    ['http-request', 'webhook-call']
);
agent.run();
```

## 🚀 Quick Start Your Agent

### 1. Choose Your Language
Pick any language with Kubernetes client library support:
- **Python**: `kubernetes` library
- **Node.js**: `@kubernetes/client-node`
- **Go**: `sigs.k8s.io/controller-runtime/pkg/client`
- **Java**: `io.kubernetes:client-java`
- **C#**: `KubernetesClient`
- **Rust**: `kube` crate

### 2. Implement Core Functions
```bash
# Required functions:
1. register()           # Create Agent CRD
2. watchTasks()         # Monitor assigned tasks
3. executeTask()        # Process tasks
4. updateTaskStatus()   # Report results
5. startHeartbeat()     # Health monitoring
```

### 3. Deploy Your Agent
```bash
# Option A: As Kubernetes Deployment
kubectl apply -f my-agent-deployment.yaml

# Option B: As standalone process
./my-agent --kubeconfig ~/.kube/config
```

### 4. Test Your Agent
```bash
# Check agent registration
kubectl get agents

# Submit test task
kubectl apply -f - <<EOF
apiVersion: mcp.io/v1
kind: Task
metadata:
  name: test-task
spec:
  domain: "my-domain"
  requiredCapabilities: ["my-capability-1"]
  payload: {"test": "data"}
EOF

# Check task execution
kubectl get tasks test-task -o yaml
```

## 🎯 Best Practices

### Error Handling
- Always update task status on failure
- Include meaningful error messages
- Implement retry logic for transient failures

### Resource Management
- Respect `maxConcurrentTasks` limit
- Clean up resources after task completion
- Monitor memory and CPU usage

### Security
- Use proper RBAC permissions
- Validate task payloads
- Sanitize inputs and outputs

### Monitoring
- Send regular heartbeats
- Log important events
- Expose metrics if possible

## 🔗 Next Steps

- **[MCP Conductor Examples](../../internal/agent/examples/)** - Go agent examples
- **[Kubernetes Client Libraries](https://kubernetes.io/docs/reference/using-api/client-libraries/)** - Official client libraries
- **[CRD Documentation](../architecture/mcp_k8s_architecture.md)** - Understanding the data model

---

**🎉 Start building your custom MCP agent and extend your AI's capabilities!**
