package fsm

import "zoomClient/clients"

// State 表示Agent的状态
type State struct {
	Messages         []clients.Message `json:"messages"`
	TurnCount        int               `json:"turn_count"`
	TransitionReason *string           `json:"transition_reason,omitempty"`
}
