// Package ui implements the Bubble Tea TUI for lazyaddons. The
// package is built around a single Model that owns all state and
// dispatches Update/View to per-screen helpers based on a
// `screen` enum. This avoids the indirection of composed
// sub-models for a UI that is, in practice, a 5-screen flow.
package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pentsec/lazyaddons/internal/addon"
	"github.com/pentsec/lazyaddons/internal/config"
	"github.com/pentsec/lazyaddons/internal/github"
	"github.com/pentsec/lazyaddons/internal/update"
	"github.com/pentsec/lazyaddons/internal/wowpath"
)

// screen identifies the currently active TUI screen.
type screen int

const (
	screenList screen = iota
	screenAddForm
	screenReleasePicker
	screenProgress
	screenError
	screenWowPath
	screenWowBrowse
	screenConfirmRemove
	screenConfirmReplace
	// Profile screens (T3+): manage the multi-profile flow.
	// screenProfileAdd is also the first-run screen when the
	// user has zero profiles configured.
	screenProfilePicker
	screenProfileAdd
	screenProfileRename
	screenProfileDelete
)

// String returns a stable identifier for the screen, useful for
// tests that match on the rendered output.
func (s screen) String() string {
	switch s {
	case screenList:
		return "list"
	case screenAddForm:
		return "addForm"
	case screenReleasePicker:
		return "releasePicker"
	case screenProgress:
		return "progress"
	case screenError:
		return "error"
	case screenWowPath:
		return "wowPath"
	case screenWowBrowse:
		return "wowBrowse"
	case screenConfirmRemove:
		return "confirmRemove"
	case screenConfirmReplace:
		return "confirmReplace"
	case screenProfilePicker:
		return "profilePicker"
	case screenProfileAdd:
		return "profileAdd"
	case screenProfileRename:
		return "profileRename"
	case screenProfileDelete:
		return "profileDelete"
	}
	return "unknown"
}

// AddonStatus is the per-addon status badge, used by both the model
// and the list renderer.
type AddonStatus int

const (
	StatusUnknown AddonStatus = iota
	StatusOK
	StatusUpdate
	StatusError
	StatusInstalling
)

// String returns the badge character for a status.
func (s AddonStatus) String() string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusUpdate:
		return "↑"
	case StatusError:
		return "✗"
	case StatusInstalling:
		return "⟳"
	}
	return "?"
}

// Label returns a human-readable label for the status.
func (s AddonStatus) Label() string {
	switch s {
	case StatusOK:
		return "up to date"
	case StatusUpdate:
		return "update avail"
	case StatusError:
		return "error"
	case StatusInstalling:
		return "installing"
	}
	return "unknown"
}

