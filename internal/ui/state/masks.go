package state

var suggestedMasks = []struct{ name, description string }{
	{"node_modules", "Installed Node.js dependencies."},
	{"CloudStorage", "Cloud-provider storage, including Library/CloudStorage."},
	{".git", "Git history and repository metadata."},
	{".venv", "Python virtual environments named .venv."},
	{"venv", "Python virtual environments named venv."},
	{"__pycache__", "Python bytecode cache folders."},
	{".cache", "Hidden application and development-tool caches."},
	{"Caches", "Application caches, including Library/Caches."},
	{".gradle", "Gradle caches and project working files."},
	{".next", "Next.js generated build and cache files."},
	{".build", "Swift Package Manager build files."},
	{"DerivedData", "Xcode build intermediates and indexes."},
	{"target", "Build output, commonly used by Rust and Maven."},
	{"vendor", "Bundled third-party dependencies."},
	{"dist", "Generated distribution output."},
	{"build", "Generated build folders; this is a broad name."},
}

func maskDescription(mask string) string {
	for _, suggestion := range suggestedMasks {
		if suggestion.name == mask {
			return suggestion.description
		}
	}
	return "Custom pattern. Matches names at every depth in this root."
}

func (m *Model) openMasks() {
	m.Screen, m.Input, m.MaskInput = "masks", "", false
	m.Offset, m.maskCursor = 0, 0
	m.maskOptions = nil
	for _, suggestion := range suggestedMasks {
		m.addMaskOption(suggestion.name)
	}
	for _, mask := range m.Masks() {
		m.addMaskOption(mask)
	}
}

func (m *Model) addMaskOption(mask string) {
	for _, option := range m.maskOptions {
		if option == mask {
			return
		}
	}
	m.maskOptions = append(m.maskOptions, mask)
}

func (m *Model) maskKey(key string) {
	if key == "tab" {
		m.MaskInput = !m.MaskInput
		return
	}
	if m.MaskInput {
		switch key {
		case "enter":
			if !m.hasMask(m.Input) {
				m.toggleMask(m.Input)
			}
			if m.hasMask(m.Input) {
				m.addMaskOption(m.Input)
				for i, mask := range m.maskOptions {
					if mask == m.Input {
						m.maskCursor = i
						break
					}
				}
				m.Input, m.MaskInput = "", false
			}
		case "backspace":
			m.Input = backspace(m.Input)
		default:
			m.Input += typedCharacter(key)
		}
		return
	}
	switch key {
	case "up", "k":
		m.maskCursor = max(0, m.maskCursor-1)
	case "down", "j":
		m.maskCursor = min(len(m.maskOptions)-1, m.maskCursor+1)
	case "pgup":
		m.maskCursor = max(0, m.maskCursor-10)
	case "pgdown":
		m.maskCursor = min(len(m.maskOptions)-1, m.maskCursor+10)
	case "enter", "space", " ":
		if m.maskCursor >= 0 && m.maskCursor < len(m.maskOptions) {
			m.toggleMask(m.maskOptions[m.maskCursor])
		}
	case "a":
		m.MaskInput = true
	}
}
