package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"whatsrook"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	quitting        bool
}

// Run launches the modern Bubble Tea standby TUI with dot-based navigation and .env integration.
func Run(ctx context.Context, defaultDB string, boundPort int) (SessionResult, bool, error) {
	ti := textinput.New()
	ti.Placeholder = "+2348062795602"
	ti.CharLimit = 32
	ti.Width = 36

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
	if err != nil {
		return SessionResult{}, false, err
	}

	fm := finalModel.(model)
	return fm.result, fm.result.ShouldRun, nil
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.currentTime = time.Time(msg)
		return m, tickCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
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
		if m.cursor < 2 {
			m.cursor++
		}
	case "1":
		m.cursor = 0
		return m.selectMainOption()
	case "2":
		m.cursor = 1
		return m.selectMainOption()
	case "3", "q", "esc":
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
			m.statusMsg = "No saved sessions found in database. Please create a new session."
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
	case 2: // Exit
		m.quitting = true
		return m, tea.Quit
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
			m.statusMsg = "Saved to .env. To see this menu again in the future, run with -i."
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
		m.statusMsg = "Saved to .env. To see this menu again in the future, run with -i."
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
	case "y", "Y", "enter":
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
	case "n", "N", "esc", "b", "0":
		m.state = stateSessionActions
		m.cursor = 3
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
		m.input.Placeholder = "session name or phone (optional)"
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
			m.input.Placeholder = "session name or phone (optional)"
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
				m.statusMsg = "Invalid phone number. Must include country code without spaces (e.g. +2348062795602)."
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

	// Top Dynamic Status Header
	portStr := strconv.Itoa(m.boundPort)
	timeStr := m.currentTime.Format("15:04:05")
	headerText := fmt.Sprintf(
		"%s %s %s %s %s",
		headerTitleStyle.Render("WHATSROOK STANDBY"),
		headerMutedStyle.Render("• PORT :"+portStr),
		headerMutedStyle.Render("• WAITING FOR SESSION"),
		headerMutedStyle.Render("•"),
		headerTimeStyle.Render(timeStr),
	)
	s.WriteString(headerBox.Render(headerText))
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

func (m model) viewMain() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("MAIN MENU"))
	s.WriteString("\n\n")

	options := []string{
		"Connect to an existing session",
		"Create a new session",
		"Exit",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to select • Esc/q to exit"))
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

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to select • Esc to go back"))
	return s.String()
}

func (m model) viewSessionActions() string {
	var s strings.Builder
	name := m.selectedSession.PushName
	if name == "" {
		name = "Personal"
	}
	s.WriteString(titleStyle.Render(fmt.Sprintf("SESSION: +%s (%s • %s)", m.selectedSession.User, name, m.selectedSession.Platform)))
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

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to select • Esc to go back"))
	return s.String()
}

func (m model) viewEditVariables() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render(fmt.Sprintf("SESSION CONFIGURATION (+%s)", m.selectedSession.User)))
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

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to modify • Esc to go back"))
	return s.String()
}

func (m model) viewEditClient() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SELECT CLIENT PLATFORM PROFILE"))
	s.WriteString("\n\n")

	options := []string{
		"Default (Desktop / Chrome)",
		"Android Phone",
		"iPhone (iOS)",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to apply • Esc to go back"))
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

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to apply • Esc to go back"))
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
	s.WriteString(helpStyle.Render("Enter to apply • Esc to cancel"))
	return s.String()
}

func (m model) viewDeleteConfirm() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("CONFIRM SESSION DELETION"))
	s.WriteString("\n\n")
	s.WriteString(errorStyle.Render(
		fmt.Sprintf("Are you sure you want to delete session +%s?", m.selectedSession.User),
	))
	s.WriteString("\n\n")
	s.WriteString(renderDotItem(m.cursor == 0, "Cancel and return"))
	s.WriteString(renderDotItem(m.cursor == 1, "Confirm deletion"))
	s.WriteString("\n")
	s.WriteString(helpStyle.Render("Use ↑/↓ to choose • Enter to confirm • Esc to cancel"))
	return s.String()
}

func (m model) viewNewAuth() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("CREATE NEW SESSION • AUTHENTICATION METHOD"))
	s.WriteString("\n\n")

	options := []string{
		"QR Code (scan with WhatsApp Linked Devices)",
		"Pairing Code (enter phone number & receive 8-digit code)",
		"Back to main menu",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to select • Esc to go back"))
	return s.String()
}

func (m model) viewNewPhoneInput() string {
	var s strings.Builder
	if m.result.Pair {
		s.WriteString(titleStyle.Render("ENTER PHONE NUMBER FOR PAIRING CODE"))
		s.WriteString("\n\n")
		s.WriteString(inputPromptStyle.Render("Phone Number (with country code, e.g. +2348062795602):"))
	} else {
		s.WriteString(titleStyle.Render("ENTER SESSION NAME OR PHONE (QR SCAN)"))
		s.WriteString("\n\n")
		s.WriteString(inputPromptStyle.Render("Session Identifier [optional, leave blank for auto]:"))
	}
	s.WriteByte('\n')
	s.WriteString("  " + m.input.View())
	s.WriteString("\n\n")
	s.WriteString(helpStyle.Render("Enter to continue • Esc to go back"))
	return s.String()
}

func (m model) viewNewClient() string {
	var s strings.Builder
	s.WriteString(titleStyle.Render("SELECT CLIENT PLATFORM PROFILE"))
	s.WriteString("\n\n")

	options := []string{
		"Default (Desktop / Chrome)",
		"Android Phone",
		"iPhone (iOS)",
	}

	for i, opt := range options {
		s.WriteString(renderDotItem(m.cursor == i, opt))
	}

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to select • Esc to go back"))
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

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to select • Esc to go back"))
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

	s.WriteString(helpStyle.Render("Use ↑/↓ to navigate • Enter to select • Esc to go back"))
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
