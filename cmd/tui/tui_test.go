package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"whatsrook"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestWizard_FirstTimeConnection_InitialState(t *testing.T) {
	ti := textinput.New()
	ti.Placeholder = "+15551234567"

	// Simulate first-time connection where stored sessions is empty
	var emptySessions []whatsrook.StoredSession

	initialState := stateMain
	if len(emptySessions) == 0 {
		initialState = stateNewAuth
	}

	m := model{
		ctx:         context.Background(),
		state:       initialState,
		defaultDB:   "default",
		sessions:    emptySessions,
		currentTime: time.Now(),
		input:       ti,
		result: SessionResult{
			ClientType: whatsrook.ClientChrome,
			Database:   "default",
			Verbose:    false,
		},
	}

	if m.state != stateNewAuth {
		t.Fatalf("expected initial state on first time connection to be stateNewAuth, got %v", m.state)
	}

	// Verify viewNewAuth renders the setup wizard
	view := m.View()
	if !strings.Contains(view, "CREATE NEW SESSION") {
		t.Fatalf("expected view to contain 'CREATE NEW SESSION', got: %s", view)
	}
}

func TestWizard_DBStep_NavigationFlow(t *testing.T) {
	ti := textinput.New()
	ti.CharLimit = 32
	ti.Width = 32

	m := model{
		ctx:         context.Background(),
		state:       stateNewLogLevel,
		defaultDB:   "postgres://user:pass@localhost:5432/whatsrook?sslmode=disable",
		currentTime: time.Now(),
		input:       ti,
		result: SessionResult{
			Session:    "+15551234567",
			ClientType: whatsrook.ClientChrome,
			Database:   "postgres://user:pass@localhost:5432/whatsrook?sslmode=disable",
		},
	}

	// Step 1: Select log level (option 1: Standard) -> should transition to stateNewDB
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updatedModel.(model)

	if m.state != stateNewDB {
		t.Fatalf("expected state after log level to be stateNewDB, got %v", m.state)
	}

	// Verify input limits were expanded for DB URL
	if m.input.CharLimit < 256 {
		t.Fatalf("expected input CharLimit >= 256 for DB URL, got %d", m.input.CharLimit)
	}
	if m.input.Value() != "postgres://user:pass@localhost:5432/whatsrook?sslmode=disable" {
		t.Fatalf("expected input value to match pre-configured database, got %q", m.input.Value())
	}

	// Verify viewNewDB renders PostgreSQL prompt
	view := m.View()
	if !strings.Contains(view, "CONFIGURE DATABASE CONNECTION") {
		t.Fatalf("expected view to contain 'CONFIGURE DATABASE CONNECTION', got: %s", view)
	}
	if !strings.Contains(view, "PostgreSQL URL") {
		t.Fatalf("expected view to prompt for PostgreSQL URL, got: %s", view)
	}

	// Step 2: Press enter in stateNewDB to accept DB -> should transition to stateNewSaveOption
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedModel.(model)

	if m.state != stateNewSaveOption {
		t.Fatalf("expected state after DB step to be stateNewSaveOption, got %v", m.state)
	}

	// Verify viewNewSaveOption renders database in summary preview
	saveView := m.View()
	if !strings.Contains(saveView, "SAVE CONFIGURATION & LAUNCH") {
		t.Fatalf("expected view to contain 'SAVE CONFIGURATION & LAUNCH', got: %s", saveView)
	}
	if !strings.Contains(saveView, "Database: postgres://user:pass@localhost:5432/whatsrook?sslmode=disable") {
		t.Fatalf("expected save view to preview configured database URL, got: %s", saveView)
	}

	// Step 3: Press Back ('b' or Esc) from stateNewSaveOption -> should transition back to stateNewDB
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = updatedModel.(model)

	if m.state != stateNewDB {
		t.Fatalf("expected Back from stateNewSaveOption to return to stateNewDB, got %v", m.state)
	}

	// Step 4: Press Esc from stateNewDB -> should transition back to stateNewLogLevel
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updatedModel.(model)

	if m.state != stateNewLogLevel {
		t.Fatalf("expected Esc from stateNewDB to return to stateNewLogLevel, got %v", m.state)
	}
}

func TestWizard_CustomDB_InputUpdate(t *testing.T) {
	ti := textinput.New()
	ti.CharLimit = 256
	ti.Width = 50

	customURL := "postgresql://myuser:secret@custom-postgres.internal:5433/botdb?sslmode=require"
	ti.SetValue(customURL)

	m := model{
		ctx:         context.Background(),
		state:       stateNewDB,
		defaultDB:   "default",
		currentTime: time.Now(),
		input:       ti,
		result: SessionResult{
			Session:    "+15551234567",
			ClientType: whatsrook.ClientAndroid,
			Database:   "default",
		},
	}

	// Press Enter with custom URL
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedModel.(model)

	if m.state != stateNewSaveOption {
		t.Fatalf("expected state to advance to stateNewSaveOption, got %v", m.state)
	}
	if m.result.Database != customURL {
		t.Fatalf("expected result Database to be %q, got %q", customURL, m.result.Database)
	}
}

func TestSaveDotEnv_PreservesPostgreSQL(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() {
		_ = os.Chdir(origWd)
	}()

	customDB := "postgresql://remote-host:5433/whatsrook_test?sslmode=disable"
	err := SaveDotEnv("+2348012345678", "android", true, customDB)
	if err != nil {
		t.Fatalf("SaveDotEnv failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if err != nil {
		t.Fatalf("failed to read created .env: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "SESSION=+2348012345678") {
		t.Errorf("expected SESSION in .env, got: %s", content)
	}
	if !strings.Contains(content, "CLIENT=android") {
		t.Errorf("expected CLIENT in .env, got: %s", content)
	}
	if !strings.Contains(content, "VERBOSE=true") {
		t.Errorf("expected VERBOSE in .env, got: %s", content)
	}
	if !strings.Contains(content, "DATABASE_URL="+customDB) {
		t.Errorf("expected DATABASE_URL in .env, got: %s", content)
	}
}
