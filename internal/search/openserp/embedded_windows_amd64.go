//go:build openserp_embedded && windows && amd64

package openserp

import _ "embed"

//go:embed assets/openserp-windows-amd64
var embeddedBinary []byte
