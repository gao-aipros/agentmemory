package service

import (
	"strings"
	"testing"
)

// TestBuildSyntheticCompression_InferType verifies type inference for each tool name
// category and hookType override.
func TestBuildSyntheticCompression_InferType(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		hookType string
		wantType string
	}{
		// Web fetch variants
		{"WebFetch camelCase", "WebFetch", "", "web_fetch"},
		{"fetch lowercase", "fetch", "", "web_fetch"},
		{"http tool", "http", "", "web_fetch"},
		{"web", "web", "", "web_fetch"},
		{"tool starts with fetch", "fetch_thing", "", "web_fetch"},
		{"tool ends with web", "get_web", "", "web_fetch"},

		// Search variants
		{"grep tool", "grep", "", "search"},
		{"search tool", "search", "", "search"},
		{"glob tool", "glob_something", "", "search"},
		{"find tool", "find", "", "search"},

		// Command run variants
		{"bash tool", "BashExec", "", "command_run"},
		{"shell tool", "shell_run", "", "command_run"},
		{"exec tool", "exec", "", "command_run"},
		{"run tool", "run", "", "command_run"},

		// File edit variants
		{"edit tool", "edit", "", "file_edit"},
		{"update tool", "update", "", "file_edit"},
		{"patch file", "patch", "", "file_edit"},
		{"replace tool", "replace_text", "", "file_edit"},

		// File write variants
		{"write tool", "write_file", "", "file_write"},
		{"create tool", "CreateResource", "", "file_write"},

		// File read variants
		{"read tool", "read", "", "file_read"},
		{"view tool", "view", "", "file_read"},
		{"ReadFile tool", "ReadFile", "", "file_read"},

		// Subagent variants
		{"task tool", "task", "", "subagent"},
		{"agent tool", "agent", "", "subagent"},
		{"SpawnAgent tool", "SpawnAgent", "", "subagent"},
		{"taskManager tool", "taskManager", "", "subagent"},

		// HookType overrides
		{"post_tool_failure hookType", "", "post_tool_failure", "error"},
		{"prompt_submit hookType", "", "prompt_submit", "conversation"},
		{"subagent_stop hookType", "", "subagent_stop", "subagent"},
		{"task_completed hookType", "", "task_completed", "subagent"},
		{"notification hookType", "", "notification", "notification"},

		// Unknown / default
		{"unknown tool", "unknown_tool", "", "other"},
		{"empty toolName and hookType", "", "", "other"},
		{"nil/empty", "", "", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSyntheticCompression("test title", "test narrative", tt.toolName, tt.hookType, "", nil)
			if tt.wantType == "other" {
				// "other" type does not add a prefix; verify no type bracket prefix exists
				if strings.Contains(got.CompressedText, "[") {
					t.Errorf("compressed_text should not have a type prefix for type %q, got: %q",
						tt.wantType, got.CompressedText)
				}
			} else {
				if !strings.Contains(got.CompressedText, tt.wantType) {
					t.Errorf("compressed_text should mention type %q for toolName=%q hookType=%q, got: %q",
						tt.wantType, tt.toolName, tt.hookType, got.CompressedText)
				}
			}
		})
	}
}

// TestBuildSyntheticCompression_ExtractFiles verifies file extraction from JSON input
// using various keys.
func TestBuildSyntheticCompression_ExtractFiles(t *testing.T) {
	tests := []struct {
		name       string
		toolInput  string
		wantFiles  []string
		wantInText bool // whether files should appear in compressed_text or concepts
	}{
		{
			name:       "file_path key",
			toolInput:  `{"file_path": "/tmp/test.txt"}`,
			wantFiles:  []string{"/tmp/test.txt"},
			wantInText: true,
		},
		{
			name:       "filepath key",
			toolInput:  `{"filepath": "/home/user/doc.md"}`,
			wantFiles:  []string{"/home/user/doc.md"},
			wantInText: true,
		},
		{
			name:       "path key",
			toolInput:  `{"path": "/usr/local/bin"}`,
			wantFiles:  []string{"/usr/local/bin"},
			wantInText: true,
		},
		{
			name:       "filePath key",
			toolInput:  `{"filePath": "config.yaml"}`,
			wantFiles:  []string{"config.yaml"},
			wantInText: true,
		},
		{
			name:       "file key",
			toolInput:  `{"file": "main.go"}`,
			wantFiles:  []string{"main.go"},
			wantInText: true,
		},
		{
			name:       "pattern key",
			toolInput:  `{"pattern": "*.go"}`,
			wantFiles:  []string{"*.go"},
			wantInText: true,
		},
		{
			name:       "multiple file keys",
			toolInput:  `{"file_path": "/a.txt", "filepath": "/b.txt", "file": "/c.txt"}`,
			wantFiles:  []string{"/a.txt", "/b.txt", "/c.txt"},
			wantInText: true,
		},
		{
			name:       "empty input",
			toolInput:  "",
			wantFiles:  nil,
			wantInText: false,
		},
		{
			name:       "invalid JSON",
			toolInput:  "not json",
			wantFiles:  nil,
			wantInText: false,
		},
		{
			name:       "file value too long is excluded",
			toolInput:  `{"file_path": "` + strings.Repeat("a", 512) + `"}`,
			wantFiles:  nil,
			wantInText: false,
		},
		{
			name:       "empty string file path excluded",
			toolInput:  `{"file_path": ""}`,
			wantFiles:  nil,
			wantInText: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildSyntheticCompression("test title", "test narrative", "read", "", tt.toolInput, nil)
			if tt.wantInText {
				for _, f := range tt.wantFiles {
					found := false
					for _, c := range result.Concepts {
						if c == f {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("concepts should contain file %q, got: %v", f, result.Concepts)
					}
				}
			}
			// Verify at least one concept includes a file path if files were extracted
			if len(tt.wantFiles) > 0 && len(result.Concepts) == 0 {
				t.Errorf("expected at least one concept for files, got none")
			}
			if len(tt.wantFiles) == 0 && len(result.Concepts) > 0 {
				t.Errorf("expected no concepts when no files, got: %v", result.Concepts)
			}
		})
	}
}

