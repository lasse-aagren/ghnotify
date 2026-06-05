// Package dialog provides native OS input dialogs. macOS uses osascript;
// stubs for other platforms will be added when Linux/Windows support lands.
package dialog

import (
	"fmt"
	"os/exec"
	"strings"
)

// Input shows a text-input dialog and returns the entered text.
// Returns ("", false) if the user cancels.
func Input(prompt, defaultText string) (string, bool) {
	script := fmt.Sprintf(`display dialog %q default answer %q`, prompt, defaultText)
	return runDialog(script)
}

// Secret shows a password-style dialog with hidden input.
// Returns ("", false) if the user cancels.
func Secret(prompt string) (string, bool) {
	script := fmt.Sprintf(`display dialog %q default answer "" with hidden answer`, prompt)
	return runDialog(script)
}

// Choose shows a dialog with labelled buttons and returns the chosen button label.
// Returns ("", false) if the user cancels.
func Choose(prompt string, defaultButton string, buttons ...string) (string, bool) {
	quoted := make([]string, len(buttons))
	for i, b := range buttons {
		quoted[i] = fmt.Sprintf("%q", b)
	}
	script := fmt.Sprintf(`display dialog %q buttons {%s} default button %q`,
		prompt, strings.Join(quoted, ", "), defaultButton)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", false // cancelled
	}
	text := strings.TrimSpace(string(out))
	const marker = "button returned:"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", false
	}
	return strings.TrimSpace(text[idx+len(marker):]), true
}

// Alert shows an informational alert (fire-and-forget).
func Alert(title, message string) {
	script := fmt.Sprintf(`display alert %q message %q`, title, message)
	_ = exec.Command("osascript", "-e", script).Run()
}

func runDialog(script string) (string, bool) {
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", false // cancelled or error
	}
	text := strings.TrimSpace(string(out))
	const marker = "text returned:"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", false
	}
	// Value runs to end of output (after "text returned:").
	// Trim any trailing ", button returned:OK" if present.
	val := text[idx+len(marker):]
	if i := strings.Index(val, ", button returned:"); i >= 0 {
		val = val[:i]
	}
	return strings.TrimSpace(val), true
}
