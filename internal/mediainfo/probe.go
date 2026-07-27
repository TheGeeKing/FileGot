package mediainfo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

var probeCache = struct {
	sync.Mutex
	values map[cacheKey]Metadata
}{values: make(map[cacheKey]Metadata)}

func Probe(ctx context.Context, executable, path string) (Metadata, error) {
	if executable == "" {
		executable = "mediainfo"
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
	var stdout, stderr limitedBuffer
	command := exec.CommandContext(timeout, executable, "--Output=JSON", absolute)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if timeout.Err() != nil {
			return Metadata{}, fmt.Errorf("MediaInfo timed out: %w", timeout.Err())
		}
		if stdout.overflow || stderr.overflow {
			return Metadata{}, fmt.Errorf("MediaInfo output exceeds %d bytes", MaxOutputBytes)
		}
		if message := bytes.TrimSpace(stderr.Bytes()); len(message) > 0 {
			return Metadata{}, fmt.Errorf("run MediaInfo: %w: %s", err, message)
		}
		return Metadata{}, fmt.Errorf("run MediaInfo: %w", err)
	}
	if stdout.overflow || stderr.overflow {
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
	if executable == "" {
		executable = "mediainfo"
	}
	timeout, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	command := exec.CommandContext(timeout, executable, "--Version")
	var output limitedBuffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return fmt.Errorf("run MediaInfo: %w", err)
	}
	if output.overflow {
		return fmt.Errorf("MediaInfo output exceeds %d bytes", MaxOutputBytes)
	}
	return nil
}

type limitedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	remaining := MaxOutputBytes - buffer.Len()
	if remaining <= 0 {
		buffer.overflow = true
		return len(data), nil
	}
	if len(data) > remaining {
		buffer.overflow = true
		_, _ = buffer.Buffer.Write(data[:remaining])
		return len(data), nil
	}
	return buffer.Buffer.Write(data)
}
