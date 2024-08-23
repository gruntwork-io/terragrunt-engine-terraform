package main

import (
	tgengine "github.com/gruntwork-io/terragrunt-engine-go/engine"
	"github.com/gruntwork-io/terragrunt-engine-terraform/engine"

	"github.com/hashicorp/go-plugin"
)

func main() {
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
