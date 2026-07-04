package voicetranscription

import "strings"

// RetryableError marks a provider failure that can be retried automatically.
type RetryableError struct {
	Message         string
	ExecuteID       string
	LogID           string
	RawResponseJSON string
}

func (err RetryableError) Error() string {
	return strings.TrimSpace(err.Message)
}

// TerminalError marks a failure that should not be retried automatically.
type TerminalError struct {
	Message         string
	ExecuteID       string
	LogID           string
	RawResponseJSON string
}

func (err TerminalError) Error() string {
	return strings.TrimSpace(err.Message)
}

func errorExecuteID(cause error) string {
	switch typed := cause.(type) {
	case RetryableError:
		return strings.TrimSpace(typed.ExecuteID)
	case *RetryableError:
		return strings.TrimSpace(typed.ExecuteID)
	case TerminalError:
		return strings.TrimSpace(typed.ExecuteID)
	case *TerminalError:
		return strings.TrimSpace(typed.ExecuteID)
	default:
		return ""
	}
}

func errorLogID(cause error) string {
	switch typed := cause.(type) {
	case RetryableError:
		return strings.TrimSpace(typed.LogID)
	case *RetryableError:
		return strings.TrimSpace(typed.LogID)
	case TerminalError:
		return strings.TrimSpace(typed.LogID)
	case *TerminalError:
		return strings.TrimSpace(typed.LogID)
	default:
		return ""
	}
}

func errorRawResponse(cause error) string {
	switch typed := cause.(type) {
	case RetryableError:
		return typed.RawResponseJSON
	case *RetryableError:
		return typed.RawResponseJSON
	case TerminalError:
		return typed.RawResponseJSON
	case *TerminalError:
		return typed.RawResponseJSON
	default:
		return ""
	}
}
