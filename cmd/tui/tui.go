package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"whatsrook"
	"whatsrook/cmd/updater"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type state int

const (
	stateMain state = iota
	stateSessionsList
	stateSessionActions
	stateEditVariables
	stateEditClient
	stateEditLogLevel
	stateEditDB
	stateDeleteConfirm
	stateNewAuth
	stateNewPhoneInput
	stateNewBusinessAccount
	stateNewClient
	stateNewLogLevel
	stateNewDB
	stateNewSaveOption
	stateUpdateMenu
	stateUpdateProgress
	stateDependenciesMenu
	stateDependenciesProgress
	stateDonate
)

// SessionResult contains the final configured session parameters to launch.
type SessionResult struct {
	Session       string
	Pair          bool
	QRCode        bool
	ClientType    whatsrook.ClientType
	Database      string
	Verbose       bool
	ShouldRun     bool
	ShouldRestart bool
}

type tickMsg time.Time

type updateLogChunkMsg string

type updateFinishedMsg struct {
	result *updater.UpdateResult
	err    error
}

type chanWriter struct {
	ch chan<- string
}

func (w *chanWriter) Write(p []byte) (n int, err error) {
	text := string(p)
	lines := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\r' || r == '\n'
	})
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			select {
			case w.ch <- trimmed:
			default:
			}
		}
	}
	return len(p), nil
}

type model struct {
	ctx             context.Context
	state           state
	defaultDB       string
	boundPort       int
	currentTime     time.Time
	cursor          int
	sessions        []whatsrook.StoredSession
	selectedSession whatsrook.StoredSession
	result          SessionResult
	input           textinput.Model
	statusMsg       string
	isErrorStatus   bool
	width           int
	height          int
	quitting        bool
	shouldRestart   bool
	isBusinessAcct  bool

	// Updater state
	updateIsBeta  bool
	updateLogs    []string
	updateDone    bool
	updateResult  *updater.UpdateResult
	updateErr     error
	updateLogChan chan string
	updateFinChan chan updateFinishedMsg
}

// Run launches the modern Bubble Tea standby TUI with responsive layout.
func Run(ctx context.Context, defaultDB string, boundPort int) (SessionResult, bool, error) {
	ti := textinput.New()
	ti.Placeholder = "+15551234567"
	ti.CharLimit = 32
	ti.Width = 32

	dataDir := whatsrook.DefaultDataDir()
	sessions, _ := whatsrook.ListStoredSessions(ctx, dataDir, defaultDB)

	initialState := stateMain
	initialCursor := 0
	if len(sessions) == 0 {
		initialState = stateNewAuth
	}

	m := model{
		ctx:         ctx,
		state:       initialState,
		cursor:      initialCursor,
		defaultDB:   defaultDB,
		boundPort:   boundPort,
		sessions:    sessions,
		currentTime: time.Now(),
		input:       ti,
		result: SessionResult{
			ClientType: whatsrook.ClientChrome,
			Database:   defaultDB,
			Verbose:    false,
		},
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()

	ClearTerminal()

	if err != nil {
		return SessionResult{}, false, err
	}

	fm := finalModel.(model)
	if fm.shouldRestart {
		fm.result.ShouldRestart = true
		return fm.result, false, nil
	}
	return fm.result, fm.result.ShouldRun, nil
}

// ClearTerminal resets terminal state, restores cursor visibility and echo, and clears the screen.
func ClearTerminal() {
	// 1. Show cursor, reset styling, exit alt-screen, disable mouse tracking, clear screen & scrollback
	_, _ = os.Stdout.WriteString("\033[?25h\033[0m\033[?1049l\033[?1000l\033[?1002l\033[?1003l\033[?1006l\033[H\033[2J\033[3J")
	_ = os.Stdout.Sync()

	// 2. Restore standard terminal discipline (echo & sane mode) on Unix/Linux/Termux/macOS
	if runtime.GOOS != "windows" {
		cmd := exec.Command("stty", "sane")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		_ = cmd.Run()

		cmdClear := exec.Command("clear")
		cmdClear.Stdout = os.Stdout
		_ = cmdClear.Run()
	} else {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	}

	// 3. Ensure cursor is definitely visible and normal
	_, _ = os.Stdout.WriteString("\033[?25h\033[0m")
	_ = os.Stdout.Sync()
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		textinput.Blink,
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitForLog(logChan <-chan string, doneChan <-chan updateFinishedMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case line, ok := <-logChan:
			if ok {
				return updateLogChunkMsg(line)
			}
			done := <-doneChan
			return done
		case done := <-doneChan:
			return done
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputWidth := msg.Width - 8
		if inputWidth > 36 {
			inputWidth = 36
		} else if inputWidth < 18 {
			inputWidth = 18
		}
		m.input.Width = inputWidth
		return m, nil

	case tickMsg:
		m.currentTime = time.Time(msg)
		return m, tickCmd()

	case updateLogChunkMsg:
		line := string(msg)
		if line != "" {
			if strings.HasPrefix(line, "[") && len(m.updateLogs) > 0 && strings.HasPrefix(m.updateLogs[len(m.updateLogs)-1], "[") {
				m.updateLogs[len(m.updateLogs)-1] = line
			} else {
				m.updateLogs = append(m.updateLogs, line)
				if len(m.updateLogs) > 30 {
					m.updateLogs = m.updateLogs[len(m.updateLogs)-30:]
				}
			}
		}
		return m, waitForLog(m.updateLogChan, m.updateFinChan)

	case updateFinishedMsg:
		m.updateDone = true
		m.updateResult = msg.result
		m.updateErr = msg.err
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.state == stateMain {
				m.quitting = true
				return m, tea.Quit
			}
			m.state = stateMain
			m.cursor = 0
			m.statusMsg = ""
			return m, nil
		}

		switch m.state {
		case stateMain:
			return m.updateMain(msg)
		case stateSessionsList:
			return m.updateSessionsList(msg)
		case stateSessionActions:
			return m.updateSessionActions(msg)
		case stateEditVariables:
			return m.updateEditVariables(msg)
		case stateEditClient:
			return m.updateEditClient(msg)
		case stateEditLogLevel:
			return m.updateEditLogLevel(msg)
		case stateEditDB:
			return m.updateEditDB(msg)
		case stateDeleteConfirm:
			return m.updateDeleteConfirm(msg)
		case stateNewAuth:
			return m.updateNewAuth(msg)
		case stateNewPhoneInput:
			return m.updateNewPhoneInput(msg)
		case stateNewBusinessAccount:
			return m.updateNewBusinessAccount(msg)
		case stateNewClient:
			return m.updateNewClient(msg)
		case stateNewLogLevel:
			return m.updateNewLogLevel(msg)
		case stateNewDB:
			return m.updateNewDB(msg)
		case stateNewSaveOption:
			return m.updateNewSaveOption(msg)
		case stateUpdateMenu:
			return m.updateUpdateMenu(msg)
		case stateUpdateProgress:
			return m.updateUpdateProgress(msg)
		case stateDependenciesMenu:
			return m.updateDependenciesMenu(msg)
		case stateDependenciesProgress:
			return m.updateDependenciesProgress(msg)
		case stateDonate:
			return m.updateDonate(msg)
		}
	}

	if m.state == stateNewPhoneInput || m.state == stateEditDB || m.state == stateNewDB {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 6 {
			m.cursor++
		}
	case "1":
		m.cursor = 0
		return m.selectMainOption()
	case "2":
		m.cursor = 1
		return m.selectMainOption()
	case "3":
		m.cursor = 2
		return m.selectMainOption()
	case "4":
		m.cursor = 3
		return m.selectMainOption()
	case "5":
		m.cursor = 4
		return m.selectMainOption()
	case "6":
		m.cursor = 5
		return m.selectMainOption()
	case "7", "q", "esc":
		m.quitting = true
		return m, tea.Quit
	case "enter":
		return m.selectMainOption()
	}
	return m, nil
}

