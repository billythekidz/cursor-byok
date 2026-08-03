//go:build openserp_embedded && windows && 386

package openserp

import _ "embed"

//go:embed assets/openserp-windows-386
var embeddedBinary []byte
