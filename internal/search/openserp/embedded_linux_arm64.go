//go:build openserp_embedded && linux && arm64

package openserp

import _ "embed"

//go:embed assets/openserp-linux-arm64
var embeddedBinary []byte
