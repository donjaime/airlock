package global

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// PromptSelect prints a numbered list and returns the 0-based index chosen by the user.
func PromptSelect(prompt string, choices []string) (int, error) {
	return promptSelect(os.Stdin, os.Stderr, prompt, choices, -1)
}

// PromptSelectDefault is like PromptSelect but pressing Enter selects defaultIdx.
func PromptSelectDefault(prompt string, choices []string, defaultIdx int) (int, error) {
	return promptSelect(os.Stdin, os.Stderr, prompt, choices, defaultIdx)
}

// promptSelect is the internal implementation. defaultIdx < 0 means no default.
func promptSelect(in io.Reader, out io.Writer, prompt string, choices []string, defaultIdx int) (int, error) {
	for i, c := range choices {
		if i == defaultIdx {
			fmt.Fprintf(out, "  %d. %s  ←\n", i+1, c)
		} else {
			fmt.Fprintf(out, "  %d. %s\n", i+1, c)
		}
	}
	r := bufio.NewReader(in)
	for {
		if defaultIdx >= 0 {
			fmt.Fprintf(out, "%s [1-%d, Enter=%d]: ", prompt, len(choices), defaultIdx+1)
		} else {
			fmt.Fprintf(out, "%s [1-%d]: ", prompt, len(choices))
		}
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" && defaultIdx >= 0 {
			return defaultIdx, nil
		}
		n, err := strconv.Atoi(trimmed)
		if err == nil && n >= 1 && n <= len(choices) {
			return n - 1, nil
		}
		fmt.Fprintf(out, "Please enter a number between 1 and %d.\n", len(choices))
	}
}

// PromptText prompts for a string value; returns defaultVal if the user presses enter.
func PromptText(prompt, defaultVal string) (string, error) {
	return promptText(os.Stdin, os.Stderr, prompt, defaultVal)
}

func promptText(in io.Reader, out io.Writer, prompt, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Fprintf(out, "%s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Fprintf(out, "%s: ", prompt)
	}
	r := bufio.NewReader(in)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(line)
	if v == "" {
		return defaultVal, nil
	}
	return v, nil
}

// PromptConfirm asks a y/N question. Returns true only on explicit "y" or "yes".
func PromptConfirm(prompt string) (bool, error) {
	return promptConfirm(os.Stdin, os.Stderr, prompt)
}

func promptConfirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	fmt.Fprintf(out, "%s [y/N]: ", prompt)
	r := bufio.NewReader(in)
	line, err := r.ReadString('\n')
	if err != nil {
		return false, err
	}
	v := strings.ToLower(strings.TrimSpace(line))
	return v == "y" || v == "yes", nil
}