func (m model) selectMainOption() (tea.Model, tea.Cmd) {
	m.statusMsg = ""
	switch m.cursor {
	case 0: // Connect to an existing session
		dataDir := whatsrook.DefaultDataDir()
		sessions, err := whatsrook.ListStoredSessions(m.ctx, dataDir, m.defaultDB)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Error reading sessions: %v", err)
			m.isErrorStatus = true
			return m, nil
		}
		if len(sessions) == 0 {
			m.cursor = 0
			m.result = SessionResult{
				ClientType: whatsrook.ClientChrome,
				Database:   m.defaultDB,
				Verbose:    false,
			}
			m.state = stateNewAuth
			m.statusMsg = "No saved sessions found. Starting setup wizard..."
			m.isErrorStatus = false
			return m, nil
		}
		m.sessions = sessions
		m.cursor = 0
		m.state = stateSessionsList
	case 1: // Create a new session
		m.cursor = 0
		m.result = SessionResult{
			ClientType: whatsrook.ClientChrome,
			Database:   m.defaultDB,
			Verbose:    false,
		}
		m.state = stateNewAuth
	case 2: // Check & install updates
		m.cursor = 0
		m.state = stateUpdateMenu
	case 3: // Install dependencies
		m.cursor = 0
		m.state = stateDependenciesMenu
	case 4: // Restart WhatsRook
		m.shouldRestart = true
		m.quitting = true
		return m, tea.Quit
	case 5: // Donate
		m.cursor = 0
		m.state = stateDonate
	case 6: // Exit
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateUpdateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 3 {
			m.cursor++
		}
	case "1":
		m.cursor = 0
		return m.selectUpdateOption()
	case "2":
		m.cursor = 1
		return m.selectUpdateOption()
	case "3":
		m.cursor = 2
		return m.selectUpdateOption()
	case "4", "esc", "b", "0":
		m.state = stateMain
		m.cursor = 2
		m.statusMsg = ""
	case "enter":
		return m.selectUpdateOption()
	}
	return m, nil
}

func (m model) selectUpdateOption() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0: // Check for updates
		var buf bytes.Buffer
		currentChannel := updater.GetStoredChannel()
		up := updater.New(updater.Options{
			Out:     &buf,
			Channel: currentChannel,
		})
		res, err := up.Check(m.ctx)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Update check failed: %v", err)
			m.isErrorStatus = true
		} else if res.HasNewVersion {
			m.statusMsg = fmt.Sprintf("New version available: %s (Current: %s)",
				updater.FormatVersionDisplay(res.LatestVersion),
				updater.FormatVersionDisplay(res.CurrentVersion))
			m.isErrorStatus = false
		} else {
			m.statusMsg = fmt.Sprintf("WhatsRook is up to date (%s • %s)",
				updater.FormatVersionDisplay(res.CurrentVersion), currentChannel)
			m.isErrorStatus = false
		}
		return m, nil

	case 1: // Update to stable (Live Progress)
		_ = updater.SetStoredChannel("stable")
		return m.startLiveUpgrade(false)

	case 2: // Update to alpha / beta (Live Progress)
		_ = updater.SetStoredChannel("beta")
		return m.startLiveUpgrade(true)

	case 3: // Back
		m.state = stateMain
		m.cursor = 2
		m.statusMsg = ""
	}
	return m, nil
}

func (m model) startLiveUpgrade(isBeta bool) (tea.Model, tea.Cmd) {
	m.state = stateUpdateProgress
	m.updateIsBeta = isBeta
	m.updateLogs = []string{"Connecting to release repository..."}
	m.updateDone = false
	m.updateResult = nil
	m.updateErr = nil

	logChan := make(chan string, 100)
	doneChan := make(chan updateFinishedMsg, 1)
	m.updateLogChan = logChan
	m.updateFinChan = doneChan

	go func() {
		writer := &chanWriter{ch: logChan}
		chName := "stable"
		if isBeta {
			chName = "beta"
		}
		up := updater.New(updater.Options{
			Out:     writer,
			Channel: chName,
		})
		res, err := up.Upgrade(context.Background(), isBeta)
		doneChan <- updateFinishedMsg{result: res, err: err}
		close(logChan)
	}()

	return m, waitForLog(logChan, doneChan)
}

func (m model) updateUpdateProgress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.updateDone {
		switch msg.String() {
		case "enter":
			if m.updateResult != nil && m.updateResult.Updated {
				m.shouldRestart = true
				m.quitting = true
				return m, tea.Quit
			}
			m.state = stateUpdateMenu
			m.cursor = 0
			return m, nil
		case "esc", "b", "q":
			m.state = stateUpdateMenu
			m.cursor = 0
			return m, nil
		}
	} else if msg.String() == "esc" {
		m.state = stateUpdateMenu
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m model) updateDependenciesMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 2 {
			m.cursor++
		}
	case "1":
		m.cursor = 0
		return m.selectDependencyOption()
	case "2":
		m.cursor = 1
		return m.selectDependencyOption()
	case "3", "esc", "b", "0":
		m.state = stateMain
		m.cursor = 3
		m.statusMsg = ""
	case "enter":
		return m.selectDependencyOption()
	}
	return m, nil
}

func (m model) selectDependencyOption() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0:
		if missing, ok := missingDependencies(); !ok {
			m.statusMsg = "Unable to determine dependency status."
			m.isErrorStatus = true
		} else if len(missing) == 0 {
			m.statusMsg = "All required dependencies are already installed."
			m.isErrorStatus = false
		} else {
			m.statusMsg = fmt.Sprintf("Missing dependencies: %s", strings.Join(missing, ", "))
			m.isErrorStatus = true
		}
		return m, nil
	case 1:
		return m.startDependencyInstall()
	case 2:
		m.state = stateMain
		m.cursor = 3
		m.statusMsg = ""
	}
	return m, nil
}

