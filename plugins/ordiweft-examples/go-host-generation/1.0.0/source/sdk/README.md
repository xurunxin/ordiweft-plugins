# Ordiweft public Go plugin SDK

This Apache-2.0 source distribution contains only pkg/aigc and pkg/pluginhost.
The original module path is preserved for existing consumers. It has no
non-standard-library dependencies. LICENSE/NOTICE are included per package.
The exact Go version is recorded in go.mod and sdk-export.json.

For an offline consumer, add a local replace directive for
gitea.nkit.top/xurx/Ordiweft pointing at this directory. Use an already installed
matching Go toolchain with GOTOOLCHAIN=local, GOPROXY=off and GOSUMDB=off.
The SDK does not provide a host, sandbox, credentials or permission to submit
work. Morpheus and Ordiweft host internals are not in this distribution.

sdk-export.json records the source commit, whether the source was dirty, and
the SHA-256/length of every exported file other than that manifest itself.
Hashes describe these bytes; they are not an official signature or test proof.