package types

type Event struct {
	Agent                string
	Event                string
	SessionID            string
	TurnID               string
	CWD                  string
	Prompt               string
	LastAssistantMessage string
	StopHookActive       bool
	PermissionMode       string
	Tool                 *ToolEvent
	Files                []FileChange
	Raw                  map[string]any
}

type ToolEvent struct {
	Name    string
	Command string
	Input   any
	Output  any
}

type FileChange struct {
	Path        string
	BeforeLines int
	AfterLines  int
	Added       int
	Removed     int
	Status      string
}

type Decision struct {
	Mode     string `json:"mode"`
	RuleID   string `json:"rule_id,omitempty"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
}

const (
	ModeAllow    = "allow"
	ModeNudge    = "nudge"
	ModeBlock    = "block"
	ModeContinue = "continue"
)
