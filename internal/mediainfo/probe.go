package mediainfo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const probeTimeout = 15 * time.Second

type cacheKey struct {
	executable string
	path       string
	size       int64
	modified   int64
}

type outputBudget struct {
	sync.Mutex
	remaining int
	overflow  bool
}

var probeCache = struct {
	sync.Mutex
	values map[cacheKey]Metadata
}{values: make(map[cacheKey]Metadata)}

func Probe(ctx context.Context, executable, path string) (Metadata, error) {
	executable, err := ResolveExecutable(executable)
	if err != nil {
		return Metadata{}, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("resolve media path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Metadata{}, fmt.Errorf("inspect media file: %w", err)
	}
	key := cacheKey{
		executable: executable, path: absolute,
		size: info.Size(), modified: info.ModTime().UnixNano(),
	}
	probeCache.Lock()
	cached, ok := probeCache.values[key]
	probeCache.Unlock()
	if ok {
		return cached, nil
	}

	timeout, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	budget := &outputBudget{remaining: MaxOutputBytes}
	stdout := limitedBuffer{budget: budget}
	stderr := limitedBuffer{budget: budget}
	command := exec.CommandContext(timeout, executable, "--Output=JSON", absolute)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return Metadata{}, fmt.Errorf("MediaInfo cancelled: %w", ctx.Err())
		}
		if timeout.Err() != nil {
			return Metadata{}, fmt.Errorf("MediaInfo timed out: %w", timeout.Err())
		}
		if budget.overflow {
			return Metadata{}, fmt.Errorf("MediaInfo output exceeds %d bytes", MaxOutputBytes)
		}
		if message := bytes.TrimSpace(stderr.Bytes()); len(message) > 0 {
			return Metadata{}, fmt.Errorf("run MediaInfo: %w: %s", err, message)
		}
		return Metadata{}, fmt.Errorf("run MediaInfo: %w", err)
	}
	if budget.overflow {
		return Metadata{}, fmt.Errorf("MediaInfo output exceeds %d bytes", MaxOutputBytes)
	}
	result, err := DecodeBytes(stdout.Bytes())
	if err != nil {
		return Metadata{}, err
	}
	probeCache.Lock()
	probeCache.values[key] = result
	probeCache.Unlock()
	return result, nil
}

func TestExecutable(ctx context.Context, executable string) error {
	executable, err := ResolveExecutable(executable)
	if err != nil {
		return err
	}
	timeout, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	command := exec.CommandContext(timeout, executable, "--Version")
	budget := &outputBudget{remaining: MaxOutputBytes}
	stdout := limitedBuffer{budget: budget}
	stderr := limitedBuffer{budget: budget}
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run MediaInfo: %w", err)
	}
	if budget.overflow {
		return fmt.Errorf("MediaInfo output exceeds %d bytes", MaxOutputBytes)
	}
	return nil
}

func ResolveExecutable(configured string) (string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate FileGot executable: %w", err)
	}
	return resolveExecutable(configured, filepath.Dir(current), exec.LookPath, regularFile)
}

func resolveExecutable(
	configured, applicationDir string,
	lookPath func(string) (string, error),
	exists func(string) bool,
) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		path, err := lookPath(configured)
		if err != nil {
			return "", fmt.Errorf("configured MediaInfo executable %q: %w", configured, err)
		}
		return path, nil
	}
	if path, err := lookPath("mediainfo"); err == nil {
		return path, nil
	}
	bundled := filepath.Join(applicationDir, "tools", executableName())
	if exists(bundled) {
		return bundled, nil
	}
	return "", fmt.Errorf("MediaInfo CLI not found in settings, PATH, or %s", bundled)
}

func executableName() string {
	if runtime.GOOS == "windows" {
		return "MediaInfo.exe"
	}
	return "mediainfo"
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

type limitedBuffer struct {
	bytes.Buffer
	budget *outputBudget
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	buffer.budget.Lock()
	defer buffer.budget.Unlock()
	if buffer.budget.remaining <= 0 {
		buffer.budget.overflow = true
		return len(data), nil
	}
	if len(data) > buffer.budget.remaining {
		buffer.budget.overflow = true
		_, _ = buffer.Buffer.Write(data[:buffer.budget.remaining])
		buffer.budget.remaining = 0
		return len(data), nil
	}
	buffer.budget.remaining -= len(data)
	return buffer.Buffer.Write(data)
}
