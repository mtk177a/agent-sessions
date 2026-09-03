package main

import (
	"context"
	"os"

	"github.com/mtk177a/agent-sessions/internal/buildinfo"
	"github.com/mtk177a/agent-sessions/internal/cli"
	"github.com/mtk177a/agent-sessions/internal/provider"
	"github.com/mtk177a/agent-sessions/internal/provider/codex"
)

func main() {
	runner := cli.Runner{Version: buildinfo.Version, Registry: provider.NewRegistry(codex.New())}
	os.Exit(runner.Run(context.Background(), os.Args[1:], os.Stdout))
}
