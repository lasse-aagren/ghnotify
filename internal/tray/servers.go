package tray

import (
	"fmt"
	"log"

	"github.com/boyvinall/ghnotify/internal/auth"
	"github.com/boyvinall/ghnotify/internal/config"
	"github.com/boyvinall/ghnotify/internal/dialog"
	"github.com/boyvinall/ghnotify/internal/poller"
	"github.com/getlantern/systray"
)

// serverSection manages the Servers sub-menu and the Add Server button.
type serverSection struct {
	mgr     *auth.Manager
	cfg     *config.AppConfig
	poll    *poller.Manager
	mParent *systray.MenuItem // "Servers" top-level item
	mAdd    *systray.MenuItem // "Add server…" top-level item
	items   []*serverItem
}

func newServerSection(mgr *auth.Manager, cfg *config.AppConfig, poll *poller.Manager) *serverSection {
	return &serverSection{mgr: mgr, cfg: cfg, poll: poll}
}

// build creates the initial menu structure based on currently configured servers.
// Must be called from the systray onReady goroutine.
func (ss *serverSection) build() {
	ss.mParent = systray.AddMenuItem("Servers", "")
	ss.mParent.Disable()

	for _, sc := range ss.cfg.Servers {
		ss.addItem(sc)
	}

	ss.mAdd = systray.AddMenuItem("Add server…", "")
	go ss.listenAdd()

	// Register for auth changes so item titles stay current.
	ss.mgr.OnChange(func(host string) {
		for _, it := range ss.items {
			if it.host == host {
				it.refresh()
				return
			}
		}
	})
}

func (ss *serverSection) addItem(sc config.ServerConfig) *serverItem {
	it := &serverItem{
		host:    sc.Host,
		authTyp: sc.AuthType,
		mgr:     ss.mgr,
		poll:    ss.poll,
		section: ss,
	}
	it.build(ss.mParent)
	ss.items = append(ss.items, it)
	go it.listen()
	return it
}

func (ss *serverSection) listenAdd() {
	for range ss.mAdd.ClickedCh {
		ss.doAddServer()
	}
}

func (ss *serverSection) doAddServer() {
	host, ok := dialog.Input("Enter GitHub hostname:", "github.com")
	if !ok || host == "" {
		return
	}
	// Check duplicate.
	for _, it := range ss.items {
		if it.host == host {
			dialog.Alert("Already configured", fmt.Sprintf("%q is already in the server list.", host))
			return
		}
	}

	authChoice, ok := dialog.Choose(
		fmt.Sprintf("How should ghnotify authenticate to %s?", host),
		"OAuth", "OAuth", "PAT",
	)
	if !ok {
		return
	}

	var (
		authType     config.AuthType
		clientID     string
		clientSecret string
	)
	if authChoice == "OAuth" {
		authType = config.AuthTypeOAuth
		clientID, ok = dialog.Input("Enter OAuth App Client ID:", "")
		if !ok || clientID == "" {
			return
		}
		clientSecret, ok = dialog.Secret("Enter OAuth App Client Secret:")
		if !ok || clientSecret == "" {
			return
		}
	} else {
		authType = config.AuthTypePAT
	}

	if err := ss.mgr.AddServer(host, authType, clientID, clientSecret); err != nil {
		dialog.Alert("Error", err.Error())
		return
	}

	// Build the new menu item (appended under Servers).
	sc := config.ServerConfig{Host: host, AuthType: authType, ClientID: clientID}
	it := ss.addItem(sc)

	// Start polling this server immediately (will no-op if not yet authenticated).
	ss.poll.StartServer(host)

	// Offer immediate login.
	if authType == config.AuthTypeOAuth {
		btn, ok := dialog.Choose(
			fmt.Sprintf("Log in to %s now with OAuth?", host),
			"Login", "Login", "Later",
		)
		if ok && btn == "Login" {
			go it.doLogin()
		}
	} else {
		btn, ok := dialog.Choose(
			fmt.Sprintf("Enter a PAT for %s now?", host),
			"Enter PAT", "Enter PAT", "Later",
		)
		if ok && btn == "Enter PAT" {
			go it.doSetPAT()
		}
	}
}

