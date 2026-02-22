package docker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ListFiles lists files in a host directory by running find inside an alpine container.
// Only files matching the given extensions are returned.
func (d *Docker) ListFiles(ctx context.Context, hostPath string, exts []string) ([]FileInfo, error) {
	// BusyBox find doesn't support -printf, so use find + stat -c instead
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--name", ContainerPrefix+"ls-"+ephemeralSuffix(),
		"-v", hostPath+":/src:ro",
		d.cfg.AlpineImage,
		"sh", "-c", `find /src -maxdepth 1 -type f -exec stat -c '%n	%s' {} +`,
	)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list files: %w (%s)", err, strings.TrimSpace(buf.String()))
	}

	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}

	var files []FileInfo
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		// stat -c '%n' returns full path like /src/video.mkv — strip prefix
		name := strings.TrimPrefix(parts[0], "/src/")
		// Filter by extension
		dot := strings.LastIndex(name, ".")
		if dot < 0 {
			continue
		}
		ext := strings.ToLower(name[dot:])
		if !extSet[ext] {
			continue
		}
		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		files = append(files, FileInfo{Name: name, Size: size})
	}
	return files, nil
}

// CopyFile copies a single file between two host directories using an alpine container.
func (d *Docker) CopyFile(ctx context.Context, srcHostDir, dstHostDir, filename string) error {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--name", ContainerPrefix+"cp-"+ephemeralSuffix(),
		"-v", srcHostDir+":/src:ro",
		"-v", dstHostDir+":/dst",
		d.cfg.AlpineImage,
		"cp", "/src/"+filename, "/dst/"+filename,
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy %s: %w (%s)", filename, err, strings.TrimSpace(buf.String()))
	}

	// Fix ownership
	return d.Chown(ctx, dstHostDir, filename)
}

// CopyFiles copies multiple files between two host directories.
func (d *Docker) CopyFiles(ctx context.Context, srcHostDir, dstHostDir string, filenames []string) (int, error) {
	copied := 0
	for _, f := range filenames {
		if err := d.CopyFile(ctx, srcHostDir, dstHostDir, f); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, nil
}

// PathExists checks if a host path exists and is a directory via an alpine container.
func (d *Docker) PathExists(ctx context.Context, hostPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--name", ContainerPrefix+"pathcheck-"+ephemeralSuffix(),
		"-v", hostPath+":/check:ro",
		d.cfg.AlpineImage,
		"test", "-d", "/check",
	)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// Exit code 1 means test -d failed (not a dir or doesn't exist)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	// Other errors (e.g. docker itself failed, volume mount failed)
	return false, nil
}
