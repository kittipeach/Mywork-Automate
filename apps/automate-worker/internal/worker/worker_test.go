package worker

import (
	"strings"
	"testing"

	"github.com/mywork/automate/internal/config"
)

func TestNewBootstrap_ResolvesWiring(t *testing.T) {
	cfg := config.Config{
		Env:              config.EnvDev,
		TemporalHostPort: "temporal:7233",
		FileStore:        config.FileStoreAzureBlob,
	}
	b := NewBootstrap(cfg)
	if b.TemporalHostPort != "temporal:7233" {
		t.Errorf("TemporalHostPort = %q", b.TemporalHostPort)
	}
	if b.TaskQueue != TaskQueue {
		t.Errorf("TaskQueue = %q, want %q", b.TaskQueue, TaskQueue)
	}
	if b.FileStore != config.FileStoreAzureBlob {
		t.Errorf("FileStore = %q", b.FileStore)
	}
}

func TestBootstrap_String(t *testing.T) {
	b := NewBootstrap(config.Config{TemporalHostPort: "localhost:7233", FileStore: config.FileStoreLocal})
	s := b.String()
	for _, want := range []string{TaskQueue, "localhost:7233", "local"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, missing %q", s, want)
		}
	}
}
