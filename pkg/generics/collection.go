package generics

// Contains returns true if a specific value is contained in the given collection.
func Contains[T comparable](collection []T, value T) bool {
	for _, v := range collection {
		if v == value {
			return true
		}
	}

	return false
}