func (m model) startDependencyInstall() (tea.Model, tea.Cmd) {
	m.state = stateDependenciesProgress
	m.updateLogs = []string{"Checking required system dependencies..."}
	m.updateDone = false
	m.updateResult = nil
	m.updateErr = nil

	logChan := make(chan string, 100)
	doneChan := make(chan updateFinishedMsg, 1)
	m.updateLogChan = logChan
	m.updateFinChan = doneChan

	go func() {
		writer := &chanWriter{ch: logChan}
		deps, _ := missingDependencies()
		if len(deps) == 0 {
			writer.Write([]byte("[OK] No missing dependencies detected.\n"))
			doneChan <- updateFinishedMsg{}
			close(logChan)
			return
		}
		for _, dep := range deps {
			fmt.Fprintf(writer, "[MISSING] %s not found on PATH.\n", dep)
		}
		writer.Write([]byte("Preparing installation for this system...\n"))
		for _, dep := range deps {
			if err := runDependencyInstall(dep, writer); err != nil {
				fmt.Fprintf(writer, "[ERROR] %s installation failed: %v\n", dep, err)
				doneChan <- updateFinishedMsg{err: err}
				close(logChan)
				return
			}
		}
		refreshWindowsPath()
		if missing, _ := missingDependencies(); len(missing) == 0 {
			writer.Write([]byte("[OK] Required dependencies installed successfully.\n"))
			doneChan <- updateFinishedMsg{}
		} else {
			fmt.Fprintf(writer, "[WARN] Installation completed, but %s is still not detected on PATH.\n", strings.Join(missing, ", "))
			doneChan <- updateFinishedMsg{}
		}
		close(logChan)
	}()

	return m, waitForLog(logChan, doneChan)
}

func (m model) updateDependenciesProgress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.updateDone {
		switch msg.String() {
		case "enter", "esc", "b", "q":
			m.state = stateDependenciesMenu
			m.cursor = 0
			m.statusMsg = ""
			return m, nil
		}
	} else if msg.String() == "esc" || msg.String() == "b" || msg.String() == "q" {
		m.state = stateDependenciesMenu
		m.cursor = 0
		m.statusMsg = ""
		return m, nil
	}
	return m, nil
}

func (m model) updateSessionsList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxIdx := len(m.sessions)
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < maxIdx {
			m.cursor++
		}
	case "esc", "b", "q", "0":
		m.state = stateMain
		m.cursor = 0
		m.statusMsg = ""
	case "enter":
		if m.cursor == maxIdx { // Back
			m.state = stateMain
			m.cursor = 0
			m.statusMsg = ""
			return m, nil
		}
		m.selectedSession = m.sessions[m.cursor]
		phone := "+" + m.selectedSession.User
		clientType := whatsrook.ClientChrome
		if strings.EqualFold(m.selectedSession.Platform, "Android") {
			clientType = whatsrook.ClientAndroid
		} else if strings.EqualFold(m.selectedSession.Platform, "iOS") {
			clientType = whatsrook.ClientIos
		}
		m.result = SessionResult{
			Session:    phone,
			ClientType: clientType,
			Database:   m.defaultDB,
			Verbose:    false,
		}
		m.state = stateSessionActions
		m.cursor = 0
		m.statusMsg = ""
	default:
		if n, err := strconv.Atoi(msg.String()); err == nil && n >= 1 && n <= len(m.sessions) {
			m.cursor = n - 1
			return m.updateSessionsList(tea.KeyMsg{Type: tea.KeyEnter})
		}
	}
	return m, nil
}

func (m model) updateSessionActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 4 {
			m.cursor++
		}
	case "1":
		m.cursor = 0
		return m.selectSessionAction()
	case "2":
		m.cursor = 1
		return m.selectSessionAction()
	case "3":
		m.cursor = 2
		return m.selectSessionAction()
	case "4":
		m.cursor = 3
		return m.selectSessionAction()
	case "0", "esc", "b", "q":
		m.state = stateSessionsList
		m.cursor = 0
		m.statusMsg = ""
	case "enter":
		return m.selectSessionAction()
	}
	return m, nil
}

func (m model) selectSessionAction() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0: // Run session
		m.result.ShouldRun = true
		return m, tea.Quit
	case 1: // Save to .env
		clientStr := "default"
		switch m.result.ClientType {
		case whatsrook.ClientAndroid:
			clientStr = "android"
		case whatsrook.ClientIos:
			clientStr = "ios"
		}
		if err := SaveDotEnv(m.result.Session, clientStr, m.result.Verbose, m.result.Database); err != nil {
			m.statusMsg = fmt.Sprintf("Failed to save .env: %v", err)
			m.isErrorStatus = true
		} else {
			m.statusMsg = "Saved to .env. Run with -i to view menu again."
			m.isErrorStatus = false
		}
	case 2: // Edit variables
		m.state = stateEditVariables
		m.cursor = 0
		m.statusMsg = ""
	case 3: // Delete session
		m.state = stateDeleteConfirm
		m.cursor = 0
		m.statusMsg = ""
	case 4: // Back
		m.state = stateSessionsList
		m.cursor = 0
		m.statusMsg = ""
	}
	return m, nil
}

func (m model) updateEditVariables(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 5 {
			m.cursor++
		}
	case "1":
		m.state = stateEditClient
		m.cursor = 0
	case "2":
		m.state = stateEditLogLevel
		m.cursor = 0
	case "3":
		m.state = stateEditDB
		m.input.CharLimit = 256
		m.input.Width = 50
		m.input.Placeholder = "postgres://user:pass@host:5432/dbname (or 'default')"
		m.input.SetValue(m.result.Database)
		m.input.Focus()
		return m, textinput.Blink
	case "4":
		m.saveCurrentToEnv()
	case "5":
		m.result.ShouldRun = true
		return m, tea.Quit
	case "0", "esc", "b", "q":
		m.state = stateSessionActions
		m.cursor = 2
	case "enter":
		switch m.cursor {
		case 0: // Edit client profile
			m.state = stateEditClient
			m.cursor = 0
		case 1: // Edit logging level
			m.state = stateEditLogLevel
			m.cursor = 0
		case 2: // Edit database
			m.state = stateEditDB
			m.input.CharLimit = 256
			m.input.Width = 50
			m.input.Placeholder = "postgres://user:pass@host:5432/dbname (or 'default')"
			m.input.SetValue(m.result.Database)
			m.input.Focus()
			return m, textinput.Blink
		case 3: // Save to .env
			m.saveCurrentToEnv()
		case 4: // Launch with updated variables
			m.result.ShouldRun = true
			return m, tea.Quit
		case 5: // Back
			m.state = stateSessionActions
			m.cursor = 2
		}
	}
	return m, nil
}

func (m *model) saveCurrentToEnv() {
	clientStr := "default"
	switch m.result.ClientType {
	case whatsrook.ClientAndroid:
		clientStr = "android"
	case whatsrook.ClientIos:
		clientStr = "ios"
	}
	if err := SaveDotEnv(m.result.Session, clientStr, m.result.Verbose, m.result.Database); err != nil {
		m.statusMsg = fmt.Sprintf("Failed to save .env: %v", err)
		m.isErrorStatus = true
	} else {
		m.statusMsg = "Saved to .env. Run with -i to view menu again."
		m.isErrorStatus = false
	}
}

func (m model) updateEditClient(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 2 {
			m.cursor++
		}
	case "1":
		m.result.ClientType = whatsrook.ClientChrome
		m.state = stateEditVariables
		m.cursor = 0
	case "2":
		m.result.ClientType = whatsrook.ClientAndroid
		m.state = stateEditVariables
		m.cursor = 0
	case "3":
		m.result.ClientType = whatsrook.ClientIos
		m.state = stateEditVariables
		m.cursor = 0
	case "esc", "b", "0":
		m.state = stateEditVariables
		m.cursor = 0
	case "enter":
		switch m.cursor {
		case 0:
			m.result.ClientType = whatsrook.ClientChrome
		case 1:
			m.result.ClientType = whatsrook.ClientAndroid
		case 2:
			m.result.ClientType = whatsrook.ClientIos
		}
		m.state = stateEditVariables
		m.cursor = 0
	}
	return m, nil
}

