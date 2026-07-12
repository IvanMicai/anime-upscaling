package main

import (
	"context"
	"fmt"
	"os/exec"
)

func cmdStop(ctx context.Context) error {
	fmt.Println("Parando processos video2x e ffmpeg...")

	killed := 0
	for _, name := range []string{"video2x", "ffmpeg"} {
		cmd := exec.CommandContext(ctx, "pkill", "-f", name)
		if err := cmd.Run(); err == nil {
			killed++
		}
	}

	if killed == 0 {
		fmt.Println("Nenhum processo rodando.")
	} else {
		fmt.Println("Processos parados.")
	}
	return nil
}
