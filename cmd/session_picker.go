package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/Cyclone1070/iav/internal/ui/picker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List and manage chat sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionPicker()
	},
}

type sessionPickerWrapper struct {
	picker       *picker.Picker
	cfg          *config.Config
	state        *state.State
	store        *session.Store
	renaming     bool
	renameItemID string
	textInput          textinput.Model
	newSessionCreated  bool
}

func (w *sessionPickerWrapper) Init() tea.Cmd {
	return nil
}

func (w *sessionPickerWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if w.renaming {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				newName := strings.TrimSpace(w.textInput.Value())
				if newName != "" && w.renameItemID != "" {
					_ = w.store.Rename(w.renameItemID, newName)
					summaries, _ := w.store.List()
					w.picker.RefreshItems(w.mapSessionsToItems(summaries))
				}
				w.renaming = false
				w.renameItemID = ""
				w.textInput.Blur()
				return w, nil
			case "esc":
				w.renaming = false
				w.renameItemID = ""
				w.textInput.Blur()
				return w, nil
			}
		}
		var cmd tea.Cmd
		w.textInput, cmd = w.textInput.Update(msg)
		return w, cmd
	}

	res, cmd := w.picker.Update(msg)
	w.picker = res.(*picker.Picker)
	return w, cmd
}

func (w *sessionPickerWrapper) View() string {
	if w.renaming {
		return fmt.Sprintf("\n  Rename session:\n\n  %s\n\n  (Enter to save, Esc to cancel)\n", w.textInput.View())
	}
	return w.picker.View()
}

func (w *sessionPickerWrapper) mapSessionsToItems(sessions []domain.SessionSummary) []picker.Item {
	var items []picker.Item
	for _, s := range sessions {
		name := s.Name
		if name == "" {
			name = "(untitled)"
		}

		items = append(items, picker.Item{
			ID:     s.ID,
			Label:  name,
			Detail: fmt.Sprintf("%d msgs  %s", s.MessageCount, s.Updated.Format("2.Jan 15:04")),
			Active: s.ID == w.state.CurrentSessionID(),
			Group:  getDateGroup(s.Updated),
		})
	}
	return items
}

func getDateGroup(t time.Time) string {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	thisWeek := today.AddDate(0, 0, -7)

	if t.After(today) {
		return "Today"
	}
	if t.After(yesterday) {
		return "Yesterday"
	}
	if t.After(thisWeek) {
		return "This Week"
	}
	return "Earlier"
}

func runSessionPicker() error {
	bootstrapFS := fs.NewOSFileSystem(-1)

	configMgr := config.NewManager(bootstrapFS)
	cfg, err := configMgr.Load()
	if err != nil {
		return err
	}

	stateMgr := state.NewManager(bootstrapFS)
	appState, err := stateMgr.Load()
	if err != nil {
		return err
	}

	store, err := buildSessionStore(cfg, bootstrapFS)
	if err != nil {
		return err
	}

	summaries, err := store.List()
	if err != nil {
		return err
	}

	ti := textinput.New()
	ti.Placeholder = "New session name..."

	wrapper := &sessionPickerWrapper{
		cfg:       cfg,
		state:     appState,
		store:     store,
		textInput: ti,
	}

	pickerCfg := picker.Config{
		Title: "SESSIONS",
		Items: wrapper.mapSessionsToItems(summaries),
		Actions: []picker.Action{
			{
				Key:   "n",
				Label: "new",
				Quit:  true,
				Fn: func(item picker.Item) tea.Cmd {
					sess, err := store.Create()
					if err != nil {
						return nil
					}
					appState.SetCurrentSessionID(sess.ID)
					_ = appState.Save()
					wrapper.newSessionCreated = true
					return nil
				},
			},
			{
				Key:   "r",
				Label: "rename",
				Fn: func(item picker.Item) tea.Cmd {
					wrapper.renaming = true
					wrapper.renameItemID = item.ID
					wrapper.textInput.SetValue(item.Label)
					wrapper.textInput.Focus()
					return textinput.Blink
				},
			},
			{
				Key:   "d",
				Label: "delete",
				Fn: func(item picker.Item) tea.Cmd {
					_ = store.Delete(item.ID)
					if item.ID == appState.CurrentSessionID() {
						appState.SetCurrentSessionID("")
						_ = appState.Save()
					}
					summaries, _ := store.List()
					wrapper.picker.RefreshItems(wrapper.mapSessionsToItems(summaries))
					return nil
				},
			},
		},
	}

	wrapper.picker = picker.NewPicker(pickerCfg)
	p := tea.NewProgram(wrapper)

	if _, err := p.Run(); err != nil {
		return err
	}

	if wrapper.newSessionCreated {
		fmt.Printf("Started new session\n")
	} else if selected, ok := wrapper.picker.Selected(); ok {
		appState.SetCurrentSessionID(selected.ID)
		_ = appState.Save()
		fmt.Printf("Selected session\n")
	}

	return nil
}