func (m model) updateEditLogLevel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 {
			m.cursor++
		}
	case "1":
		m.result.Verbose = false
		m.state = stateEditVariables
		m.cursor = 1
	case "2":
		m.result.Verbose = true
		m.state = stateEditVariables
		m.cursor = 1
	case "esc", "b", "0":
		m.state = stateEditVariables
		m.cursor = 1
	case "enter":
		m.result.Verbose = m.cursor == 1
		m.state = stateEditVariables
		m.cursor = 1
	}
	return m, nil
}

func (m model) updateEditDB(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if val != "" {
			m.result.Database = val
		}
		m.state = stateEditVariables
		m.cursor = 2
	case "esc":
		m.state = stateEditVariables
		m.cursor = 2
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 {
			m.cursor++
		}
	case "1":
		m.cursor = 0
		return m.selectDeleteConfirm()
	case "2":
		m.cursor = 1
		return m.selectDeleteConfirm()
	case "y", "Y":
		m.cursor = 1
		return m.selectDeleteConfirm()
	case "n", "N", "esc", "b", "0":
		m.state = stateSessionActions
		m.cursor = 3
	case "enter":
		return m.selectDeleteConfirm()
	}
	return m, nil
}

func (m model) selectDeleteConfirm() (tea.Model, tea.Cmd) {
	if m.cursor == 0 { // Cancel and return
		m.state = stateSessionActions
		m.cursor = 3
		return m, nil
	}

	// Confirm deletion
	dataDir := whatsrook.DefaultDataDir()
	phone := "+" + m.selectedSession.User
	if err := whatsrook.DeleteStoredSession(m.ctx, dataDir, m.defaultDB, phone); err != nil {
		m.statusMsg = fmt.Sprintf("Failed to logout and delete session: %v", err)
		m.isErrorStatus = true
	} else {
		m.statusMsg = fmt.Sprintf("Session +%s logged out and erased successfully.", m.selectedSession.User)
		m.isErrorStatus = false
	}
	sessions, _ := whatsrook.ListStoredSessions(m.ctx, dataDir, m.defaultDB)
	m.sessions = sessions
	if len(m.sessions) == 0 {
		m.state = stateMain
		m.cursor = 0
	} else {
		m.state = stateSessionsList
		m.cursor = 0
	}
	return m, nil
}

func (m model) updateNewAuth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 2 {
			m.cursor++
		}
	case "1":
		m.result.QRCode = true
		m.result.Pair = false
		m.state = stateNewPhoneInput
		m.input.CharLimit = 32
		m.input.Width = 32
		m.input.Placeholder = "session phone"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "2":
		m.result.QRCode = false
		m.result.Pair = true
		m.state = stateNewPhoneInput
		m.input.CharLimit = 32
		m.input.Width = 32
		m.input.Placeholder = "+15551234567"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "3", "0", "esc", "b":
		m.state = stateMain
		if len(m.sessions) == 0 {
			m.cursor = 1
		} else {
			m.cursor = 0
		}
	case "enter":
		switch m.cursor {
		case 0: // QR Code
			m.result.QRCode = true
			m.result.Pair = false
			m.state = stateNewPhoneInput
			m.input.CharLimit = 32
			m.input.Width = 32
			m.input.Placeholder = "session phone number"
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case 1: // Pairing Code
			m.result.QRCode = false
			m.result.Pair = true
			m.state = stateNewPhoneInput
			m.input.CharLimit = 32
			m.input.Width = 32
			m.input.Placeholder = "+15551234567"
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case 2: // Back
			m.state = stateMain
			if len(m.sessions) == 0 {
				m.cursor = 1
			} else {
				m.cursor = 0
			}
		}
	}
	return m, nil
}

func (m model) updateNewPhoneInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if m.result.Pair {
			if val == "" {
				m.statusMsg = "Phone number is required. Use a valid international format (e.g. +15551234567)."
				m.isErrorStatus = true
				return m, nil
			}
			clean := strings.TrimPrefix(val, "+")
			if len(clean) < 7 || len(clean) > 15 || !isNumeric(clean) {
				m.statusMsg = "Invalid phone number. Use a valid international format (e.g. +15551234567)."
				m.isErrorStatus = true
				return m, nil
			}
			if !strings.HasPrefix(val, "+") {
				val = "+" + val
			}
			m.result.Session = val
		} else {
			if val == "" {
				val = "session_" + strconv.FormatInt(time.Now().Unix(), 10)
			}
			m.result.Session = val
		}
		m.statusMsg = ""
		m.state = stateNewBusinessAccount
		m.cursor = 0
		return m, nil
	case "esc":
		m.state = stateNewAuth
		m.cursor = 0
		m.statusMsg = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateNewBusinessAccount(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 {
			m.cursor++
		}
	case "1":
		m.isBusinessAcct = false
		m.state = stateNewClient
		m.cursor = 0
	case "2":
		m.isBusinessAcct = true
		m.state = stateNewClient
		m.cursor = 0
	case "esc", "b", "0":
		m.state = stateNewPhoneInput
		m.input.Focus()
		return m, textinput.Blink
	case "enter":
		m.isBusinessAcct = m.cursor == 1
		m.state = stateNewClient
		m.cursor = 0
	}
	return m, nil
}

func (m model) updateNewClient(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		maxCursor := 2
		if m.isBusinessAcct {
			maxCursor = 0
		}
		if m.cursor < maxCursor {
			m.cursor++
		}
	case "1":
		m.result.ClientType = whatsrook.ClientChrome
		m.state = stateNewLogLevel
		m.cursor = 0
	case "2":
		if !m.isBusinessAcct {
			m.result.ClientType = whatsrook.ClientAndroid
			m.state = stateNewLogLevel
			m.cursor = 0
		}
	case "3":
		if !m.isBusinessAcct {
			m.result.ClientType = whatsrook.ClientIos
			m.state = stateNewLogLevel
			m.cursor = 0
		}
	case "esc", "b", "0":
		m.state = stateNewBusinessAccount
		m.cursor = 0
	case "enter":
		switch m.cursor {
		case 0:
			m.result.ClientType = whatsrook.ClientChrome
		case 1:
			if !m.isBusinessAcct {
				m.result.ClientType = whatsrook.ClientAndroid
			}
		case 2:
			if !m.isBusinessAcct {
				m.result.ClientType = whatsrook.ClientIos
			}
		}
		m.state = stateNewLogLevel
		m.cursor = 0
	}
	return m, nil
}

func (m model) updateNewLogLevel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 {
			m.cursor++
		}
	case "1":
		m.result.Verbose = false
		m.state = stateNewDB
		m.cursor = 0
		return m.setupNewDBInput()
	case "2":
		m.result.Verbose = true
		m.state = stateNewDB
		m.cursor = 0
		return m.setupNewDBInput()
	case "esc", "b", "0":
		m.state = stateNewClient
		m.cursor = 0
	case "enter":
		m.result.Verbose = m.cursor == 1
		m.state = stateNewDB
		m.cursor = 0
		return m.setupNewDBInput()
	}
	return m, nil
}

