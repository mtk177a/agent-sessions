package main

import (
	"context"
	"os"

	"github.com/mtk177a/agent-sessions/internal/buildinfo"
	"github.com/mtk177a/agent-sessions/internal/cli"
	"github.com/mtk177a/agent-sessions/internal/provider"
	"github.com/mtk177a/agent-sessions/internal/provider/chatgpt"
	"github.com/mtk177a/agent-sessions/internal/provider/claude"
	"github.com/mtk177a/agent-sessions/internal/provider/codex"
)

func main() {
	os.Exit(newRunner().Run(context.Background(), os.Args[1:], os.Stdout))
}

func newRunner() cli.Runner {
	return cli.Runner{Version: buildinfo.Version, Registry: provider.NewRegistry(claude.New(), chatgpt.New(), codex.New())}
}
