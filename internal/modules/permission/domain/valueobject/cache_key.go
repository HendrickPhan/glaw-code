package valueobject

// CacheKey uniquely identifies a permission request.
type CacheKey struct {
	ToolName string
	Input    string
}
