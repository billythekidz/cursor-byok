//go:build openserp_embedded && windows && arm64

package openserp

import _ "embed"

//go:embed assets/openserp-windows-arm64
var embeddedBinary []byte
