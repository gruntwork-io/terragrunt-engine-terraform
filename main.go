package main

import (
	"os"

	tgengine "github.com/gruntwork-io/terragrunt-engine-go/engine"
	"github.com/gruntwork-io/terragrunt-engine-terraform/engine"
	"github.com/sirupsen/logrus"

	"github.com/hashicorp/go-plugin"
)

const (
	engineLogLevelEnv     = "TG_ENGINE_LOG_LEVEL"
	defaultEngineLogLevel = "INFO"
)

func main() {
	engineLogLevel := os.Getenv(engineLogLevelEnv)
	if engineLogLevel == "" {
		engineLogLevel = defaultEngineLogLevel
	}

	parsedLevel, err := logrus.ParseLevel(engineLogLevel)
	if err != nil {
		logrus.Warnf("Error parsing log level: %v", err)
		parsedLevel = logrus.InfoLevel
	}

	logrus.SetLevel(parsedLevel)

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "engine",
			MagicCookieValue: "terragrunt",
		},
		Plugins: map[string]plugin.Plugin{
			"terraform": &tgengine.TerragruntGRPCEngine{Impl: &engine.TerraformEngine{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