func (m *model) setupNewDBInput() (tea.Model, tea.Cmd) {
	m.input.CharLimit = 256
	m.input.Width = 50
	m.input.Placeholder = "postgres://user:pass@host:5432/dbname (or 'default')"
	dbVal := m.result.Database
	if dbVal == "" {
		dbVal = m.defaultDB
	}
	if dbVal == "" {
		dbVal = "default"
	}
	m.input.SetValue(dbVal)
	m.input.Focus()
	return *m, textinput.Blink
}

func (m model) updateNewDB(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if val != "" {
			m.result.Database = val
		} else if m.result.Database == "" {
			m.result.Database = "default"
		}
		m.state = stateNewSaveOption
		m.cursor = 0
		m.statusMsg = ""
		return m, nil
	case "esc":
		m.state = stateNewLogLevel
		m.cursor = 0
		m.statusMsg = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateNewSaveOption(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 2 {
			m.cursor++
		}
	case "1":
		m.result.ShouldRun = true
		return m, tea.Quit
	case "2":
		m.saveCurrentToEnv()
		m.result.ShouldRun = true
		return m, tea.Quit
	case "esc", "b", "0":
		m.state = stateNewDB
		m.cursor = 0
		return m.setupNewDBInput()
	case "enter":
		switch m.cursor {
		case 0: // Launch session immediately
			m.result.ShouldRun = true
			return m, tea.Quit
		case 1: // Save to .env and launch session
			m.saveCurrentToEnv()
			m.result.ShouldRun = true
			return m, tea.Quit
		case 2: // Back
			m.state = stateNewDB
			m.cursor = 0
			return m.setupNewDBInput()
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var s strings.Builder

	// Top Responsive Dynamic Header
	portStr := strconv.Itoa(m.boundPort)
	timeStr := m.currentTime.Format("15:04:05")

	var headerText string
	if m.width > 0 && m.width < 45 {
		headerText = fmt.Sprintf(
			"%s %s",
			headerTitleStyle.Render("WHATSROOK"),
			headerTimeStyle.Render(timeStr),
		)
	} else if m.width > 0 && m.width < 65 {
		headerText = fmt.Sprintf(
			"%s %s %s",
			headerTitleStyle.Render("WHATSROOK STANDBY"),
			headerMutedStyle.Render(":"+portStr),
			headerTimeStyle.Render(timeStr),
		)
	} else {
		headerText = fmt.Sprintf(
			"%s %s %s %s %s",
			headerTitleStyle.Render("WHATSROOK STANDBY"),
			headerMutedStyle.Render("• PORT :"+portStr),
			headerMutedStyle.Render("• WAITING FOR SESSION"),
			headerMutedStyle.Render("•"),
			headerTimeStyle.Render(timeStr),
		)
	}

	if m.width > 0 {
		boxWidth := m.width - 2
		if boxWidth > 74 {
			boxWidth = 74
		} else if boxWidth < 20 {
			boxWidth = 20
		}
		s.WriteString(headerBox.Width(boxWidth).Render(headerText))
	} else {
		s.WriteString(headerBox.Render(headerText))
	}
	s.WriteByte('\n')

	switch m.state {
	case stateMain:
		s.WriteString(m.viewMain())
	case stateSessionsList:
		s.WriteString(m.viewSessionsList())
	case stateSessionActions:
		s.WriteString(m.viewSessionActions())
	case stateEditVariables:
		s.WriteString(m.viewEditVariables())
	case stateEditClient:
		s.WriteString(m.viewEditClient())
	case stateEditLogLevel:
		s.WriteString(m.viewEditLogLevel())
	case stateEditDB:
		s.WriteString(m.viewEditDB())
	case stateDeleteConfirm:
		s.WriteString(m.viewDeleteConfirm())
	case stateNewAuth:
		s.WriteString(m.viewNewAuth())
	case stateNewPhoneInput:
		s.WriteString(m.viewNewPhoneInput())
	case stateNewBusinessAccount:
		s.WriteString(m.viewNewBusinessAccount())
	case stateNewClient:
		s.WriteString(m.viewNewClient())
	case stateNewLogLevel:
		s.WriteString(m.viewNewLogLevel())
	case stateNewDB:
		s.WriteString(m.viewNewDB())
	case stateNewSaveOption:
		s.WriteString(m.viewNewSaveOption())
	case stateUpdateMenu:
		s.WriteString(m.viewUpdateMenu())
	case stateUpdateProgress:
		s.WriteString(m.viewUpdateProgress())
	case stateDependenciesMenu:
		s.WriteString(m.viewDependenciesMenu())
	case stateDependenciesProgress:
		s.WriteString(m.viewDependenciesProgress())
	case stateDonate:
		s.WriteString(m.viewDonate())
	}

	if m.statusMsg != "" {
		if m.isErrorStatus {
			s.WriteString(errorStyle.Render("! " + m.statusMsg))
		} else {
			s.WriteString(successStyle.Render("✓ " + m.statusMsg))
		}
		s.WriteByte('\n')
	}

	return s.String()
}

// renderDotItem renders a clean active (●) or inactive (○) focused item.
func renderDotItem(active bool, text string) string {
	if active {
		return "  " + activeDotStyle.Render("●") + " " + activeItemStyle.Render(text) + "\n"
	}
	return "  " + inactiveDotStyle.Render("○") + " " + inactiveItemStyle.Render(text) + "\n"
}

func (m model) getHelpText(action string) string {
	if m.width > 0 && m.width < 55 {
		if action != "" {
			return fmt.Sprintf("↑/↓ move • Enter %s • Esc back", action)
		}
		return "↑/↓ move • Enter select • Esc back"
	}
	if action != "" {
		return fmt.Sprintf("Use ↑/↓ to navigate • Enter to %s • Esc to go back", action)
	}
	return "Use ↑/↓ to navigate • Enter to select • Esc/q to exit"
}

func (m model) viewMain() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("MAIN MENU"))
	s.WriteString("\n\n")

	options := []string{
		"Connect to an existing session",
		"Create a new session",
		"Check & install updates",
		"Install system dependencies",
		"Restart WhatsRook",
		"Donate to support this project",
		"Exit",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewUpdateMenu() string {
	var s strings.Builder
	currentChannel := updater.GetStoredChannel()
	s.WriteString(titleStyle.Render(fmt.Sprintf("UPDATER (Channel: %s)", currentChannel)))
	s.WriteString("\n\n")

	options := []string{
		"Check for updates",
		"Update to latest stable release",
		"Update to latest alpha / nightly build",
		"Back to main menu",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewDependenciesMenu() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SYSTEM DEPENDENCIES"))
	s.WriteString("\n\n")
	missing, ok := missingDependencies()
	statusText := "All required dependencies are installed."
	if !ok {
		statusText = "Dependency status could not be checked."
	} else if len(missing) > 0 {
		statusText = fmt.Sprintf("Missing: %s", strings.Join(missing, ", "))
	}
	s.WriteString(activeItemStyle.Render(statusText))
	s.WriteString("\n\n")

	options := []string{
		"Check required dependencies",
		"Install missing dependencies",
		"Back to main menu",
	}
	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewUpdateProgress() string {
	var s strings.Builder
	channelName := "Stable Release"
	if m.updateIsBeta {
		channelName = "Alpha / Nightly Build"
	}
	s.WriteString(titleStyle.Render(fmt.Sprintf("UPDATING WHATSROOK (%s)", strings.ToUpper(channelName))))
	s.WriteString("\n\n")

	contentWidth := m.width - 4
	if contentWidth > 74 {
		contentWidth = 74
	} else if contentWidth < 20 {
		contentWidth = 20
	}

	logLineStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Width(contentWidth)

	if m.updateDone {
		if m.updateErr != nil {
			s.WriteString(errorStyle.Width(contentWidth).Render(fmt.Sprintf("! Upgrade error: %v", m.updateErr)))
			s.WriteString("\n\n")
			s.WriteString(helpStyle.Render("Press [Enter] or [Esc] to return to updater menu"))
		} else if m.updateResult != nil && m.updateResult.Updated {
			s.WriteString(successStyle.Width(contentWidth).Render(fmt.Sprintf("✓ Upgrade successful! Version: %s", updater.FormatVersionDisplay(m.updateResult.LatestVersion))))
			s.WriteString("\n\n")
			s.WriteString(activeItemStyle.Width(contentWidth).Render("  Press [Enter] to restart WhatsRook with new version now."))
			s.WriteString("\n\n")
			s.WriteString(helpStyle.Render("Press [Enter] to restart • [Esc] to exit"))
		} else if m.updateResult != nil {
			s.WriteString(successStyle.Width(contentWidth).Render(fmt.Sprintf("✓ Already at the latest version (%s).", updater.FormatVersionDisplay(m.updateResult.CurrentVersion))))
			s.WriteString("\n\n")
			s.WriteString(helpStyle.Render("Press [Enter] or [Esc] to return to updater menu"))
		}
	} else {
		s.WriteString(activeItemStyle.Render("  ● Update in progress..."))
		s.WriteString("\n\n")

		// Render streaming log lines
		s.WriteString(headerMutedStyle.Render("  Live Output:"))
		s.WriteByte('\n')

		maxLines := 8
		if m.height > 0 && m.height < 24 {
			maxLines = 4
		} else if m.height >= 35 {
			maxLines = 14
		}

		visibleLogs := m.updateLogs
		if len(visibleLogs) > maxLines {
			visibleLogs = visibleLogs[len(visibleLogs)-maxLines:]
		}

		for _, logLine := range visibleLogs {
			s.WriteString("  ")
			s.WriteString(logLineStyle.Render(logLine))
			s.WriteByte('\n')
		}
		s.WriteByte('\n')
		s.WriteString(helpStyle.Render(m.getHelpText("")))
	}

	return s.String()
}

func (m model) viewDependenciesProgress() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("INSTALLING DEPENDENCIES"))
	s.WriteString("\n\n")

	contentWidth := m.width - 4
	if contentWidth > 74 {
		contentWidth = 74
	} else if contentWidth < 20 {
		contentWidth = 20
	}

	logLineStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Width(contentWidth)

	if m.updateDone {
		if m.updateErr != nil {
			s.WriteString(errorStyle.Width(contentWidth).Render(fmt.Sprintf("! Dependency install failed: %v", m.updateErr)))
			s.WriteString("\n\n")
			s.WriteString(helpStyle.Render("Press [Enter] or [Esc] to return to dependencies menu"))
		} else {
			s.WriteString(successStyle.Width(contentWidth).Render("✓ Dependency installation completed."))
			s.WriteString("\n\n")
			s.WriteString(helpStyle.Render("Press [Enter] or [Esc] to return to dependencies menu"))
		}
	} else {
		s.WriteString(activeItemStyle.Render("  ● Dependency install in progress..."))
		s.WriteString("\n\n")
		s.WriteString(headerMutedStyle.Render("  Live Output:"))
		s.WriteByte('\n')

		maxLines := 8
		if m.height > 0 && m.height < 24 {
			maxLines = 4
		} else if m.height >= 35 {
			maxLines = 14
		}

		visibleLogs := m.updateLogs
		if len(visibleLogs) > maxLines {
			visibleLogs = visibleLogs[len(visibleLogs)-maxLines:]
		}
		for _, logLine := range visibleLogs {
			s.WriteString("  ")
			s.WriteString(logLineStyle.Render(logLine))
			s.WriteByte('\n')
		}
		s.WriteByte('\n')
		s.WriteString(helpStyle.Render(m.getHelpText("")))
	}

	return s.String()
}

func (m model) viewSessionsList() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SAVED WHATSAPP SESSIONS"))
	s.WriteString("\n\n")

	for i, sess := range m.sessions {
		name := sess.PushName
		if name == "" {
			name = "Personal"
		}
		line := fmt.Sprintf("+%s (%s • %s)", sess.User, name, sess.Platform)
		s.WriteString(renderDotItem(m.cursor == i, line))
	}

	// Back option
	backIdx := len(m.sessions)
	s.WriteString(renderDotItem(m.cursor == backIdx, "Back to main menu"))

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewSessionActions() string {
	var s strings.Builder
	name := m.selectedSession.PushName
	if name == "" {
		name = "Personal"
	}
	title := fmt.Sprintf("SESSION: +%s (%s)", m.selectedSession.User, name)
	s.WriteString(titleStyle.Render(title))
	s.WriteString("\n\n")

	options := []string{
		"Run session",
		"Save to .env as default",
		"Edit session variables",
		"Logout & delete session",
		"Back",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewEditVariables() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render(fmt.Sprintf("CONFIG (+%s)", m.selectedSession.User)))
	s.WriteString("\n\n")

	clientName := "Default (Chrome)"
	switch m.result.ClientType {
	case whatsrook.ClientAndroid:
		clientName = "Android"
	case whatsrook.ClientIos:
		clientName = "iOS"
	}

	logLevel := "Standard (INFO)"
	if m.result.Verbose {
		logLevel = "Verbose (DEBUG)"
	}

	dbName := m.result.Database
	if dbName == "" || dbName == "default" {
		dbName = "default (PostgreSQL)"
	}

	options := []string{
		fmt.Sprintf("Client Profile: %s", clientName),
		fmt.Sprintf("Logging Level: %s", logLevel),
		fmt.Sprintf("Database: %s", dbName),
		"Save configuration to .env",
		"Launch session with updated variables",
		"Back",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("modify")))
	return s.String()
}

func (m model) viewEditClient() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SELECT CLIENT PROFILE"))
	s.WriteString("\n\n")

	options := []string{
		"Default (Desktop / Chrome)",
		"Android Phone",
		"iPhone (iOS)",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("apply")))
	return s.String()
}

