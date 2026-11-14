package dto

// ErrorResponse is the standard JSON error payload returned by API endpoints.
type ErrorResponse struct {
	Error string `json:"error"`
}