// Model is the root Bubble Tea model. It owns the config, the
// tracked addons, the current screen, and per-screen local state.
type Model struct {
	// Screen state
	Screen screen
	Width  int
	Height int

	// Domain state
	Config     *config.Config
	WowPath    wowpath.Path
	GitHub     *github.Client
	Selection  int
	Statuses   map[string]AddonStatus // keyed by addon.Name
	ErrMessage string

	// ActiveProfile points into Config.Profiles at the currently
	// active profile. nil when no profile is active (e.g. fresh
	// install). Path-consuming code (clone, update, backup, scan)
	// should read m.ActiveProfile.WoWPath; m.WowPath is kept in
	// sync via setActiveProfile for convenience.
	ActiveProfile *config.Profile

	// Add flow state
	AddInput        string
	AddReleases     []github.Release
	AddError        string
	AddPickedRelease string // "" => branch mode, otherwise tag

	// Clone state — carried across the progress screen so the
	// async clone command knows what to install.
	PendingAddon struct {
		Name   string
		URL    string
		Mode   string // "branch" or "release"
		Target string // branch name or tag
	}

	// WoW path setup state
	WowPathInput string
	WowPathError string
	WowCandidates []string // auto-detected candidates
	WowCandSel    int      // selected candidate index (-1 = none)
	WowSearching  bool     // true while detection is running
	WowBrowsePath  string // current directory in browser
	WowBrowseSel   int    // selected index in browser
	WowBrowseError string // error message for browser
	WowWriteWarning string // set when path is valid but not writable

	// Progress state
	ProgressLabel string

	// Progress animation
	spinnerFrame  int
	progressStart time.Time
	progressStep  int
	progressTotal int

	// Remove confirmation state
	PendingRemove string

	// Replace folder confirmation state
	ReplaceFolder   string
	ReplaceName     string
	ReplaceURL      string
	ReplaceMode     string
	ReplaceTarget   string
	ReplaceSel      int // 0 = keep existing, 1 = replace

	// Self-update state
	UpdateBanner *update.CheckResult

	// List scroll and search state
	SearchQuery  string
	SearchActive bool
	ScrollOffset int

	// Profile picker state
	ProfileCursor int // index of selected profile in picker

	// Profile add state. PendingProfileName carries the name the
	// user typed on screenProfileAdd; on the path screen it is
	// checked by confirmPath/acceptPath to know we are in
	// "create profile" mode.
	PendingProfileName string
	PendingProfileID   string // id of the profile being renamed or deleted
	PendingProfilePath string // resolved path during creation (between confirm and accept)
	ProfileNameError   string // shown on screenProfileAdd / screenProfileRename
	ProfileError       string // shown on screenProfileDelete for active-profile rejection etc.
}

// NewModel constructs a Model with sensible defaults. Callers
// typically override Config and WowPath before passing the model
// to tea.NewProgram.
func NewModel() *Model {
	return &Model{
		Screen:        screenList,
		Config:        config.Default(),
		GitHub:        github.New(),
		Statuses:      map[string]AddonStatus{},
		WowBrowsePath: "/",
		WowCandSel:    -1,
	}
}

// StartScreen returns the screen the program should show on
// launch. If no profiles are configured (fresh install), it
// starts on screenProfileAdd so the user must create a profile
// before anything else. Otherwise it lands on the addon list.
func (m *Model) StartScreen() screen {
	if m.Config == nil || len(m.Config.Profiles) == 0 {
		return screenProfileAdd
	}
	return screenList
}

// SetActiveProfile wires the active profile pointer and syncs
// the convenience m.WoWPath field. Callers should use this
// rather than assigning m.ActiveProfile directly so the path
// stays consistent.
func (m *Model) SetActiveProfile(p *config.Profile) {
	m.ActiveProfile = p
	if p != nil {
		m.WowPath = wowpath.Path(p.WoWPath)
	}
}

func (m *Model) startProgress(label string, step, total int) {
	m.ProgressLabel = label
	m.progressStep = step
	m.progressTotal = total
	m.progressStart = time.Now()
	m.spinnerFrame = 0
	m.Screen = screenProgress
}

// Init is the Bubble Tea Init function. If there are tracked
// addons in the active profile, it fires an automatic update
// check on startup.
func (m Model) Init() tea.Cmd {
	if m.WowSearching {
		return detectCandidatesCmd()
	}
	if m.ActiveProfile != nil && len(m.ActiveProfile.Addons) > 0 {
		return checkAllUpdatesCmd(string(m.WowPath), m.ActiveProfile.Addons)
	}
	return nil
}

// Update is the Bubble Tea Update dispatch. It routes messages
// based on the current screen and returns the (possibly mutated)
// model plus any follow-up commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case spinnerTickMsg:
		if m.Screen == screenProgress {
			m.spinnerFrame++
			return m, spinnerCmd()
		}
		return m, nil
	case progressStepMsg:
		m.ProgressLabel = msg.Label
		m.progressStep = msg.Step
		m.progressTotal = msg.Total
		return m, nil
	case cloneDoneMsg:
		m.handleCloneDone(msg)
		return m, nil
	case wowCandidatesMsg:
		m.WowCandidates = msg.Candidates
		m.WowSearching = false
		if len(m.WowCandidates) > 0 {
			m.WowCandSel = 0
		}
		return m, nil
	case releaseFetchedMsg:
		cmd := m.handleReleaseFetched(msg)
		return m, cmd
	case updatesCheckedMsg:
		cmd := m.handleUpdatesChecked(msg)
		return m, cmd
	case updateAppliedMsg:
		m.handleUpdateApplied(msg)
		return m, nil
	case selfUpdateDoneMsg:
		handleSelfUpdateDone(&m, msg)
		return m, nil
	}
	return m, nil
}

