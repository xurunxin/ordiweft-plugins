package pluginhost

const BailianProfile = "dashscope-v1"
const BailianPackage = "ordiweft-migrations/dashscope"

// ValidBailianProfileDescriptor identifies the only data package currently
// permitted to select a trusted provider implementation. It cannot select a
// connection, request sandbox privileges, or include a second contribution.
func ValidBailianProfileDescriptor(d Descriptor) bool {
	return d.ProviderProfile == BailianProfile && d.PackageID == BailianPackage &&
		d.Runtime == "data" && d.Generation != nil && d.Generation.ID == "dashscope" &&
		d.Generation.Runtime == "builtin" && d.Generation.Version == d.Version &&
		len(d.Contributions) == 1 && d.Contributions[0] == (Contribution{ID: "dashscope", Kind: "provider"}) &&
		len(d.Requirements) == 0 && len(d.Definitions) == 0 && d.Compatibility == nil && d.Sandbox == nil
}
