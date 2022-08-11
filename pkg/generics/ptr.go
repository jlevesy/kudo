package generics

// Ptr returns the pointer to a value.
func Ptr[T any](v T) *T { return &v }