func (m model) viewEditLogLevel() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SELECT LOGGING LEVEL"))
	s.WriteString("\n\n")

	options := []string{
		"Standard (INFO)",
		"Verbose (DEBUG)",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("apply")))
	return s.String()
}

func (m model) viewEditDB() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("EDIT DATABASE CONNECTION"))
	s.WriteString("\n\n")
	s.WriteString(inputPromptStyle.Render("PostgreSQL URL (or 'default'):"))
	s.WriteByte('\n')
	s.WriteString("  ")
	s.WriteString(m.input.View())
	s.WriteString("\n\n")
	s.WriteString(helpStyle.Render(m.getHelpText("apply")))
	return s.String()
}

func (m model) viewDeleteConfirm() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("CONFIRM SESSION LOGOUT & DELETION"))
	s.WriteString("\n\n")
	s.WriteString(errorStyle.Render(
		fmt.Sprintf("Are you sure you want to log out and erase +%s?", m.selectedSession.User),
	))
	s.WriteString("\n\n")
	s.WriteString(headerMutedStyle.Render("  This unlinks the companion from WhatsApp servers and removes all local database records.\n\n"))
	s.WriteString(renderDotItem(m.cursor == 0, "Cancel and return"))
	s.WriteString(renderDotItem(m.cursor == 1, "Confirm logout & delete"))
	s.WriteString("\n")
	s.WriteString(helpStyle.Render(m.getHelpText("confirm")))
	return s.String()
}

