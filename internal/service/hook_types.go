package service

// Hook event type constants — the 13 valid observation types captured
// across the agent session lifecycle.
const (
	HookSessionStart       = "session_start"
	HookUserPromptSubmit   = "user_prompt_submit"
	HookPreToolUse         = "pre_tool_use"
	HookPostToolUse        = "post_tool_use"
	HookPostToolUseFailure = "post_tool_use_failure"
	HookPreCompact         = "pre_compact"
	HookSubagentStart      = "subagent_start"
	HookSubagentStop       = "subagent_stop"
	HookNotification       = "notification"
	HookTaskCompleted      = "task_completed"
	HookPostCommit         = "post_commit"
	HookSessionEnd         = "session_end"
	HookPermissionPrompt   = "permission_prompt"
)

// ValidHookTypes is the list of all 13 recognized hook event types.
var ValidHookTypes = []string{
	HookSessionStart,
	HookUserPromptSubmit,
	HookPreToolUse,
	HookPostToolUse,
	HookPostToolUseFailure,
	HookPreCompact,
	HookSubagentStart,
	HookSubagentStop,
	HookNotification,
	HookTaskCompleted,
	HookPostCommit,
	HookSessionEnd,
	HookPermissionPrompt,
}

// ValidateHookType returns true if the given type is one of the 13 valid hook types.
func ValidateHookType(hookType string) bool {
	for _, valid := range ValidHookTypes {
		if hookType == valid {
			return true
		}
	}
	return false
}

// ValidateImportance returns true if importance is within the valid range [0.0, 1.0].
func ValidateImportance(importance float64) bool {
	return importance >= 0.0 && importance <= 1.0
}
