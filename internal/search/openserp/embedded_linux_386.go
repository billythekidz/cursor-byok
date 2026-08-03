//go:build openserp_embedded && linux && 386

package openserp

import _ "embed"

//go:embed assets/openserp-linux-386
var embeddedBinary []byte
