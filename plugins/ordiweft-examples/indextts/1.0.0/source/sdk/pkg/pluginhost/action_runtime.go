package pluginhost

// ValidIsolatedActionDescriptor is the supported offline protocol profile,
// not an artifact/platform check, installation proof or permission grant.
func ValidIsolatedActionDescriptor(d Descriptor) bool {
	if (d.Runtime != "node" && d.Runtime != "process") || d.Generation != nil || len(d.Contributions) < 1 || len(d.Contributions) > 33 || d.Contributions[0].Kind != "processor" || d.Definitions[d.Contributions[0].ID] == "" || !ValidActionRequirements(d.Requirements) {
		return false
	}
	for _, contribution := range d.Contributions[1:] {
		if contribution.Kind != "node" || d.Definitions[contribution.ID] == "" {
			return false
		}
	}
	return true
}
