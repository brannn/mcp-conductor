/*
Copyright 2024 MCP Conductor Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
	"github.com/mcp-conductor/mcp-conductor/internal/common"
	"github.com/mcp-conductor/mcp-conductor/internal/mcp"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(mcpv1.AddToScheme(scheme))
}

func main() {
	var configFile string
	var httpMode bool
	var port int

	flag.StringVar(&configFile, "config", "",
		"The server will load its initial configuration from this file.")
	flag.BoolVar(&httpMode, "http", false,
		"Run in HTTP mode instead of stdio mode")
	flag.IntVar(&port, "port", 8080,
		"Port to listen on in HTTP mode")
	flag.Parse()

	// Setup logging
	logger := common.SetupAgentLogger("mcp-server")

	if httpMode {
		logger.Info("Starting MCP Server in HTTP mode",
			"version", "v0.1.0",
			"port", port,
			"description", "Kubernetes-native MCP orchestration server")
	} else {
		logger.Info("Starting MCP Server in stdio mode",
			"version", "v0.1.0",
			"description", "Kubernetes-native MCP orchestration server")
	}

	// Create Kubernetes client
	k8sConfig, err := common.GetKubernetesConfig()
	if err != nil {
		logger.Error(err, "Failed to get Kubernetes config")
		os.Exit(1)
	}

	k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes client")
		os.Exit(1)
	}

	// Create MCP server
	server := mcp.NewServer(k8sClient, logger)
	if httpMode {
		server.Port = port
	}

	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start MCP server
	if httpMode {
		if err := server.StartHTTP(ctx); err != nil {
			logger.Error(err, "Failed to start HTTP MCP server")
			os.Exit(1)
		}
	} else {
		if err := server.StartStdio(ctx); err != nil {
			logger.Error(err, "Failed to start stdio MCP server")
			os.Exit(1)
		}
	}

	logger.Info("MCP Server started successfully")

	// Keep the server running
	select {}
}
