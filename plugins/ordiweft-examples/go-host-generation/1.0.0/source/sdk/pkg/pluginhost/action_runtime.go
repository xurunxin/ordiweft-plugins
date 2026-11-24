package pluginhost

// ValidIsolatedActionDescriptor is the supported offline protocol profile,
// not an artifact/platform check, installation proof or permission grant.
func ValidIsolatedActionDescriptor(d Descriptor) bool {
	return (d.Runtime == "node" || d.Runtime == "process") && d.Generation == nil && len(d.Contributions) == 1 && d.Contributions[0].Kind == "processor" && d.Definitions[d.Contributions[0].ID] != "" && ValidActionRequirements(d.Requirements)
}
