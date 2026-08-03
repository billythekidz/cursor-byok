//go:build openserp_embedded && linux && amd64

package openserp

import _ "embed"

//go:embed assets/openserp-linux-amd64
var embeddedBinary []byte
