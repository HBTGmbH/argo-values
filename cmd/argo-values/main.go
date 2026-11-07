package main

import (
	"argo-values/internal/logger"
	"os"

	"github.com/alecthomas/kong"
)

// Global holds the flags that are shared by all commands.
type Global struct {
	LogLevel        string `name:"log-level" short:"l" env:"LOG_LEVEL" default:"debug" help:"Log level (debug, info, warn, error, fatal, panic)"`
	LogFormat       string `name:"log-format" env:"LOG_FORMAT" default:"json" help:"Log format (text, json)"`
	LogOutput       string `name:"log-output" env:"LOG_OUTPUT" default:"stderr" help:"Log output (stderr, stdout)"`
	KubeConfigPath  string `name:"kubeconfig" short:"k" help:"Path to kubeconfig file"`
	ValuesFileName  string `name:"file-values" short:"v" default:"app-values.yaml" help:"Path to values file"`
	TargetDirectory string `name:"file-target" short:"t" default:"." help:"Path to application files"`
	DryRun          bool   `name:"dry-run" short:"d" help:"dry-run enabled (true, false)"`
}

type CLI struct {
	Global Global `embed:""`

	Discover DiscoverCmd `cmd:"" help:"Validates Argo values CMP plugin usage for an application"`
	Generate GenerateCmd `cmd:"" help:"Generate Argo application resources using Helm"`
	Init     InitCmd     `cmd:"" help:"Initialize Argo application values from ConfigMaps and Secrets"`
	Watch    WatchCmd    `cmd:"" help:"Watch Kubernetes resources and refresh Argo applications"`

	Help HelpCmd `cmd:"" default:"1" hidden:"" help:"Show help."`
}

type HelpCmd struct{}

// Run prints the root help when no subcommand was given.
func (HelpCmd) Run(ctx *kong.Context) error {
	rootCtx := &kong.Context{
		Kong: ctx.Kong,
		Path: []*kong.Path{{App: ctx.Kong.Model}},
	}
	return kong.DefaultHelpPrinter(kong.HelpOptions{ValueFormatter: kong.DefaultHelpValueFormatter}, rootCtx)
}

func New(commandLine *CLI) (*kong.Kong, error) {
	return kong.New(commandLine,
		kong.Name("argo-values"),
		kong.Description("A CLI tool for managing Argo application with values from additional resources like ConfigMaps and Secrets"),
	)
}

func main() {
	var commandLine CLI

	parser, err := New(&commandLine)
	if err != nil {
		logger.Fatalf("Failed to initialize CLI: %v", err)
	}

	ctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	logger.Init(commandLine.Global.LogLevel, commandLine.Global.LogFormat, commandLine.Global.LogOutput)

	if err := ctx.Run(&commandLine.Global); err != nil {
		logger.Fatalf("%v", err)
	}
}