func (m model) viewNewAuth() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("CREATE NEW SESSION"))
	s.WriteString("\n\n")

	options := []string{
		"QR Code (scan with WhatsApp)",
		"Pairing Code (enter phone number)",
		"Back to main menu",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewNewPhoneInput() string {
	var s strings.Builder
	if m.result.Pair {
		s.WriteString(titleStyle.Render("ENTER PHONE FOR PAIRING CODE"))
		s.WriteString("\n\n")
		s.WriteString(inputPromptStyle.Render("Phone Number (e.g. +15551234567):"))
	} else {
		s.WriteString(titleStyle.Render("ENTER SESSION IDENTIFIER"))
		s.WriteString("\n\n")
		s.WriteString(inputPromptStyle.Render("Session Name:"))
	}
	s.WriteByte('\n')
	s.WriteString("  ")
	s.WriteString(m.input.View())
	s.WriteString("\n\n")
	s.WriteString(helpStyle.Render(m.getHelpText("continue")))
	return s.String()
}

func (m model) viewNewBusinessAccount() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("ACCOUNT TYPE"))
	s.WriteString("\n\n")

	options := []string{
		"Regular WhatsApp Account",
		"WhatsApp Business Account",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewNewClient() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SELECT CLIENT PROFILE"))
	s.WriteString("\n\n")

	if m.isBusinessAcct {
		s.WriteString(titleStyle.Render("BUSINESS ACCOUNT: CHROME ONLY"))
		s.WriteString("\n\n")
		options := []string{
			"Default (Desktop / Chrome)",
		}
		for i, opt := range options {
			s.WriteString(renderDotItem(m.cursor == i, opt))
		}
	} else {
		options := []string{
			"Default (Desktop / Chrome)",
			"Android Phone",
			"iPhone (iOS)",
		}
		for i, opt := range options {
			s.WriteString(renderDotItem(m.cursor == i, opt))
		}
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewNewLogLevel() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SELECT LOGGING LEVEL"))
	s.WriteString("\n\n")

	options := []string{
		"Standard (INFO)",
		"Verbose (DEBUG)",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) viewNewDB() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("CONFIGURE DATABASE CONNECTION"))
	s.WriteString("\n\n")
	s.WriteString(inputPromptStyle.Render("PostgreSQL URL (press Enter for default):"))
	s.WriteByte('\n')
	s.WriteString("  ")
	s.WriteString(m.input.View())
	s.WriteString("\n\n")
	s.WriteString(helpStyle.Render(m.getHelpText("continue")))
	return s.String()
}

func (m model) viewNewSaveOption() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SAVE CONFIGURATION & LAUNCH"))
	s.WriteString("\n\n")

	dbName := m.result.Database
	if dbName == "" || dbName == "default" {
		dbName = "default (PostgreSQL)"
	}

	s.WriteString(headerMutedStyle.Render(fmt.Sprintf("  • Session:  %s\n", m.result.Session)))
	s.WriteString(headerMutedStyle.Render(fmt.Sprintf("  • Database: %s\n\n", dbName)))

	options := []string{
		"Launch session immediately",
		"Save configuration to .env and launch",
		"Back",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render(m.getHelpText("select")))
	return s.String()
}

func (m model) updateDonate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b", "0", "q":
		m.state = stateMain
		m.cursor = 5
	case "enter":
		m.state = stateMain
		m.cursor = 5
	}
	return m, nil
}

func (m model) viewDonate() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SUPPORT THIS PROJECT"))
	s.WriteString("\n\n")

	donateURL := "https://github.com/Thruqe#support-this-project"
	s.WriteString("Thank you for using WhatsRook!\n")
	s.WriteString("If you find this project useful, please consider supporting it:\n\n")
	s.WriteString("  ")
	s.WriteString(activeItemStyle.Render(donateURL))
	s.WriteString("\n\n")
	s.WriteString("You can:\n")
	s.WriteString("  • Star the project on GitHub\n")
	s.WriteString("  • Become a sponsor\n")
	s.WriteString("  • Make a donation\n\n")

	s.WriteString(helpStyle.Render("Press [Esc] to go back"))
	return s.String()
}

func refreshWindowsPath() {
	if runtime.GOOS != "windows" {
		return
	}
	var extraPaths []string
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		extraPaths = append(extraPaths,
			filepath.Join(localAppData, "Microsoft", "WinGet", "Links"),
			filepath.Join(localAppData, "Programs", "ffmpeg", "bin"),
			filepath.Join(localAppData, "whatsrook", "bin"),
		)
	}
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		extraPaths = append(extraPaths,
			filepath.Join(userProfile, "scoop", "shims"),
			filepath.Join(userProfile, "scoop", "apps", "ffmpeg", "current", "bin"),
		)
	}
	if progData := os.Getenv("ProgramData"); progData != "" {
		extraPaths = append(extraPaths,
			filepath.Join(progData, "chocolatey", "bin"),
		)
	}
	if progFiles := os.Getenv("ProgramFiles"); progFiles != "" {
		extraPaths = append(extraPaths,
			filepath.Join(progFiles, "ffmpeg", "bin"),
		)
	}

	curPath := os.Getenv("PATH")
	pathList := filepath.SplitList(curPath)
	pathSet := make(map[string]bool, len(pathList))
	for _, p := range pathList {
		pathSet[filepath.Clean(p)] = true
	}

	changed := false
	for _, extra := range extraPaths {
		clean := filepath.Clean(extra)
		if !pathSet[clean] {
			if fi, err := os.Stat(clean); err == nil && fi.IsDir() {
				curPath = clean + string(os.PathListSeparator) + curPath
				pathSet[clean] = true
				changed = true
			}
		}
	}
	if changed {
		_ = os.Setenv("PATH", curPath)
	}
}

func missingDependencies() ([]string, bool) {
	refreshWindowsPath()
	deps := []string{"ffmpeg"}
	missing := make([]string, 0, len(deps))
	for _, dep := range deps {
		if _, err := exec.LookPath(dep); err != nil {
			missing = append(missing, dep)
		}
	}
	return missing, true
}

