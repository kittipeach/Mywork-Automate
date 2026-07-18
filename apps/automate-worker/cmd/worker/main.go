// Command worker runs the automate-worker Temporal worker (docs/spec/04 §2.3).
// This is the composition root: it wires the real dependencies (pgx pool,
// masking, secrets, the interpreter activities) and serves the task queue.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pkg/sftp"
	"go.temporal.io/sdk/client"
	temporalworker "go.temporal.io/sdk/worker"
	"golang.org/x/crypto/ssh"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
	"github.com/mywork/automate/apps/automate-worker/internal/interpreter"
	"github.com/mywork/automate/apps/automate-worker/internal/pgxquerier"
	"github.com/mywork/automate/apps/automate-worker/internal/worker"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/pkg/filestore"
	"github.com/mywork/automate/pkg/logscrub"
	mailersmtp "github.com/mywork/automate/pkg/mailer/smtp"
	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/obs"
	"github.com/mywork/automate/pkg/secrets"
)

// connPools caches one pgx pool per external-connection DSN so db.query nodes
// that target a customer database reuse connections across runs. Safe for
// concurrent use by parallel activities.
type connPools struct {
	mu      sync.Mutex
	pools   map[string]*pgxpool.Pool
	timeout time.Duration
}

func newConnPools(timeout time.Duration) *connPools {
	return &connPools{pools: map[string]*pgxpool.Pool{}, timeout: timeout}
}

// openQuerier returns a Querier for dsn, creating (and caching) its pool on
// first use. It satisfies dbquery.Deps.OpenQuerier.
func (p *connPools) openQuerier(ctx context.Context, dsn string) (dbquery.Querier, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pool, ok := p.pools[dsn]
	if !ok {
		np, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("open external pool: %w", err)
		}
		p.pools[dsn] = np
		pool = np
	}
	return pgxquerier.New(pool, p.timeout), nil
}

func (p *connPools) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pool := range p.pools {
		pool.Close()
	}
}

const (
	// stmtTimeout bounds a single db.query statement (FR-DB-007); the
	// interpreter's per-activity StartToCloseTimeout is the outer Temporal bound.
	stmtTimeout = 120 * time.Second
	// secretsFile is the dev/local secret store read by the file resolver. In
	// SIT+ the Key Vault resolver (Workload Identity) slots in behind the same
	// caching resolver.
	secretsFile = "secrets.local.yaml"
	// defaultFileDir/defaultSMTPAddr are the dev defaults for the file/delivery
	// deps (docker-compose ships azurite + mailhog on these ports).
	defaultFileDir  = "/tmp/automate-files"
	defaultSMTPAddr = "localhost:1025"
	// defaultFrom is the envelope sender for delivery.email in dev.
	defaultFrom = "automate@mywork.local"
)