// handleKey dispatches a key press to the screen-specific
// handler. The list and add screens are the only ones with
// meaningful bindings.
func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	switch m.Screen {
	case screenList:
		return updateList(&m, key)
	case screenAddForm:
		return updateAddForm(&m, key)
	case screenReleasePicker:
		return updateReleasePicker(&m, key)
	case screenProgress:
		return updateProgress(&m, key)
	case screenError:
		m.Screen = screenList
		m.ErrMessage = ""
		return m, nil
	case screenWowPath:
		return updateWowPath(&m, key)
	case screenWowBrowse:
		return updateWowBrowse(&m, key)
	case screenConfirmRemove:
		return updateConfirmRemove(&m, key)
	case screenConfirmReplace:
		return updateConfirmReplace(&m, key)
	case screenProfilePicker:
		return updateProfilePicker(&m, key)
	case screenProfileAdd:
		return updateProfileAdd(&m, key)
	case screenProfileRename:
		return updateProfileRename(&m, key)
	case screenProfileDelete:
		return updateProfileDelete(&m, key)
	}
	return m, nil
}

// View dispatches rendering to the screen-specific View. Every
// screen is wrapped with the Header() so the logo is always visible.
func (m Model) View() string {
	var content string
	switch m.Screen {
	case screenList:
		content = viewList(&m)
	case screenAddForm:
		content = viewAddForm(&m)
	case screenReleasePicker:
		content = viewReleasePicker(&m)
	case screenProgress:
		content = viewProgress(&m)
	case screenError:
		content = viewError(&m)
	case screenWowPath:
		content = viewWowPath(&m)
	case screenWowBrowse:
		content = viewWowBrowse(&m)
	case screenConfirmRemove:
		content = viewConfirmRemove(&m)
	case screenConfirmReplace:
		content = viewConfirmReplace(&m)
	case screenProfilePicker:
		content = viewProfilePicker(&m)
	case screenProfileAdd:
		content = viewProfileAdd(&m)
	case screenProfileRename:
		content = viewProfileRename(&m)
	case screenProfileDelete:
		content = viewProfileDelete(&m)
	default:
		content = fmt.Sprintf("unknown screen %d", m.Screen)
	}
	w := max(m.Width-2, minInner)
	return WrapFrame(Header(w)+"\n"+content+"\n"+m.Footer(w), w)
}

// handleReleaseFetched processes the GitHub API response for the
// latest release. On success it starts the clone with the real
// tag and returns the clone command; on failure it shows the error.
func (m *Model) handleReleaseFetched(msg releaseFetchedMsg) tea.Cmd {
	if msg.Err != nil {
		m.ErrMessage = fmt.Sprintf("Could not fetch release: %v", msg.Err)
		m.Screen = screenError
		return nil
	}
	return m.startClone(msg.Name, msg.URL, addon.TrackModeRelease, msg.TagName)
}

// wowCandidatesMsg is sent when auto-detection finishes.
type wowCandidatesMsg struct {
	Candidates []string
}

func detectCandidatesCmd() tea.Cmd {
	return func() tea.Msg {
		return wowCandidatesMsg{Candidates: wowpath.DetectCandidates()}
	}
}

// Helper: readKey is exported only to the package's test file.
func (m *Model) selectedAddon() *config.Addon {
	if m.ActiveProfile == nil {
		return nil
	}
	if len(m.ActiveProfile.Addons) == 0 {
		return nil
	}
	if m.Selection < 0 || m.Selection >= len(m.ActiveProfile.Addons) {
		return nil
	}
	return &m.ActiveProfile.Addons[m.Selection]
}