func runDependencyInstall(dep string, writer io.Writer) error {
	if dep != "ffmpeg" {
		return fmt.Errorf("unsupported dependency: %s", dep)
	}

	switch runtime.GOOS {
	case "windows":
		var attempts []struct {
			name string
			cmd  *exec.Cmd
		}

		if commandExists("winget") {
			attempts = append(attempts,
				struct {
					name string
					cmd  *exec.Cmd
				}{
					name: "WinGet (Gyan.FFmpeg)",
					cmd:  exec.Command("winget", "install", "--id", "Gyan.FFmpeg", "-e", "--accept-source-agreements", "--accept-package-agreements"),
				},
				struct {
					name string
					cmd  *exec.Cmd
				}{
					name: "WinGet (ffmpeg)",
					cmd:  exec.Command("winget", "install", "ffmpeg", "--accept-source-agreements", "--accept-package-agreements"),
				},
			)
		}
		if commandExists("choco") {
			attempts = append(attempts, struct {
				name string
				cmd  *exec.Cmd
			}{
				name: "Chocolatey (choco install ffmpeg)",
				cmd:  exec.Command("choco", "install", "ffmpeg", "-y"),
			})
		}
		if commandExists("scoop") {
			attempts = append(attempts, struct {
				name string
				cmd  *exec.Cmd
			}{
				name: "Scoop (scoop install ffmpeg)",
				cmd:  exec.Command("scoop", "install", "ffmpeg"),
			})
		}

		var lastErr error
		for _, attempt := range attempts {
			_, _ = fmt.Fprintf(writer, "==> Attempting installation via %s...\n", attempt.name)
			attempt.cmd.Stdout = writer
			attempt.cmd.Stderr = writer
			if err := attempt.cmd.Run(); err == nil {
				refreshWindowsPath()
				if _, ok := exec.LookPath("ffmpeg"); ok == nil {
					return nil
				}
			} else {
				lastErr = err
				_, _ = fmt.Fprintf(writer, "[WARN] %s failed: %v\n", attempt.name, err)
			}
		}

		// Fallback: automated PowerShell direct download and installation for Windows
		if commandExists("powershell") {
			_, _ = fmt.Fprintln(writer, "==> Attempting direct installation via PowerShell...")
			psScript := `$dest = Join-Path $env:LOCALAPPDATA 'whatsrook\bin'; ` +
				`if (-not (Test-Path $dest)) { New-Item -ItemType Directory -Path $dest -Force | Out-Null }; ` +
				`$zip = Join-Path $env:TEMP 'ffmpeg-release-essentials.zip'; ` +
				`Write-Host 'Downloading FFmpeg binary...'; ` +
				`Invoke-WebRequest -Uri 'https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip' -OutFile $zip -UseBasicParsing; ` +
				`Write-Host 'Extracting archive...'; ` +
				`$tmpExtract = Join-Path $env:TEMP 'ffmpeg-extract'; ` +
				`if (Test-Path $tmpExtract) { Remove-Item -Recurse -Force $tmpExtract }; ` +
				`Expand-Archive -Path $zip -DestinationPath $tmpExtract -Force; ` +
				`$exe = Get-ChildItem -Path $tmpExtract -Filter 'ffmpeg.exe' -Recurse | Select-Object -First 1; ` +
				`if ($exe) { Copy-Item -Path $exe.FullName -Destination (Join-Path $dest 'ffmpeg.exe') -Force; Write-Host 'FFmpeg installed to' $dest } ` +
				`else { throw 'ffmpeg.exe not found in downloaded package' }; ` +
				`Remove-Item -Force $zip -ErrorAction SilentlyContinue; ` +
				`Remove-Item -Recurse -Force $tmpExtract -ErrorAction SilentlyContinue; ` +
				`$userPath = [Environment]::GetEnvironmentVariable('Path', 'User'); ` +
				`if ($userPath -notlike ('*' + $dest + '*')) { [Environment]::SetEnvironmentVariable('Path', $dest + ';' + $userPath, 'User') }`

			psCmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
			psCmd.Stdout = writer
			psCmd.Stderr = writer
			if err := psCmd.Run(); err == nil {
				refreshWindowsPath()
				if _, ok := exec.LookPath("ffmpeg"); ok == nil {
					return nil
				}
			} else {
				lastErr = err
				_, _ = fmt.Fprintf(writer, "[WARN] PowerShell installer failed: %v\n", err)
			}
		}

		if lastErr != nil {
			return lastErr
		}
		return fmt.Errorf("no supported Windows installer (winget, choco, scoop, powershell) succeeded")

	case "darwin":
		if commandExists("brew") {
			_, _ = fmt.Fprintln(writer, "==> Installing FFmpeg via Homebrew...")
			cmd := exec.Command("brew", "install", "ffmpeg")
			cmd.Stdout = writer
			cmd.Stderr = writer
			return cmd.Run()
		}
		if commandExists("port") {
			_, _ = fmt.Fprintln(writer, "==> Installing FFmpeg via MacPorts...")
			cmd := exec.Command("sudo", "port", "install", "ffmpeg")
			cmd.Stdout = writer
			cmd.Stderr = writer
			return cmd.Run()
		}
		return fmt.Errorf("no supported macOS package manager found (Homebrew or MacPorts required)")

	case "linux":
		candidates := []struct {
			name string
			cmd  *exec.Cmd
		}{
			{"apt-get", exec.Command("bash", "-lc", "sudo apt-get update && sudo apt-get install -y ffmpeg")},
			{"dnf", exec.Command("bash", "-lc", "sudo dnf install -y ffmpeg")},
			{"yum", exec.Command("bash", "-lc", "sudo yum install -y ffmpeg")},
			{"pacman", exec.Command("bash", "-lc", "sudo pacman -Sy --noconfirm ffmpeg")},
			{"apk", exec.Command("bash", "-lc", "sudo apk add --no-cache ffmpeg")},
			{"zypper", exec.Command("bash", "-lc", "sudo zypper install -y ffmpeg")},
		}
		for _, c := range candidates {
			if commandExists(c.name) {
				_, _ = fmt.Fprintf(writer, "==> Installing FFmpeg via %s...\n", c.name)
				c.cmd.Stdout = writer
				c.cmd.Stderr = writer
				return c.cmd.Run()
			}
		}
		return fmt.Errorf("no supported Linux package manager found for FFmpeg installation")

	case "android":
		if commandExists("pkg") {
			_, _ = fmt.Fprintln(writer, "==> Installing FFmpeg via Termux pkg...")
			cmd := exec.Command("pkg", "install", "-y", "ffmpeg")
			cmd.Stdout = writer
			cmd.Stderr = writer
			return cmd.Run()
		}
		if commandExists("apt-get") {
			_, _ = fmt.Fprintln(writer, "==> Installing FFmpeg via apt-get...")
			cmd := exec.Command("bash", "-lc", "apt-get update && apt-get install -y ffmpeg")
			cmd.Stdout = writer
			cmd.Stderr = writer
			return cmd.Run()
		}
		return fmt.Errorf("no supported Android package manager found for FFmpeg installation")

	default:
		return fmt.Errorf("unsupported platform %s for automatic dependency installation", runtime.GOOS)
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