// getenv returns the env var or a default when unset/blank.
func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func main() {
	// JSON logs (E11-S3, ELK) with secret scrubbing (spec 07 §4).
	logger := slog.New(logscrub.NewHandler(slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(logger)

	// OpenTelemetry tracing (E11-S1). Stdout exporter by default; a collector
	// sidecar forwards to App Insights. Non-fatal on error.
	if shutdown, err := obs.Init(context.Background(), "automate-worker"); err != nil {
		logger.Warn("otel init failed", "err", err)
	} else {
		defer func() { _ = shutdown(context.Background()) }()
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}
	logger.Info("automate-worker bootstrap", "summary", worker.NewBootstrap(cfg).String())

	if err := run(cfg, logger); err != nil {
		logger.Error("worker exited", "err", err)
		os.Exit(1)
	}
}

// run wires the real dependencies and serves the automate task queue until the
// process is interrupted. Every failure is wrapped with context; Temporal being
// unavailable returns a clear error rather than panicking.
func run(cfg config.Config, logger *slog.Logger) error {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("build pgx pool: %w", err)
	}
	defer pool.Close()
	querier := pgxquerier.New(pool, stmtTimeout)

	maskingEngine, err := masking.NewEngine(masking.DefaultRules())
	if err != nil {
		return fmt.Errorf("build masking engine: %w", err)
	}

	fileResolver, err := secrets.NewFileResolver(secretsFile)
	if err != nil {
		return fmt.Errorf("build secret resolver: %w", err)
	}
	resolver := secrets.NewCachingResolver(fileResolver, 5*time.Minute, nil)

	// File/delivery deps (spec 08 E7-S1, E8). LocalFileStore is the dev/PVC
	// backend; the SMTP sender targets mailhog; MFT is a lazily-dialled SFTP
	// client (nil endpoint until a connection is configured — delivery.mft then
	// errors clearly rather than panicking).
	fileStore, err := filestore.NewLocalFileStore(getenv("FILE_DIR", defaultFileDir))
	if err != nil {
		return fmt.Errorf("build file store: %w", err)
	}
	sender := mailersmtp.New(getenv("SMTP_ADDR", defaultSMTPAddr), getenv("SMTP_FROM", defaultFrom))
	mft := newSFTPClient(getenv("SFTP_ADDR", ""), getenv("SFTP_USER", ""), os.Getenv("SFTP_PASSWORD"))

	// Per-connection pool cache: db.query nodes that target an external database
	// (E6-S1) dial it through here, one cached pool per DSN.
	extPools := newConnPools(stmtTimeout)
	defer extPools.closeAll()

	activities := interpreter.NewActivities(
		dbquery.Deps{
			Secrets:     resolver,
			Querier:     querier,
			Masking:     maskingEngine,
			MaskPoint:   masking.PointPreview,
			OpenQuerier: extPools.openQuerier,
		},
		interpreter.WithFileStore(fileStore),
		interpreter.WithMailer(sender),
		interpreter.WithMFT(mft),
	)

	c, err := client.Dial(client.Options{HostPort: cfg.TemporalHostPort, Namespace: worker.Namespace})
	if err != nil {
		return fmt.Errorf("dial temporal at %s: %w", cfg.TemporalHostPort, err)
	}
	defer c.Close()

	w := temporalworker.New(c, worker.TaskQueue, temporalworker.Options{})
	w.RegisterWorkflow(interpreter.FlowWorkflow)
	w.RegisterActivity(activities)

	logger.Info("automate-worker serving", "taskQueue", worker.TaskQueue, "temporal", cfg.TemporalHostPort)
	if err := w.Run(temporalworker.InterruptCh()); err != nil {
		return fmt.Errorf("run task queue %s: %w", worker.TaskQueue, err)
	}
	return nil
}

// sftpClient is the thin SFTP adapter implementing interpreter.MFTClient
// (spec 08 E8-S1). It dials lazily on the first Upload, then uploads atomically:
// the bytes are written to "<remotePath>.tmp" and renamed onto the final path
// only after a clean close, so a partial transfer never leaves a half-written
// file at the destination. Parent directories are created (mkdir -p).
//
// This is a thin network adapter with no unit tests: it can only be exercised
// against a live SFTP endpoint (the docker-compose sftp-mock / a real MFT) in
// the E8-S1 integration test. The atomic/mkdir/idempotency logic that matters is
// small and lives here; the executor-side path sanitisation is unit-tested.
type sftpClient struct {
	addr     string // "host:port"; empty => not configured
	user     string
	password string
}

// newSFTPClient builds an SFTP client for addr with password auth. An empty addr
// yields a client whose Upload returns a clear "not configured" error, so
// delivery.mft fails gracefully in environments without an MFT endpoint.
func newSFTPClient(addr, user, password string) *sftpClient {
	return &sftpClient{addr: addr, user: user, password: password}
}

// Upload writes r to remotePath atomically (see type doc). It dials a fresh SSH
// connection per call — deliveries are infrequent and this keeps the client
// stateless; a pooled connection is a later optimisation.
func (c *sftpClient) Upload(_ context.Context, remotePath string, r io.Reader) error {
	if strings.TrimSpace(c.addr) == "" {
		return fmt.Errorf("mft: no SFTP endpoint configured (set SFTP_ADDR)")
	}

	sshCfg := &ssh.ClientConfig{
		User: c.user,
		Auth: []ssh.AuthMethod{ssh.Password(c.password)},
		// TODO(E8-S1 integration): pin the host key from Key Vault
		// (ssh.FixedHostKey) instead of trusting on first use. InsecureIgnoreHostKey
		// is only acceptable against the sftp-mock in dev.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // G106: dev sftp-mock only; host-key pinning tracked in the TODO above (E8-S1)
		Timeout:         30 * time.Second,
	}
	conn, err := ssh.Dial("tcp", c.addr, sshCfg)
	if err != nil {
		return fmt.Errorf("mft: dial %s: %w", c.addr, err)
	}
	defer func() { _ = conn.Close() }()

	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("mft: sftp session: %w", err)
	}
	defer func() { _ = client.Close() }()

	if dir := path.Dir(remotePath); dir != "." && dir != "/" {
		if err := client.MkdirAll(dir); err != nil {
			return fmt.Errorf("mft: mkdir %s: %w", dir, err)
		}
	}

	tmp := remotePath + ".tmp"
	f, err := client.Create(tmp)
	if err != nil {
		return fmt.Errorf("mft: create %s: %w", tmp, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = client.Remove(tmp)
		return fmt.Errorf("mft: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = client.Remove(tmp)
		return fmt.Errorf("mft: close %s: %w", tmp, err)
	}
	// Atomic publish: rename tmp onto the final path (idempotent — overwrites any
	// prior attempt at the destination).
	if err := client.PosixRename(tmp, remotePath); err != nil {
		_ = client.Remove(tmp)
		return fmt.Errorf("mft: rename %s -> %s: %w", tmp, remotePath, err)
	}
	return nil
}

// compile-time assertion that sftpClient satisfies interpreter.MFTClient.
var _ interpreter.MFTClient = (*sftpClient)(nil)
