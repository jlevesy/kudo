package generics

// Ptr returns the pointer to a value.
func Ptr[T comparable](v T) *T { return &v }
