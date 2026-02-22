package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"anime-upscaling/internal/config"
	"anime-upscaling/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "upscale":
		err = cmdUpscale(ctx)
	case "optimize":
		err = cmdOptimize(ctx)
	case "pipeline":
		err = cmdPipeline(ctx)
	case "check":
		err = cmdCheck(ctx, os.Args[2:])
	case "stop":
		err = cmdStop(ctx)
	case "logs":
		err = cmdLogs(ctx)
	case "serve":
		err = server.CmdServe(config.NewConfig())
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: animeup <command>

Commands:
  upscale    Upscale videos with video2x using 2 GPUs in parallel
  optimize   Compress videos with ffmpeg H.265 (sequential)
  pipeline   Upscale (GPU) + compress (CPU) in parallel pipeline
  check      Verify video integrity (ffprobe + decode)
  stop       Stop all running Docker containers
  logs       Tail all log files with colors
  serve      Start HTTP API server`)
}
