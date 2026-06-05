package poller

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/boyvinall/ghnotify/internal/auth"
	"github.com/boyvinall/ghnotify/internal/config"
	"github.com/boyvinall/ghnotify/internal/github"
)

// Manager runs a per-server poll loop and notifies callers of PR state changes.
type Manager struct {
	authMgr *auth.Manager
	cfg     *config.AppConfig
	store   *stateStore

	onChange func([]Change)

	mu       sync.Mutex
	cancels  map[string]context.CancelFunc // per-server cancel
	wg       sync.WaitGroup
	rootCtx  context.Context
	rootStop context.CancelFunc
}

// NewManager creates a Manager. Call Start() to begin polling.
func NewManager(authMgr *auth.Manager, cfg *config.AppConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		authMgr:  authMgr,
		cfg:      cfg,
		store:    newStateStore(),
		cancels:  make(map[string]context.CancelFunc),
		rootCtx:  ctx,
		rootStop: cancel,
	}
}

// OnChange registers a callback invoked (synchronously, in the poll goroutine)
// when PR state changes. Register before calling Start.
func (m *Manager) OnChange(fn func([]Change)) {
	m.onChange = fn
}

// MyPRs returns a current snapshot of all tracked PRs authored by the user.
func (m *Manager) MyPRs() []github.PR {
	return m.store.MyPRs()
}

// ReviewRequests returns a current snapshot of all tracked review requests.
func (m *Manager) ReviewRequests() []github.PR {
	return m.store.ReviewRequests()
}

// Start launches poll goroutines for all configured servers.
func (m *Manager) Start() {
	for _, sc := range m.cfg.Servers {
		m.startServer(sc.Host)
	}
}

// StartServer begins polling a server that was added after Start.
func (m *Manager) StartServer(host string) {
	m.startServer(host)
}

// StopServer stops the poll loop for host and clears its state.
func (m *Manager) StopServer(host string) {
	m.mu.Lock()
	if cancel, ok := m.cancels[host]; ok {
		cancel()
		delete(m.cancels, host)
	}
	m.mu.Unlock()
	m.store.RemoveHost(host)
}

// Stop shuts down all poll loops and waits for them to finish.
func (m *Manager) Stop() {
	m.rootStop()
	m.wg.Wait()
}

// PollNow triggers an immediate poll for host without waiting for the next interval.
func (m *Manager) PollNow(host string) {
	go func() {
		ctx, cancel := context.WithTimeout(m.rootCtx, 30*time.Second)
		defer cancel()
		m.pollServer(ctx, host)
	}()
}

// --- internal ----------------------------------------------------------------

func (m *Manager) startServer(host string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, running := m.cancels[host]; running {
		return
	}
	ctx, cancel := context.WithCancel(m.rootCtx)
	m.cancels[host] = cancel

	m.wg.Add(1)
	go m.runServerLoop(ctx, host)
}

func (m *Manager) runServerLoop(ctx context.Context, host string) {
	defer m.wg.Done()

	interval := m.pollInterval()

	// Poll immediately on start.
	m.pollServer(ctx, host)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter(interval)):
			m.pollServer(ctx, host)
		}
	}
}

func (m *Manager) pollServer(ctx context.Context, host string) {
	token, err := m.authMgr.GetToken(host)
	if err != nil || token == "" {
		return // not authenticated yet; skip silently
	}

	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client := github.NewClient(host, token)

	myPRs, err := client.FetchMyPRs(tctx)
	if err != nil {
		if github.IsUnauthorized(err) {
			if refreshErr := m.authMgr.RefreshToken(host); refreshErr == nil {
				if token, err = m.authMgr.GetToken(host); err == nil {
					client = github.NewClient(host, token)
					myPRs, err = client.FetchMyPRs(tctx)
				}
			}
		}
		if err != nil {
			log.Printf("ghnotify: poll %s (my PRs): %v", host, err)
		}
	}

	reviews, err := client.FetchReviewRequests(tctx)
	if err != nil {
		log.Printf("ghnotify: poll %s (review requests): %v", host, err)
	}

	changes := m.store.Update(host, myPRs, reviews)
	if len(changes) > 0 && m.onChange != nil {
		m.onChange(changes)
	}
}

func (m *Manager) pollInterval() time.Duration {
	d, err := time.ParseDuration(m.cfg.PollInterval)
	if err != nil || d < 10*time.Second {
		return 60 * time.Second
	}
	return d
}

func jitter(d time.Duration) time.Duration {
	spread := int64(float64(d) * 0.1)
	if spread == 0 {
		return d
	}
	return d + time.Duration(rand.Int63n(spread*2)-spread)
}