// TestBuildSyntheticCompression_ReturnsLowConfidence ensures synthetic results
// always have confidence=0.3.
func TestBuildSyntheticCompression_ReturnsLowConfidence(t *testing.T) {
	result := BuildSyntheticCompression("test", "narrative", "read", "", `{"file_path": "/tmp/test.go"}`, nil)
	if result.Confidence != 0.3 {
		t.Errorf("synthetic confidence should be 0.3, got %f", result.Confidence)
	}
}

// TestBuildSyntheticCompression_NonEmptyOutput ensures even with minimal input,
// compressed_text is never empty.
func TestBuildSyntheticCompression_NonEmptyOutput(t *testing.T) {
	result := BuildSyntheticCompression("", "", "", "", "", nil)
	if result.CompressedText == "" {
		t.Error("compressed_text should not be empty even with minimal input")
	}
	if result.Confidence != 0.3 {
		t.Errorf("synthetic confidence should be 0.3, got %f", result.Confidence)
	}
}

// TestBuildSyntheticCompression_Truncation ensures long narratives are truncated.
func TestBuildSyntheticCompression_Truncation(t *testing.T) {
	longString := strings.Repeat("x", 500)
	result := BuildSyntheticCompression("title", longString, "WebFetch", "", "", nil)
	if len(result.CompressedText) > 400 {
		t.Errorf("compressed_text should be truncated to 400 chars, got %d", len(result.CompressedText))
	}
}

// TestBuildSyntheticCompression_ReturnsCompressionResultConvertible verifies the result
// can be assigned to a CompressionResult (same shape).
func TestBuildSyntheticCompression_ReturnsCompressionResultConvertible(t *testing.T) {
	sr := BuildSyntheticCompression("title", "narrative", "read", "", `{"file_path": "f.go"}`, nil)
	// Verify it can be converted to CompressionResult
	cr := CompressionResult{
		CompressedText: sr.CompressedText,
		Concepts:       sr.Concepts,
	}
	if cr.CompressedText != sr.CompressedText {
		t.Error("CompressionResult conversion failed")
	}
}

// TestBuildSyntheticCompression_DirectFiles verifies that a non-nil files parameter
// is used directly (skipping extractFiles), while nil falls back to toolInputJSON.
func TestBuildSyntheticCompression_DirectFiles(t *testing.T) {
	tests := []struct {
		name      string
		files     []string
		toolInput string
		wantFiles []string
	}{
		{
			name:      "direct files appear in concepts",
			files:     []string{"/a.go", "/b.go"},
			toolInput: "",
			wantFiles: []string{"/a.go", "/b.go"},
		},
		{
			name:      "nil files falls back to toolInputJSON",
			files:     nil,
			toolInput: `{"file_path": "/c.go"}`,
			wantFiles: []string{"/c.go"},
		},
		{
			name:      "empty non-nil files skips extractFiles",
			files:     []string{},
			toolInput: `{"file_path": "/d.go"}`,
			wantFiles: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildSyntheticCompression("title", "narrative", "read", "", tt.toolInput, tt.files)
			if tt.wantFiles == nil {
				if len(result.Concepts) != 0 {
					t.Errorf("expected no concepts, got: %v", result.Concepts)
				}
				return
			}
			for _, want := range tt.wantFiles {
				found := false
				for _, c := range result.Concepts {
					if c == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("concepts should contain %q, got: %v", want, result.Concepts)
				}
			}
		})
	}
}
