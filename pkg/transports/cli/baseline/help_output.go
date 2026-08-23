package baseline

// NormalizeHelpOutput canonicalizes help text so fixtures stay stable across
// platforms and repeated runs.
func NormalizeHelpOutput(output string) string {
	return NormalizeFixtureText(output)
}
