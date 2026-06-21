package service

// BoostConfidence increases a lesson's confidence score by a reinforcement delta.
// Each reinforcement adds +0.1, capped at 1.0.
func BoostConfidence(currentConfidence float64) float64 {
	newConf := currentConfidence + 0.1
	if newConf > 1.0 {
		return 1.0
	}
	return newConf
}

// DecayConfidence decreases a lesson's confidence score due to non-use.
// Each decay removes -0.05, with a floor of 0.0.
func DecayConfidence(currentConfidence float64) float64 {
	newConf := currentConfidence - 0.05
	if newConf < 0.0 {
		return 0.0
	}
	return newConf
}

// ConfidenceDelta returns the change in confidence from a reinforcement event.
// This is the value stored in lesson_reinforcements.confidence_delta.
func ConfidenceDelta(before, after float64) float64 {
	return after - before
}
