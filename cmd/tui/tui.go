package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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
	stateNewClient
	stateNewLogLevel
	stateNewSaveOption
	stateUpdateMenu
	stateUpdateProgress
)

// SessionResult contains the final configured session parameters to launch.
type SessionResult struct {
	Session    string
	Pair       bool
	QRCode     bool
	ClientType whatsrook.ClientType
	Database   string
	Verbose    bool
	ShouldRun  bool
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
	ti.Placeholder = "+2348062795602"
	ti.CharLimit = 32
	ti.Width = 32

	m := model{
		ctx:         ctx,
		state:       stateMain,
		defaultDB:   defaultDB,
		boundPort:   boundPort,
		currentTime: time.Now(),
		input:       ti,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()

	ClearTerminal()

	if err != nil {
		return SessionResult{}, false, err
	}

	fm := finalModel.(model)
	return fm.result, fm.result.ShouldRun, nil
}

// ClearTerminal resets and clears the entire terminal display and scrollback buffer.
func ClearTerminal() {
	_, _ = os.Stdout.WriteString("\033c\033[H\033[2J\033[3J")
	_ = os.Stdout.Sync()

	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	} else {
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	}
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
		case stateNewClient:
			return m.updateNewClient(msg)
		case stateNewLogLevel:
			return m.updateNewLogLevel(msg)
		case stateNewSaveOption:
			return m.updateNewSaveOption(msg)
		case stateUpdateMenu:
			return m.updateUpdateMenu(msg)
		case stateUpdateProgress:
			return m.updateUpdateProgress(msg)
		}
	}

	if m.state == stateNewPhoneInput || m.state == stateEditDB {
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
		if m.cursor < 4 {
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
	case "5", "q", "esc":
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
			m.statusMsg = "No saved sessions found. Create a new session to begin."
			m.isErrorStatus = true
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
	case 3: // Restart WhatsRook
		ClearTerminal()
		_ = updater.RestartProcess()
		os.Exit(0)
	case 4: // Exit
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
			m.statusMsg = fmt.Sprintf("New version available: v%s (Current: v%s)", res.LatestVersion, res.CurrentVersion)
			m.isErrorStatus = false
		} else {
			m.statusMsg = fmt.Sprintf("WhatsRook is up to date (v%s • %s)", res.CurrentVersion, currentChannel)
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
				ClearTerminal()
				_ = updater.RestartProcess()
				os.Exit(0)
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
		if m.result.ClientType == whatsrook.ClientAndroid {
			clientStr = "android"
		} else if m.result.ClientType == whatsrook.ClientIos {
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
	if m.result.ClientType == whatsrook.ClientAndroid {
		clientStr = "android"
	} else if m.result.ClientType == whatsrook.ClientIos {
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
		m.statusMsg = fmt.Sprintf("Failed to delete session: %v", err)
		m.isErrorStatus = true
	} else {
		m.statusMsg = fmt.Sprintf("Session +%s removed successfully.", m.selectedSession.User)
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
		m.input.Placeholder = "session name / phone (optional)"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "2":
		m.result.QRCode = false
		m.result.Pair = true
		m.state = stateNewPhoneInput
		m.input.Placeholder = "+2348062795602"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "3", "0", "esc", "b":
		m.state = stateMain
		m.cursor = 1
	case "enter":
		switch m.cursor {
		case 0: // QR Code
			m.result.QRCode = true
			m.result.Pair = false
			m.state = stateNewPhoneInput
			m.input.Placeholder = "session name / phone (optional)"
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case 1: // Pairing Code
			m.result.QRCode = false
			m.result.Pair = true
			m.state = stateNewPhoneInput
			m.input.Placeholder = "+2348062795602"
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case 2: // Back
			m.state = stateMain
			m.cursor = 1
		}
	}
	return m, nil
}

func (m model) updateNewPhoneInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if m.result.Pair {
			clean := strings.TrimPrefix(val, "+")
			if len(clean) < 7 || len(clean) > 15 || !isNumeric(clean) {
				m.statusMsg = "Invalid phone number (e.g. +2348062795602)."
				m.isErrorStatus = true
				return m, nil
			}
			m.result.Session = val
		} else {
			if val == "" {
				val = "session_" + strconv.FormatInt(time.Now().Unix(), 10)
			}
			m.result.Session = val
		}
		m.statusMsg = ""
		m.state = stateNewClient
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

func (m model) updateNewClient(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.state = stateNewLogLevel
		m.cursor = 0
	case "2":
		m.result.ClientType = whatsrook.ClientAndroid
		m.state = stateNewLogLevel
		m.cursor = 0
	case "3":
		m.result.ClientType = whatsrook.ClientIos
		m.state = stateNewLogLevel
		m.cursor = 0
	case "esc", "b", "0":
		m.state = stateNewPhoneInput
		m.input.Focus()
		return m, textinput.Blink
	case "enter":
		switch m.cursor {
		case 0:
			m.result.ClientType = whatsrook.ClientChrome
		case 1:
			m.result.ClientType = whatsrook.ClientAndroid
		case 2:
			m.result.ClientType = whatsrook.ClientIos
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
		m.state = stateNewSaveOption
		m.cursor = 0
	case "2":
		m.result.Verbose = true
		m.state = stateNewSaveOption
		m.cursor = 0
	case "esc", "b", "0":
		m.state = stateNewClient
		m.cursor = 0
	case "enter":
		m.result.Verbose = m.cursor == 1
		m.state = stateNewSaveOption
		m.cursor = 0
	}
	return m, nil
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
		m.state = stateNewLogLevel
		m.cursor = 0
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
			m.state = stateNewLogLevel
			m.cursor = 0
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
	case stateNewClient:
		s.WriteString(m.viewNewClient())
	case stateNewLogLevel:
		s.WriteString(m.viewNewLogLevel())
	case stateNewSaveOption:
		s.WriteString(m.viewNewSaveOption())
	case stateUpdateMenu:
		s.WriteString(m.viewUpdateMenu())
	case stateUpdateProgress:
		s.WriteString(m.viewUpdateProgress())
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
		"Restart WhatsRook",
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
			s.WriteString(successStyle.Width(contentWidth).Render(fmt.Sprintf("✓ Upgrade successful! Version: v%s", m.updateResult.LatestVersion)))
			s.WriteString("\n\n")
			s.WriteString(activeItemStyle.Width(contentWidth).Render("  Press [Enter] to restart WhatsRook with new version now."))
			s.WriteString("\n\n")
			s.WriteString(helpStyle.Render("Press [Enter] to restart • [Esc] to exit"))
		} else if m.updateResult != nil {
			s.WriteString(successStyle.Width(contentWidth).Render(fmt.Sprintf("✓ Already at the latest version (%s).", m.updateResult.CurrentVersion)))
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
			s.WriteString("  " + logLineStyle.Render(logLine))
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
		"Delete session",
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
		dbName = "default (SQLite)"
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
	s.WriteString(inputPromptStyle.Render("Database URI (or 'default' for SQLite):"))
	s.WriteByte('\n')
	s.WriteString("  " + m.input.View())
	s.WriteString("\n\n")
	s.WriteString(helpStyle.Render(m.getHelpText("apply")))
	return s.String()
}

func (m model) viewDeleteConfirm() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("CONFIRM SESSION DELETION"))
	s.WriteString("\n\n")
	s.WriteString(errorStyle.Render(
		fmt.Sprintf("Are you sure you want to delete +%s?", m.selectedSession.User),
	))
	s.WriteString("\n\n")
	s.WriteString(renderDotItem(m.cursor == 0, "Cancel and return"))
	s.WriteString(renderDotItem(m.cursor == 1, "Confirm deletion"))
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
		s.WriteString(inputPromptStyle.Render("Phone Number (e.g. +2348062795602):"))
	} else {
		s.WriteString(titleStyle.Render("ENTER SESSION IDENTIFIER"))
		s.WriteString("\n\n")
		s.WriteString(inputPromptStyle.Render("Session Name [leave blank for auto]:"))
	}
	s.WriteByte('\n')
	s.WriteString("  " + m.input.View())
	s.WriteString("\n\n")
	s.WriteString(helpStyle.Render(m.getHelpText("continue")))
	return s.String()
}

func (m model) viewNewClient() string {
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

func (m model) viewNewSaveOption() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SAVE CONFIGURATION & LAUNCH"))
	s.WriteString("\n\n")

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

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
