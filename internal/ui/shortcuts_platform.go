package ui

import "strings"

func (c platformChrome) shortcut(spec string) string {
	if c.trafficLights {
		return spec
	}
	// Cmd+Ctrl becomes Ctrl+Shift, rather than collapsing into plain Ctrl.
	// Otherwise resize/equalize bindings collide with other app commands.
	spec = strings.ReplaceAll(spec, "mod+ctrl+", "ctrl+shift+")
	return strings.ReplaceAll(spec, "mod+", "ctrl+")
}

func (c platformChrome) shortcutHint(spec, macHint string) string {
	if c.trafficLights {
		return macHint
	}
	parts := strings.Split(spec, "+")
	for i, part := range parts {
		switch part {
		case "ctrl":
			parts[i] = "Ctrl"
		case "shift":
			parts[i] = "Shift"
		case "alt":
			parts[i] = "Alt"
		case "enter":
			parts[i] = "Enter"
		default:
			parts[i] = strings.ToUpper(part)
		}
	}
	return strings.Join(parts, "+")
}

func (a *app) commandHint(id string) string {
	for _, command := range a.commands() {
		if command.ID == id {
			return command.Hint
		}
	}
	return ""
}
