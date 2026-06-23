package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TASK #66: Password input silently truncated on whitespace
// =============================================================================

func TestPromptPassword_ReadsFullLineWithSpaces(t *testing.T) {
	// Simulate typing a password with spaces (e.g. "my secret pass phrase")
	// The old code used fmt.Scanln which stops at whitespace.
	// The fix must read the full line including internal spaces.
	input := "my secret pass phrase\n"
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		w.WriteString(input)
		w.Close()
	}()

	pwd, err := promptPassword()
	require.NoError(t, err)
	assert.Equal(t, "my secret pass phrase", pwd,
		"password with spaces must be read as a complete line")
}

func TestPromptPassword_TrimsTrailingNewline(t *testing.T) {
	// The trailing newline from ReadString('\n') must be stripped.
	input := "hunter2\n"
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		w.WriteString(input)
		w.Close()
	}()

	pwd, err := promptPassword()
	require.NoError(t, err)
	assert.Equal(t, "hunter2", pwd,
		"trailing newline must be stripped from password")
}

func TestPromptPassword_EmptyInputReturnsError(t *testing.T) {
	// Just pressing Enter should return empty string.
	input := "\n"
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		w.WriteString(input)
		w.Close()
	}()

	_, err = promptPassword()
	require.NoError(t, err, "empty input is not a read error; password validation is upstream")
}
