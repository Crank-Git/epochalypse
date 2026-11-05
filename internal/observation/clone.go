package observation

// Clone separates result metadata from mutable caller configuration.
func (target Target) Clone() Target {
	target.Clock.OffsetNanoseconds = cloneOffset(target.Clock.OffsetNanoseconds)
	target.Clock.LowerNanoseconds = cloneOffset(target.Clock.LowerNanoseconds)
	target.Clock.UpperNanoseconds = cloneOffset(target.Clock.UpperNanoseconds)
	return target
}

func cloneOffset(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
