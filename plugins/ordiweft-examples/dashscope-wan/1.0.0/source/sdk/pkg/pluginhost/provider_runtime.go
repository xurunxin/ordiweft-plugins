package pluginhost

// ValidIsolatedProcessProviderDescriptor describes the original-task protocol
// profile. Artifact/platform checks and host authority remain separate.
func ValidIsolatedProcessProviderDescriptor(d Descriptor) bool {
	return d.Runtime == "process" && d.Generation != nil && len(d.Requirements) > 0 && d.Generation.VoiceCatalog == nil && d.Generation.Scheduling.Independent && len(d.Contributions) == 1 && d.Contributions[0].Kind == "provider"
}
