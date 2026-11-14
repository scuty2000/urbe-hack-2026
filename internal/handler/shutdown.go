package handler

// Close should release any resource opened during the lifecycle of the _router
// (e.g., background goroutines). Currently a no-op.
func (rt *_router) Close() error {
	return nil
}
