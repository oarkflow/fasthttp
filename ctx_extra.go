package fasthttp

// StatusCode returns the current response status code.
// Used by middleware to inspect the status after calling Next().
func (c *Ctx) StatusCode() int {
	return c.status
}
