//go:build openserp_embedded && darwin && amd64

package openserp

import _ "embed"

//go:embed assets/openserp-darwin-amd64
var embeddedBinary []byte
