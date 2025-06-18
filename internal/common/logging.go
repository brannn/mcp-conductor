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

package common

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// LoggerConfig holds configuration for structured logging
type LoggerConfig struct {
	// Development enables development mode logging (human-readable, extra stack traces)
	Development bool

	// Level sets the log level (debug, info, warn, error)
	Level string

	// Format sets the log format (json, console)
	Format string

	// Component is the component name to include in logs
	Component string

	// Domain is the domain name to include in logs
	Domain string
}

// SetupLogger configures and returns a structured logger
func SetupLogger(config LoggerConfig) logr.Logger {
	opts := zap.Options{
		Development: config.Development,
	}

	// Set log level - controller-runtime zap uses different level setting
	// We'll use the development flag and let controller-runtime handle levels

	// Configure encoder
	if config.Format == "console" {
		opts.Development = true
	}

	logger := zap.New(zap.UseFlagOptions(&opts))

	// Add component and domain context
	if config.Component != "" {
		logger = logger.WithValues("component", config.Component)
	}
	if config.Domain != "" {
		logger = logger.WithValues("domain", config.Domain)
	}

	// Set as the global logger for controller-runtime
	log.SetLogger(logger)

	return logger
}

// NewLoggerConfig creates a logger configuration from environment variables
func NewLoggerConfig(component string) LoggerConfig {
	return LoggerConfig{
		Development: getEnvBoolOrDefault("LOG_DEVELOPMENT", false),
		Level:       getEnvOrDefault("LOG_LEVEL", "info"),
		Format:      getEnvOrDefault("LOG_FORMAT", "json"),
		Component:   component,
		Domain:      getEnvOrDefault("LOG_DOMAIN", ""),
	}
}

// LoggerFromContext returns a logger from the context with additional values
func LoggerFromContext(ctx context.Context, keysAndValues ...interface{}) logr.Logger {
	return log.FromContext(ctx).WithValues(keysAndValues...)
}

// LoggerWithValues returns a logger with additional key-value pairs
func LoggerWithValues(logger logr.Logger, keysAndValues ...interface{}) logr.Logger {
	return logger.WithValues(keysAndValues...)
}

// LoggerWithName returns a logger with a name suffix
func LoggerWithName(logger logr.Logger, name string) logr.Logger {
	return logger.WithName(name)
}

// ContextWithLogger returns a context with the logger embedded
func ContextWithLogger(ctx context.Context, logger logr.Logger) context.Context {
	return log.IntoContext(ctx, logger)
}

// SetupControllerLogger sets up logging for the controller
func SetupControllerLogger() logr.Logger {
	config := NewLoggerConfig("controller")
	return SetupLogger(config)
}

// SetupAgentLogger sets up logging for an agent
func SetupAgentLogger(agentName string) logr.Logger {
	config := NewLoggerConfig("agent")
	logger := SetupLogger(config)
	return logger.WithValues("agent", agentName)
}

// LogError logs an error with context
func LogError(logger logr.Logger, err error, msg string, keysAndValues ...interface{}) {
	logger.Error(err, msg, keysAndValues...)
}

// LogInfo logs an info message with context
func LogInfo(logger logr.Logger, msg string, keysAndValues ...interface{}) {
	logger.Info(msg, keysAndValues...)
}

// LogDebug logs a debug message with context
func LogDebug(logger logr.Logger, msg string, keysAndValues ...interface{}) {
	logger.V(1).Info(msg, keysAndValues...)
}

// LogWarning logs a warning message with context
func LogWarning(logger logr.Logger, msg string, keysAndValues ...interface{}) {
	logger.V(0).Info("WARNING: "+msg, keysAndValues...)
}

// GetLogLevel returns the current log level from environment
func GetLogLevel() string {
	return getEnvOrDefault("LOG_LEVEL", "info")
}

// IsDebugEnabled returns true if debug logging is enabled
func IsDebugEnabled() bool {
	level := GetLogLevel()
	return level == "debug"
}

// IsDevelopmentMode returns true if development mode is enabled
func IsDevelopmentMode() bool {
	return getEnvBoolOrDefault("LOG_DEVELOPMENT", false) ||
		getEnvOrDefault("ENVIRONMENT", "") == "development" ||
		os.Getenv("KUBERNETES_SERVICE_HOST") == "" // Not running in cluster
}
