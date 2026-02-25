package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type sessionPickerWrapper struct {
	cfg       *config.Config
	store     *session.Store
	picker    *ui.Picker
	renaming  bool
	textInput textinput.Model
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
				selected, _ := w.picker.Selected()
				newName := strings.TrimSpace(w.textInput.Value())
				if newName != "" && selected != nil {
					_ = w.store.Rename(selected.ID, newName)
					summaries, _ := w.store.List()
					w.picker.RefreshItems(w.mapSessionsToItems(summaries))
				}
				w.renaming = false
				w.textInput.Blur()
				return w, nil
			case "esc":
				w.renaming = false
				w.textInput.Blur()
				return w, nil
			}
		}
		var cmd tea.Cmd
		w.textInput, cmd = w.textInput.Update(msg)
		return w, cmd
	}

	res, cmd := w.picker.Update(msg)
	w.picker = res.(*ui.Picker)
	return w, cmd
}

func (w *sessionPickerWrapper) View() string {
	if w.renaming {
		return fmt.Sprintf("\n  Rename session:\n\n  %s\n\n  (Enter to save, Esc to cancel)\n", w.textInput.View())
	}
	return w.picker.View()
}

func (w *sessionPickerWrapper) mapSessionsToItems(sessions []domain.SessionSummary) []ui.Item {
	var items []ui.Item
	for _, s := range sessions {
		name := s.Name
		if name == "" {
			name = "(untitled)"
		}

		items = append(items, ui.Item{
			ID:     s.ID,
			Label:  name,
			Detail: fmt.Sprintf("%d msgs  %s", s.MessageCount, s.Updated.Format("2.Jan 15:04")),
			Active: s.ID == w.cfg.Session.CurrentSessionID,
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store, err := buildSessionStore(cfg)
	if err != nil {
		return err
	}

	summaries, err := store.List()
	if err != nil {
		return err
	}

	ti := textinput.New()
	ti.Placeholder = "New session name..."
	ti.Focus()

	wrapper := &sessionPickerWrapper{
		cfg:       cfg,
		store:     store,
		textInput: ti,
	}

	pickerCfg := ui.Config{
		Title: "SESSIONS",
		Items: wrapper.mapSessionsToItems(summaries),
		Actions: []ui.Action{
			{
				Key:   "n",
				Label: "new",
				Fn: func(item ui.Item) tea.Cmd {
					sess, err := store.Create()
					if err != nil {
						return nil
					}
					cfg.Session.CurrentSessionID = sess.ID
					_ = config.Save(cfg)
					return tea.Quit
				},
			},
			{
				Key:   "r",
				Label: "rename",
				Fn: func(item ui.Item) tea.Cmd {
					wrapper.renaming = true
					wrapper.textInput.SetValue(item.Label)
					wrapper.textInput.Focus()
					return textinput.Blink
				},
			},
			{
				Key:   "d",
				Label: "delete",
				Fn: func(item ui.Item) tea.Cmd {
					_ = store.Delete(item.ID)
					summaries, _ := store.List()
					wrapper.picker.RefreshItems(wrapper.mapSessionsToItems(summaries))
					return nil
				},
			},
		},
	}

	wrapper.picker = ui.NewPicker(pickerCfg)
	p := tea.NewProgram(wrapper)

	if _, err := p.Run(); err != nil {
		return err
	}

	if selected, ok := wrapper.picker.Selected(); ok {
		cfg.Session.CurrentSessionID = selected.ID
		_ = config.Save(cfg)
		fmt.Printf("\nSelected session: %s\n", selected.ID)
	}

	return nil
}
