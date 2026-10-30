package state

import (
	"context"
	"diskcleaner/internal/explain"
	"strings"
)

const explanationDisclosure = "Sends path, name, type, size, modification time and a limited list of child names to Codex. Codex uses read-only command permissions. It may read file contents and send them to the AI service, including files outside this item."

type explanationState struct {
	id        uint64
	path      string
	text      string
	offset    int
	message   string
	cancelled bool
	saveError string
}

func (m *Model) openExplanation() {
	entry, ok := m.current()
	if !ok {
		m.Message = "Highlight an item to request an AI explanation."
		return
	}
	m.explanation = explanationState{id: m.explanation.id + 1, path: entry.Path, offset: m.Offset, message: m.Message}
	m.Screen, m.Offset, m.Message = "ai-confirm", 0, ""
}

func (m *Model) leaveExplanation(message string) {
	m.Screen, m.Offset, m.Message = "list", m.explanation.offset, message
}

func (m *Model) explanationLanguage() string {
	if m.Settings.ExplainLanguage == "" {
		return "en"
	}
	return m.Settings.ExplainLanguage
}

func (m *Model) explanationLanguageLabel() string {
	if m.explanationLanguage() == "ru" {
		return "Русский"
	}
	return "English"
}

func (m *Model) explanationKey(key string) Job {
	if key == "esc" {
		message := m.explanation.message
		if m.explanation.saveError != "" {
			message = m.explanation.saveError
		}
		m.leaveExplanation(message)
		return nil
	}
	if m.Screen == "ai-confirm" && (key == "left" || key == "right" || key == "tab") {
		if m.explanationLanguage() == "en" {
			m.Settings.ExplainLanguage = "ru"
		} else {
			m.Settings.ExplainLanguage = "en"
		}
		m.save()
		m.explanation.saveError = ""
		if strings.HasPrefix(m.Message, "Settings not saved:") {
			m.explanation.saveError = m.Message
		}
		return nil
	}
	if key == "enter" && m.Screen == "ai-confirm" {
		if m.Explain == nil {
			m.leaveExplanation("AI explanation is unavailable.")
			return nil
		}
		request, run, id := explain.Request{Path: m.explanation.path, Language: m.explanationLanguage()}, m.Explain, m.explanation.id
		return m.start("ai-working", func(ctx context.Context) Event {
			text, err := run(ctx, request)
			return Event{Kind: "explain", ExplainID: id, Explanation: text, Err: err}
		})
	}
	switch key {
	case "home":
		m.Offset = 0
	case "end":
		_, m.Offset = m.render()
	default:
		m.scroll(key)
	}
	return nil
}

func (m *Model) applyExplanation(event Event) {
	if m.explanation.cancelled {
		m.leaveExplanation("AI explanation cancelled.")
	} else if event.Err != nil {
		m.leaveExplanation("AI explanation failed: " + event.Err.Error())
	} else {
		m.explanation.text = event.Explanation
		m.Screen, m.Offset = "ai-result", 0
	}
}
