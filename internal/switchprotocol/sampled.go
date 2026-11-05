package switchprotocol

// NewSampledModel returns the separate restart clock experiment.
func NewSampledModel(rules Rules, state State, clock ClockReading, utcError uint64) (*UncertainModel, error) {
	return NewUncertainModel(rules, state, clock, utcError)
}
