// Package app composes the concrete local MindWeaver runtime. Startup is
// intentionally staged: configuration, Vault ownership, filesystem cleanup,
// schema migration, and durable job recovery all finish before the loopback
// listener accepts ordinary work.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/ingest"
	"github.com/mt-hub8/MindWeaver/v2/internal/lifecycle"
	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	pdfclient "github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/client"
	"github.com/mt-hub8/MindWeaver/v2/internal/rag"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/internal/webui"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
	"github.com/mt-hub8/MindWeaver/v2/platform/config"
)

const (
	defaultWorkerInterval          = 150 * time.Millisecond
	defaultWorkerLease             = 5 * time.Minute
	defaultAnswerReconcileInterval = 5 * time.Second
	startupSweepBatch              = 128
	shutdownGrace                  = 8 * time.Second
)

// RouteRegistrar appends exact routes before localhttp seals the router. It is
// the narrow composition point for another accepted vertical slice; runtime
// route mutation and wildcard registration remain impossible.
type RouteRegistrar func(*localhttp.Router) error

// Options contains only process bootstrap controls. FirstRunVaultRoot is used
// solely when ConfigPath does not exist; an existing versioned configuration is
// always authoritative.
type Options struct {
	ConfigPath              string
	FirstRunVaultRoot       string
	HTTP                    localhttp.Config
	ExtraRoutes             []RouteRegistrar
	WorkerInterval          time.Duration
	AnswerReconcileInterval time.Duration
	PDFHelperPath           string
}

// StartupEvidence records bounded, non-sensitive reconciliation outcomes.
type StartupEvidence struct {
	ConfigCreated            bool  `json:"configCreated"`
	RecoveredJobs            int64 `json:"recoveredJobs"`
	CleanedStagingFiles      int   `json:"cleanedStagingFiles"`
	SweptBlobCandidates      int   `json:"sweptBlobCandidates"`
	ReconciledPendingAnswers int64 `json:"reconciledPendingAnswers"`
}

// App owns every process-lifetime local resource in dependency order.
type App struct {
	vault    *vault.Vault
	database *store.Store
	server   *localhttp.Server
	worker   *ingestionWorker
	rag      *ragRuntime
	backups  *backupRuntime

	bootstrap localhttp.BootstrapToken
	startup   StartupEvidence

	shutdownOnce sync.Once
	shutdownDone chan struct{}
	shutdownErr  error
}