// ── per-server item ──────────────────────────────────────────────────────────

type serverItem struct {
	host    string
	authTyp config.AuthType
	mgr     *auth.Manager
	poll    *poller.Manager
	section *serverSection

	mServer  *systray.MenuItem // sub-item title: "github.com [Connected as @user]"
	mRefresh *systray.MenuItem // "Refresh now"
	mLogin   *systray.MenuItem // "Login with OAuth…"
	mSetPAT  *systray.MenuItem // "Set PAT…"
	mLogout  *systray.MenuItem // "Logout"
	mRemove  *systray.MenuItem // "Remove server"
	mHidden  bool              // true once removed
}

func (it *serverItem) build(parent *systray.MenuItem) {
	it.mServer = parent.AddSubMenuItem(it.serverTitle(), "")
	it.mRefresh = it.mServer.AddSubMenuItem("Refresh now", "")
	it.mLogin = it.mServer.AddSubMenuItem("Login with OAuth…", "")
	it.mSetPAT = it.mServer.AddSubMenuItem("Set PAT…", "")
	it.mLogout = it.mServer.AddSubMenuItem("Logout", "")
	it.mRemove = it.mServer.AddSubMenuItem("Remove server", "")
	it.refresh()
}

func (it *serverItem) serverTitle() string {
	if it.mgr.IsAuthenticated(it.host) {
		if u := it.mgr.GetUsername(it.host); u != "" {
			return fmt.Sprintf("%s  [Connected as @%s]", it.host, u)
		}
		return fmt.Sprintf("%s  [Connected]", it.host)
	}
	return fmt.Sprintf("%s  [Not connected]", it.host)
}

// refresh updates menu item titles and visibility based on current auth state.
func (it *serverItem) refresh() {
	it.mServer.SetTitle(it.serverTitle())
	authed := it.mgr.IsAuthenticated(it.host)

	if authed {
		it.mRefresh.Show()
		it.mLogout.Show()
	} else {
		it.mRefresh.Hide()
		it.mLogout.Hide()
	}
	if it.authTyp == config.AuthTypeOAuth && !authed {
		it.mLogin.Show()
	} else {
		it.mLogin.Hide()
	}
	if it.authTyp == config.AuthTypePAT && !authed {
		it.mSetPAT.Show()
	} else {
		it.mSetPAT.Hide()
	}
}

func (it *serverItem) listen() {
	for {
		select {
		case <-it.mRefresh.ClickedCh:
			it.poll.PollNow(it.host)
		case <-it.mLogin.ClickedCh:
			go it.doLogin()
		case <-it.mSetPAT.ClickedCh:
			go it.doSetPAT()
		case <-it.mLogout.ClickedCh:
			it.doLogout()
		case <-it.mRemove.ClickedCh:
			it.doRemove()
			return
		}
	}
}

func (it *serverItem) doLogin() {
	if err := it.mgr.LoginOAuth(it.host); err != nil {
		log.Printf("oauth login %s: %v", it.host, err)
		dialog.Alert("Login failed", err.Error())
		return
	}
	it.refresh()
}

func (it *serverItem) doSetPAT() {
	token, ok := dialog.Secret(fmt.Sprintf("Enter Personal Access Token for %s:", it.host))
	if !ok || token == "" {
		return
	}
	if err := it.mgr.StorePAT(it.host, token); err != nil {
		dialog.Alert("PAT error", err.Error())
		return
	}
	it.refresh()
}

func (it *serverItem) doLogout() {
	_ = it.mgr.Logout(it.host)
	it.refresh()
}

func (it *serverItem) doRemove() {
	if err := it.mgr.RemoveServer(it.host); err != nil {
		dialog.Alert("Error", err.Error())
		return
	}
	it.poll.StopServer(it.host)
	it.mServer.Hide()
	it.mHidden = true
}
