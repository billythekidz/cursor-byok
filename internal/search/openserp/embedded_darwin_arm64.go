//go:build openserp_embedded && darwin && arm64

package openserp

import _ "embed"

//go:embed assets/openserp-darwin-arm64
var embeddedBinary []byte