// Start opens the complete local runtime. No listener is created until every
// migration and recovery step has returned successfully.
func Start(ctx context.Context, options Options) (*App, error) {
	if ctx == nil {
		return nil, errors.New("app: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.ConfigPath == "" {
		options.ConfigPath = config.DefaultFileName
	}
	if options.FirstRunVaultRoot == "" {
		options.FirstRunVaultRoot = "./vault"
	}
	if options.WorkerInterval == 0 {
		options.WorkerInterval = defaultWorkerInterval
	}
	if options.WorkerInterval < 10*time.Millisecond || options.WorkerInterval > 10*time.Second {
		return nil, errors.New("app: worker interval must be between 10ms and 10s")
	}
	if options.AnswerReconcileInterval == 0 {
		options.AnswerReconcileInterval = defaultAnswerReconcileInterval
	}
	if options.AnswerReconcileInterval < 10*time.Millisecond || options.AnswerReconcileInterval > 10*time.Minute {
		return nil, errors.New("app: answer reconciliation interval must be between 10ms and 10m")
	}

	created, err := ensureConfig(ctx, options.ConfigPath, options.FirstRunVaultRoot)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(ctx, options.ConfigPath)
	if err != nil {
		return nil, err
	}
	vaultRoot, err := config.ResolveVault(options.ConfigPath, cfg)
	if err != nil {
		return nil, err
	}

	openedVault, err := vault.Open(vaultRoot)
	if err != nil {
		return nil, fmt.Errorf("app: open Vault: %w", err)
	}
	paths := openedVault.Paths()
	closeVaultOnError := true
	defer func() {
		if closeVaultOnError {
			_ = openedVault.Close()
		}
	}()

	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		return nil, fmt.Errorf("app: open blob store: %w", err)
	}
	cleaned, err := blobs.CleanupStaging(ctx, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("app: reconcile blob staging: %w", err)
	}

	database, err := store.Open(ctx, filepath.Join(paths.Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		return nil, fmt.Errorf("app: open database: %w", err)
	}
	closeDatabaseOnError := true
	defer func() {
		if closeDatabaseOnError {
			_ = database.Close()
		}
	}()
	recovered, err := database.RecoverInterruptedAtStartup(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("app: recover durable jobs: %w", err)
	}
	lifecycleService, err := lifecycle.New(database, blobs)
	if err != nil {
		return nil, err
	}
	swept, err := sweepLifecycleAtStartup(ctx, lifecycleService)
	if err != nil {
		return nil, fmt.Errorf("app: reconcile pending blob deletion: %w", err)
	}
	ragService, err := rag.New(database)
	if err != nil {
		return nil, err
	}
	reconciledAnswers, err := ragService.ReconcilePendingAnswersOnStartup(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: reconcile pending answers: %w", err)
	}
	ragRuntime, err := newRAGRuntime(ragService, options.AnswerReconcileInterval)
	if err != nil {
		return nil, err
	}

	service, err := workbench.New(database, blobs)
	if err != nil {
		return nil, err
	}
	pdfReady := false
	pdfHelperPath := options.PDFHelperPath
	if pdfHelperPath == "" {
		pdfHelperPath = adjacentPDFHelper()
	}
	if pdfClient, clientErr := pdfclient.New(pdfHelperPath, 30*time.Second); clientErr == nil && pdfClient.Probe(ctx) == nil {
		service, err = workbench.NewWithPDF(database, blobs, pdfClient)
		if err != nil {
			return nil, err
		}
		pdfReady = true
	}
	worker := newIngestionWorker(service, database, options.WorkerInterval, defaultWorkerLease)
	backupCoordinator, err := backup.New(database, blobs, paths.Root)
	if err != nil {
		return nil, fmt.Errorf("app: initialize backup coordinator: %w", err)
	}
	backupRuntime, err := newBackupRuntime(
		coordinatorBackupEngine{backupCoordinator},
		func(source string) (string, error) {
			return backup.PrepareLiveBackupVerifyScratch(paths.Root, source)
		},
	)
	if err != nil {
		_ = backupCoordinator.Close()
		return nil, err
	}
	closeBackupOnError := true
	defer func() {
		if closeBackupOnError {
			_ = backupRuntime.Close()
		}
	}()
	evidence := StartupEvidence{
		ConfigCreated: created, RecoveredJobs: recovered, CleanedStagingFiles: cleaned,
		SweptBlobCandidates:      swept,
		ReconciledPendingAnswers: reconciledAnswers,
	}
	router := localhttp.NewRouter()
	if err := webui.Register(router); err != nil {
		return nil, err
	}
	api := newAPI(service, lifecycleService, worker, ragRuntime, backupRuntime, evidence, pdfReady)
	if err := api.Register(router); err != nil {
		return nil, err
	}
	for index, register := range options.ExtraRoutes {
		if register == nil {
			return nil, fmt.Errorf("app: extra route registrar %d is nil", index)
		}
		if err := register(router); err != nil {
			return nil, fmt.Errorf("app: register extra routes %d: %w", index, err)
		}
	}

	httpConfig := options.HTTP
	if httpConfig.MaxBodyBytes == 0 {
		httpConfig.MaxBodyBytes = ingest.MaxTextSourceBytes
	}
	if httpConfig.RequestTimeout == 0 {
		httpConfig.RequestTimeout = 2 * time.Minute
	}
	if httpConfig.RequestTimeout < 2*time.Minute {
		return nil, errors.New("app: HTTP request timeout must be at least 120s for durable Ask convergence")
	}
	if httpConfig.ReadTimeout == 0 {
		httpConfig.ReadTimeout = 2 * time.Minute
	}
	if httpConfig.WriteTimeout == 0 {
		httpConfig.WriteTimeout = 2 * time.Minute
	}
	if httpConfig.WriteTimeout < 2*time.Minute {
		return nil, errors.New("app: HTTP write timeout must be at least 120s for durable Ask convergence")
	}
	server, bootstrap, err := localhttp.Start(router, httpConfig)
	if err != nil {
		return nil, fmt.Errorf("app: start loopback server: %w", err)
	}
	worker.Start()
	ragRuntime.Start()

	closeVaultOnError = false
	closeDatabaseOnError = false
	closeBackupOnError = false
	return &App{
		vault:        openedVault,
		database:     database,
		server:       server,
		worker:       worker,
		rag:          ragRuntime,
		backups:      backupRuntime,
		bootstrap:    bootstrap,
		startup:      evidence,
		shutdownDone: make(chan struct{}),
	}, nil
}

type lifecycleSweeper interface {
	Sweep(context.Context, int) (lifecycle.SweepResult, error)
}

// sweepLifecycleAtStartup drains the durable cleanup queue before any listener
// or ordinary-work worker becomes reachable. A failed or still-pending batch is
// a startup failure: accepting new writes would conceal an incomplete purge.
func sweepLifecycleAtStartup(ctx context.Context, sweeper lifecycleSweeper) (int, error) {
	if ctx == nil {
		return 0, errors.New("app: nil startup sweep context")
	}
	if sweeper == nil {
		return 0, errors.New("app: nil startup lifecycle sweeper")
	}
	total := 0
	for {
		result, err := sweeper.Sweep(ctx, startupSweepBatch)
		total += result.ProcessedCandidates
		if err != nil {
			return total, err
		}
		if len(result.PendingBlobIDs) != 0 {
			return total, errors.New("app: startup lifecycle sweep left pending candidates")
		}
		if result.ProcessedCandidates == 0 {
			return total, nil
		}
	}
}

func adjacentPDFHelper() string {
	executable, err := os.Executable()
	if err != nil {
		return filepath.Join(".", pdfHelperName())
	}
	return filepath.Join(filepath.Dir(executable), pdfHelperName())
}

func pdfHelperName() string {
	if runtime.GOOS == "windows" {
		return "mindweaver-pdf.exe"
	}
	return "mindweaver-pdf"
}

// Origin is the exact loopback origin selected for this process.
func (app *App) Origin() string {
	if app == nil || app.server == nil {
		return ""
	}
	return app.server.Origin()
}

// LaunchURL carries the one-use bootstrap token only in a URL fragment, which
// browsers do not send in the HTTP request or Referer.
func (app *App) LaunchURL() string {
	if app == nil || app.server == nil {
		return ""
	}
	return app.server.Origin() + "/#bootstrap=" + app.bootstrap.HeaderValue()
}

// Startup returns a copy of non-sensitive startup evidence.
func (app *App) Startup() StartupEvidence {
	if app == nil {
		return StartupEvidence{}
	}
	return app.startup
}

// Done reports an unexpected or requested HTTP serving stop.
func (app *App) Done() <-chan error {
	if app == nil || app.server == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return app.server.Done()
}

// Shutdown quiesces ingress, cancels local work, then closes SQLite and finally
// releases the Vault lock. A caller timeout does not abandon cleanup: the first
// call starts cleanup and later calls may wait for its eventual safe completion.
func (app *App) Shutdown(ctx context.Context) error {
	if app == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("app: shutdown context is required")
	}
	app.shutdownOnce.Do(func() { go app.shutdown() })
	select {
	case <-app.shutdownDone:
		return app.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (app *App) shutdown() {
	defer close(app.shutdownDone)
	app.rag.Quiesce()
	grace, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	serverErr := app.server.Shutdown(grace)
	if serverErr != nil {
		serverErr = errors.Join(serverErr, app.server.Close())
	}
	// Backup Create is detached from its fast HTTP admission response. Once
	// ingress is closed, cancel that local operation, wait for its filesystem
	// cleanup/publication decision, then release its retained Vault capability
	// before SQLite or the Vault lock can be closed.
	app.backups.Quiesce()
	backupErr := app.backups.Wait(grace)
	backupErr = errors.Join(backupErr, app.backups.Wait(context.Background()), app.backups.Close())
	// Closing ingress may cancel an active provider call. Ask owns a bounded
	// WithoutCancel terminal write; do not close SQLite until it has returned.
	ragErr := app.rag.Wait(context.Background())
	app.worker.Quiesce()
	workerErr := app.worker.Wait(grace)
	if workerErr != nil {
		app.worker.Stop()
	}
	// Never close SQLite underneath a worker. The caller retains its own wait
	// bound, while this cleanup goroutine continues until the bounded local file
	// operation observes cancellation and exits.
	workerErr = errors.Join(workerErr, app.worker.Wait(context.Background()))
	app.shutdownErr = errors.Join(serverErr, backupErr, ragErr, workerErr, app.database.Close(), app.vault.Close())
}

func ensureConfig(ctx context.Context, path, firstVault string) (bool, error) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return false, nil
	case !errors.Is(err, os.ErrNotExist):
		return false, fmt.Errorf("app: inspect configuration: %w", err)
	}
	if err := config.WriteNew(ctx, path, config.Default(firstVault)); err != nil {
		// Another simultaneous launcher may have won creation; Load will validate
		// the complete file. Any other failure remains authoritative.
		if _, statErr := os.Lstat(path); statErr != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}
